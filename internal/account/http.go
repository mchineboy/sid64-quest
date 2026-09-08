package account

import (
	"bytes"
	"database/sql"
	_ "embed"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

//go:embed account.html
var markup string

//go:embed account.css
var stylesheet string
var pages = template.Must(template.New("account").Parse(markup))

type page struct {
	Mode, Title, CSRF, Error, Notice, Username, Email, CharacterName, Host string
	User                                                                   *user
	Characters                                                             []character
	Count, Limit, TelnetPort, PETSCIIPort                                  int
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.URL.Path == "/account/style.css" {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(stylesheet))
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(405)
		return
	}
	mode := "account"
	switch r.URL.Path {
	case "/", "/account":
	case "/login":
		mode = "login"
	case "/signup":
		mode = "signup"
	default:
		http.NotFound(w, r)
		return
	}
	s, err := h.readSession(r)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) && !errors.Is(err, redis.Nil) {
			http.Error(w, "Account service unavailable. Please try again.", 503)
			return
		}
		s = nil
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form.", 400)
			return
		}
		if !csrfOK(s, r) {
			http.Error(w, "This form expired. Reload the page and try again.", 403)
			return
		}
	}
	var u *user
	if s != nil && s.UserID != uuid.Nil {
		u, err = h.loadUser(r.Context(), s.UserID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Account service unavailable.", 503)
			return
		}
		if err != nil || s.PasswordVersion != version(u.PasswordHash) {
			s = nil
			u = nil
		}
	}
	if mode == "account" && u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if mode != "account" && u != nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	if s == nil {
		s, err = h.newSession(w, r, nil)
		if err != nil {
			http.Error(w, "Account service unavailable.", 503)
			return
		}
	}
	p := page{Mode: mode, CSRF: s.CSRF, User: u, Limit: maxCharacters, TelnetPort: h.cfg.Server.TelnetPort, PETSCIIPort: h.cfg.Server.PETSCIIPort}
	base, _ := url.Parse(h.cfg.Auth.BaseURL)
	if base != nil {
		p.Host = base.Hostname()
	}
	if mode == "login" && r.URL.Query().Get("changed") == "1" {
		p.Notice = "Password changed. Sign in again with your new password."
	}
	if r.Method == "POST" {
		if mode == "login" || mode == "signup" {
			limited, e := h.limited(r)
			if e != nil {
				http.Error(w, "Account service unavailable.", 503)
				return
			}
			if limited {
				w.Header().Set("Retry-After", "900")
				http.Error(w, "Too many attempts. Try again in 15 minutes.", 429)
				return
			}
			p.Username = strings.ToLower(strings.TrimSpace(r.FormValue("username")))
			if mode == "signup" {
				p.Email = strings.TrimSpace(r.FormValue("email"))
				p.CharacterName = strings.TrimSpace(r.FormValue("character_name"))
				if !validEmail(p.Email) {
					p.Error = "Enter a valid email address."
				} else if !validName(p.CharacterName) {
					p.Error = "Character names need 3–30 letters, spaces, apostrophes or hyphens."
				} else if len(r.FormValue("password")) < 8 || len(r.FormValue("password")) > 72 {
					p.Error = "Use a password between 8 and 72 bytes."
				} else if r.FormValue("password") != r.FormValue("confirm_password") {
					p.Error = "The passwords don't match."
				} else {
					_, _, e := h.auth.RegisterPlayer(p.Username, p.Email, r.FormValue("password"), p.CharacterName)
					if e != nil {
						p.Error = "Couldn't create the account. Check the username (3–20 letters, numbers or underscores), email and character name; they must be available."
					}
				}
			}
			if p.Error == "" {
				signed, e := h.auth.AuthenticateUser(p.Username, r.FormValue("password"))
				if e != nil {
					p.Error = "Incorrect username or password."
				} else {
					u = &user{ID: signed.ID, PasswordHash: signed.PasswordHash}
					if _, e = h.newSession(w, r, u); e != nil {
						http.Error(w, "Could not start a session. Please sign in again.", 503)
						return
					}
					http.Redirect(w, r, "/account", http.StatusSeeOther)
					return
				}
			}
		} else {
			if h.change(w, r, u, &p) {
				return
			}
		}
	}
	if mode == "account" {
		p.Characters, err = h.characters(r.Context(), u.ID)
		if err != nil {
			http.Error(w, "Could not load characters.", 503)
			return
		}
		p.Count = len(p.Characters)
		switch r.URL.Query().Get("saved") {
		case "character":
			p.Notice = "Character created. Select them the next time you enter the MUD."
		case "name":
			p.Notice = "Name updated. Reconnect any open terminal session to see the new name."
		case "email":
			p.Notice = "Email updated."
		}
	}
	switch mode {
	case "login":
		p.Title = "Welcome back"
	case "signup":
		p.Title = "Join SID64 Quest"
	default:
		p.Title = "Your account"
	}
	var rendered bytes.Buffer
	if err := pages.Execute(&rendered, p); err != nil {
		http.Error(w, "Could not render the account page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if p.Error != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_, _ = w.Write(rendered.Bytes())
}

func (h *Handler) change(w http.ResponseWriter, r *http.Request, u *user, p *page) bool {
	action := r.FormValue("action")
	saved := ""
	switch action {
	case "logout":
		c, _ := r.Cookie(cookieName)
		if err := h.redis.Del(r.Context(), "websession:"+c.Value).Err(); err != nil {
			p.Error = "Could not sign out. Please try again."
			return false
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.secure(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
		http.Redirect(w, r, "/login", 303)
		return true
	case "character", "rename":
		name := strings.TrimSpace(r.FormValue("character_name"))
		if !validName(name) {
			p.Error = "Character names need 3–30 letters, spaces, apostrophes or hyphens."
			return false
		}
		var err error
		if action == "character" {
			err = h.createCharacter(r.Context(), u.ID, name)
			saved = "character"
		} else {
			id, e := uuid.Parse(r.FormValue("character_id"))
			if e != nil {
				p.Error = "Character not found."
				return false
			}
			err = h.renameCharacter(r.Context(), u.ID, id, name)
			saved = "name"
		}
		if err != nil {
			if err.Error() == "character limit reached" {
				p.Error = "You can have up to five characters."
			} else if errors.Is(err, sql.ErrNoRows) {
				p.Error = "Character not found."
			} else {
				p.Error = publicError(err)
			}
			return false
		}
	case "email", "password":
		limited, e := h.limited(r)
		if e != nil {
			p.Error = "Account service unavailable."
			return false
		}
		if limited {
			p.Error = "Too many attempts. Try again in 15 minutes."
			return false
		}
		if !passwordOK(u.PasswordHash, r.FormValue("current_password")) {
			p.Error = "Your current password is incorrect."
			return false
		}
		var err error
		if action == "email" {
			email := strings.TrimSpace(r.FormValue("email"))
			if !validEmail(email) {
				p.Error = "Enter a valid email address."
				return false
			}
			var result sql.Result
			result, err = h.db.ExecContext(r.Context(), `UPDATE users SET email=$1 WHERE id=$2 AND password_hash=$3`, email, u.ID, u.PasswordHash)
			if err == nil {
				n, _ := result.RowsAffected()
				if n != 1 {
					err = sql.ErrNoRows
				}
			}
			saved = "email"
		} else {
			password := r.FormValue("password")
			if len(password) < 8 || len(password) > 72 {
				p.Error = "Use a password between 8 and 72 bytes."
				return false
			}
			if password != r.FormValue("confirm_password") {
				p.Error = "The passwords don't match."
				return false
			}
			hash, e := h.auth.HashPassword(password)
			if e != nil {
				p.Error = "Could not change your password."
				return false
			}
			var result sql.Result
			result, err = h.db.ExecContext(r.Context(), `UPDATE users SET password_hash=$1 WHERE id=$2 AND password_hash=$3`, hash, u.ID, u.PasswordHash)
			if err == nil {
				n, _ := result.RowsAffected()
				if n != 1 {
					err = sql.ErrNoRows
				}
			}
			if err == nil {
				// Stored fingerprints invalidate every existing browser session.
				http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.secure(), SameSite: http.SameSiteLaxMode, MaxAge: -1})
				http.Redirect(w, r, "/login?changed=1", 303)
				return true
			}
		}
		if err != nil {
			p.Error = publicError(err)
			return false
		}
	default:
		p.Error = "Unknown account action."
		return false
	}
	http.Redirect(w, r, "/account?saved="+saved, 303)
	return true
}

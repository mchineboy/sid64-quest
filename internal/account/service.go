// Package account provides the browser account area alongside terminal pairing.
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	rcmail "github.com/tylerhardison/race-condition-kingdom/internal/mail"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "rck_account"
const sessionTTL = 12 * time.Hour
const maxCharacters = 5
const resetTTL = time.Hour
const forgotNotice = "If that account exists, we sent password reset instructions."

type session struct {
	UserID                uuid.UUID
	CSRF, PasswordVersion string
}
type user struct {
	AuthVersion                   int64
	ID                            uuid.UUID
	Username, Email, PasswordHash string
	Created                       time.Time
}
type character struct {
	ID                                            uuid.UUID
	Name, Location                                string
	Level, Health, MaxHealth, Stamina, MaxStamina int
	Gold                                          int64
}
type Handler struct {
	db           *sql.DB
	redis        *redis.Client
	auth         *auth.AuthService
	cfg          *config.Config
	mail         rcmail.Sender
	logger       *logrus.Logger
	anonymousKey []byte
}

func New(db *sql.DB, cache *redis.Client, a *auth.AuthService, cfg *config.Config, sender rcmail.Sender, logger *logrus.Logger) *Handler {
	if sender == nil {
		sender = rcmail.NewResend(cfg, logger)
	}
	key := []byte(cfg.Auth.SecretKey)
	if len(key) < 32 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic(err)
		}
	}
	return &Handler{db: db, redis: cache, auth: a, cfg: cfg, mail: sender, logger: logger, anonymousKey: key}
}
func (h *Handler) mailConfigured() bool { return h.mail != nil && h.mail.Configured() }
func token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func version(hash string) string { v := sha256.Sum256([]byte(hash)); return hex.EncodeToString(v[:]) }
func resetKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "password-reset:" + hex.EncodeToString(sum[:])
}
func (h *Handler) secure() bool { return strings.HasPrefix(h.cfg.Auth.BaseURL, "https://") }
func (h *Handler) newSession(w http.ResponseWriter, r *http.Request, u *user) (*session, error) {
	if u == nil {
		return h.newAnonymous(w)
	}
	id, err := token()
	if err != nil {
		return nil, err
	}
	csrf, err := token()
	if err != nil {
		return nil, err
	}
	s := &session{CSRF: csrf}
	if u != nil {
		s.UserID = u.ID
		s.PasswordVersion = version(u.PasswordHash)
	}
	data, _ := json.Marshal(s)
	if err = h.redis.Set(r.Context(), "websession:"+id, data, sessionTTL).Err(); err != nil {
		return nil, err
	}
	if old, e := r.Cookie(cookieName); e == nil && len(old.Value) == 64 {
		if err = h.redis.Del(r.Context(), "websession:"+old.Value).Err(); err != nil {
			return nil, err
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, Secure: h.secure(), SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds())})
	return s, nil
}
func (h *Handler) readSession(r *http.Request) (*session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(c.Value, "a.") {
		return h.readAnonymous(c.Value)
	}
	if len(c.Value) != 64 {
		return nil, redis.Nil
	}
	data, err := h.redis.Get(r.Context(), "websession:"+c.Value).Bytes()
	if err != nil {
		return nil, err
	}
	var s session
	err = json.Unmarshal(data, &s)
	return &s, err
}
func (h *Handler) loadUser(ctx context.Context, id uuid.UUID) (*user, error) {
	var u user
	err := h.db.QueryRowContext(ctx, `SELECT id,username,email,password_hash,created_at,auth_version FROM users WHERE id=$1 AND is_active=true`, id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Created, &u.AuthVersion)
	return &u, err
}
func (h *Handler) findActiveUser(ctx context.Context, identity string) (*user, error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	if identity == "" {
		return nil, sql.ErrNoRows
	}
	var u user
	err := h.db.QueryRowContext(ctx, `SELECT id,username,email,password_hash,created_at,auth_version FROM users WHERE is_active=true AND (LOWER(username)=$1 OR LOWER(email)=$1) AND (SELECT COUNT(*) FROM users WHERE is_active=true AND (LOWER(username)=$1 OR LOWER(email)=$1))=1`, identity).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Created, &u.AuthVersion)
	return &u, err
}
func validName(name string) bool   { return auth.ValidCharacterName(name) }
func validEmail(email string) bool { return auth.ValidEmail(email) }
func publicError(err error) string {
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		return "That name or email is already in use."
	}
	return "We couldn't save that change. Please try again."
}
func (h *Handler) limited(r *http.Request) (bool, error) {
	return h.auth.LimitLogin(r, r.FormValue("username"))
}
func (h *Handler) forgotLimited(r *http.Request, identity string) (bool, error) {
	ip, err := h.auth.ClientIP(r)
	if err != nil {
		return false, err
	}
	identity = strings.ToLower(strings.TrimSpace(identity))
	script := `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`
	nIP, err := h.redis.Eval(r.Context(), script, []string{"forgot-attempts:ip:" + ip}).Int()
	if err != nil {
		return false, err
	}
	nID, err := h.redis.Eval(r.Context(), script, []string{"forgot-attempts:id:" + identity}).Int()
	if err != nil {
		return false, err
	}
	return nIP > 5 || nID > 5, nil
}

type resetGrant struct {
	UserID      uuid.UUID
	AuthVersion int64
}

func (h *Handler) createResetToken(ctx context.Context, u *user) (string, error) {
	raw, err := token()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(resetGrant{u.ID, u.AuthVersion})
	if err != nil {
		return "", err
	}
	if err = h.redis.Set(ctx, resetKey(raw), data, resetTTL).Err(); err != nil {
		return "", err
	}
	return raw, nil
}
func (h *Handler) readResetToken(ctx context.Context, raw string, consume bool) (resetGrant, error) {
	var grant resetGrant
	if len(raw) != 64 {
		return grant, redis.Nil
	}
	var data []byte
	var err error
	if consume {
		data, err = h.redis.GetDel(ctx, resetKey(raw)).Bytes()
	} else {
		data, err = h.redis.Get(ctx, resetKey(raw)).Bytes()
	}
	if err != nil {
		return grant, err
	}
	if json.Unmarshal(data, &grant) != nil || grant.UserID == uuid.Nil || grant.AuthVersion < 1 {
		return resetGrant{}, redis.Nil
	}
	return grant, nil
}
func (h *Handler) consumeResetToken(ctx context.Context, raw string) (resetGrant, error) {
	return h.readResetToken(ctx, raw, true)
}
func (h *Handler) peekResetToken(ctx context.Context, raw string) (resetGrant, error) {
	grant, err := h.readResetToken(ctx, raw, false)
	if err != nil {
		return grant, err
	}
	u, err := h.loadUser(ctx, grant.UserID)
	if err != nil {
		return grant, err
	}
	if u.AuthVersion != grant.AuthVersion {
		return grant, redis.Nil
	}
	return grant, nil
}
func (h *Handler) sendResetMail(ctx context.Context, u *user, raw string) error {
	link := strings.TrimRight(h.cfg.Auth.BaseURL, "/") + "/reset?token=" + raw
	subject := "Reset your SID64 Quest password"
	text := "Reset your SID64 Quest password using this link (expires in one hour):\n\n" + link + "\n\nIf you did not request this, you can ignore this email."
	htmlBody := "<p>Reset your SID64 Quest password using this link (expires in one hour):</p><p><a href=\"" + html.EscapeString(link) + "\">" + html.EscapeString(link) + "</a></p><p>If you did not request this, you can ignore this email.</p>"
	return h.mail.Send(ctx, u.Email, subject, text, htmlBody)
}
func (h *Handler) characters(ctx context.Context, id uuid.UUID) ([]character, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT c.id,c.name,COALESCE(r.name,'Unknown'),c.level,c.health,c.max_health,c.stamina,c.max_stamina,c.gold FROM characters c LEFT JOIN rooms r ON r.id=c.current_room_id WHERE c.user_id=$1 ORDER BY c.created_at,c.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []character{}
	for rows.Next() {
		var c character
		if err = rows.Scan(&c.ID, &c.Name, &c.Location, &c.Level, &c.Health, &c.MaxHealth, &c.Stamina, &c.MaxStamina, &c.Gold); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (h *Handler) createCharacter(ctx context.Context, id uuid.UUID, name string) error {
	if !validName(name) {
		return fmt.Errorf("invalid character name")
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner uuid.UUID
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND is_active=true FOR UPDATE`, id).Scan(&owner); err != nil {
		return err
	}
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM characters WHERE user_id=$1`, id).Scan(&n); err != nil {
		return err
	}
	if n >= maxCharacters {
		return fmt.Errorf("character limit reached")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO characters(user_id,name,gold,current_room_id) VALUES($1,$2,$3,(SELECT id FROM rooms WHERE name='Town Square' ORDER BY created_at LIMIT 1))`, id, name, game.GoldValue(h.cfg.Game.StartingGold))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (h *Handler) renameCharacter(ctx context.Context, owner, id uuid.UUID, name string) error {
	if !validName(name) {
		return fmt.Errorf("invalid character name")
	}
	result, err := h.db.ExecContext(ctx, `UPDATE characters SET name=$1 WHERE id=$2 AND user_id=$3`, name, id, owner)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func passwordOK(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func csrfOK(s *session, r *http.Request) bool {
	return s != nil && len(s.CSRF) == 64 && subtle.ConstantTimeCompare([]byte(s.CSRF), []byte(r.FormValue("csrf"))) == 1
}

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
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "rck_account"
const sessionTTL = 12 * time.Hour
const maxCharacters = 5

type session struct {
	UserID                uuid.UUID
	CSRF, PasswordVersion string
}
type user struct {
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
	db    *sql.DB
	redis *redis.Client
	auth  *auth.AuthService
	cfg   *config.Config
}

func New(db *sql.DB, cache *redis.Client, a *auth.AuthService, cfg *config.Config) *Handler {
	return &Handler{db: db, redis: cache, auth: a, cfg: cfg}
}
func token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func version(hash string) string { v := sha256.Sum256([]byte(hash)); return hex.EncodeToString(v[:]) }
func (h *Handler) secure() bool  { return strings.HasPrefix(h.cfg.Auth.BaseURL, "https://") }
func (h *Handler) newSession(w http.ResponseWriter, r *http.Request, u *user) (*session, error) {
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
	if old, e := r.Cookie(cookieName); e == nil {
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
	err := h.db.QueryRowContext(ctx, `SELECT id,username,email,password_hash,created_at FROM users WHERE id=$1 AND is_active=true`, id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Created)
	return &u, err
}
func validName(name string) bool {
	if len(name) < 3 || len(name) > 30 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == ' ' || r == '\'' || r == '-') {
			return false
		}
	}
	return true
}
func validEmail(email string) bool {
	a, e := mail.ParseAddress(email)
	return e == nil && a.Address == email && len(email) <= 254
}
func publicError(err error) string {
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		return "That name or email is already in use."
	}
	return "We couldn't save that change. Please try again."
}
func (h *Handler) limited(r *http.Request) (bool, error) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	// One Redis script makes the counter and expiry atomic.
	n, err := h.redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`, []string{"account-attempts:" + ip + ":" + strings.ToLower(strings.TrimSpace(r.FormValue("username")))}).Int()
	return n > 20, err
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
	_, err = tx.ExecContext(ctx, `INSERT INTO characters(user_id,name,gold,current_room_id) VALUES($1,$2,$3,(SELECT id FROM rooms WHERE name='Town Square' ORDER BY created_at LIMIT 1))`, id, name, h.cfg.Game.StartingGold)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (h *Handler) renameCharacter(ctx context.Context, owner, id uuid.UUID, name string) error {
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

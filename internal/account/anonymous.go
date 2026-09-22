package account

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const anonymousTTL = 15 * time.Minute

// Anonymous forms use a signed, expiring cookie, never a server-side allocation.
// The unpredictable CSRF nonce is bound to that cookie, not an account identity.
func (h *Handler) anonymousMAC(value string) string {
	mac := hmac.New(sha256.New, h.anonymousKey)
	mac.Write([]byte("rck-anonymous-v1:" + value))
	return hex.EncodeToString(mac.Sum(nil))
}
func (h *Handler) newAnonymous(w http.ResponseWriter) (*session, error) {
	csrf, err := token()
	if err != nil {
		return nil, err
	}
	value := "a." + strconv.FormatInt(time.Now().Add(anonymousTTL).Unix(), 10) + "." + csrf
	value += "." + h.anonymousMAC(value)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", HttpOnly: true, Secure: h.secure(), SameSite: http.SameSiteLaxMode, MaxAge: int(anonymousTTL.Seconds())})
	return &session{CSRF: csrf}, nil
}
func (h *Handler) readAnonymous(value string) (*session, error) {
	fields := strings.Split(value, ".")
	if len(fields) != 4 || len(fields[2]) != 64 || len(fields[3]) != 64 {
		return nil, redis.Nil
	}
	expires, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || expires <= time.Now().Unix() || expires > time.Now().Add(anonymousTTL).Unix()+5 {
		return nil, redis.Nil
	}
	payload := strings.Join(fields[:3], ".")
	if !hmac.Equal([]byte(fields[3]), []byte(h.anonymousMAC(payload))) {
		return nil, redis.Nil
	}
	return &session{CSRF: fields[2]}, nil
}

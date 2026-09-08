package account

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func testHandler(t *testing.T, db *sql.DB) (*Handler, *miniredis.Miniredis) {
	cache := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: cache.Addr()})
	t.Cleanup(func() { client.Close() })
	cfg := config.LoadFromEnv()
	cfg.Auth.BCryptCost = 4
	return New(db, client, auth.NewAuthService(db, client, cfg, logrus.New()), cfg), cache
}
func request(h *Handler, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	r.RemoteAddr = "127.0.0.1:9999"
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func start(t *testing.T, h *Handler, path string) (*http.Cookie, *session) {
	w := request(h, "GET", path, nil, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, w.Result().Cookies(), 1)
	c := w.Result().Cookies()[0]
	r := httptest.NewRequest("GET", path, nil)
	r.AddCookie(c)
	s, e := h.readSession(r)
	require.NoError(t, e)
	return c, s
}
func TestAnonymousFormsAndCSRF(t *testing.T) {
	h, cache := testHandler(t, nil)
	require.Equal(t, 303, request(h, "GET", "/account", nil, nil).Code)
	c, s := start(t, h, "/signup")
	require.True(t, c.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, c.SameSite)
	w := request(h, "POST", "/signup", url.Values{"csrf": {"wrong"}}, c)
	require.Equal(t, 403, w.Code)
	cache.FastForward(sessionTTL + time.Second)
	require.Equal(t, 403, request(h, "POST", "/signup", url.Values{"csrf": {s.CSRF}}, c).Code)
	require.Equal(t, 405, request(h, "DELETE", "/login", nil, nil).Code)
	require.Equal(t, 200, request(h, "GET", "/account/style.css", nil, nil).Code)
}
func TestRateLimit(t *testing.T) {
	h, cache := testHandler(t, nil)
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "127.0.0.1:4"
	for i := 0; i < 20; i++ {
		limited, e := h.limited(r)
		require.NoError(t, e)
		require.False(t, limited)
	}
	limited, e := h.limited(r)
	require.NoError(t, e)
	require.True(t, limited)
	cache.FastForward(16 * time.Minute)
	limited, e = h.limited(r)
	require.NoError(t, e)
	require.False(t, limited)
}
func TestNameValidation(t *testing.T) {
	for _, name := range []string{"Ari", "Anne-Marie", "O'Neil", "The Steward"} {
		require.True(t, validName(name))
	}
	for _, name := range []string{"ab", "abc\x1b[2J", "<script>", strings.Repeat("a", 31)} {
		require.False(t, validName(name))
	}
	require.False(t, validEmail("Name <test@example.org>"))
	require.True(t, validEmail("test@example.org"))
}
func TestAccountJourneyAndOwnership(t *testing.T) {
	cfg := config.LoadFromEnv()
	db, e := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, e)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		t.Skipf("Postgres unavailable: %v", e)
	}
	h, _ := testHandler(t, db)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	username := "web_" + suffix
	email := username + "@example.test"
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE username=$1`, username) })
	cookie, s := start(t, h, "/signup")
	// Letters only, distinct from game test names.
	name := "Web " + strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'a' + r - '0'
		}
		return r
	}, suffix)
	form := url.Values{"csrf": {s.CSRF}, "username": {username}, "email": {email}, "character_name": {name}, "password": {"test-password-123"}, "confirm_password": {"test-password-123"}}
	w := request(h, "POST", "/signup", form, cookie)
	require.Equal(t, 303, w.Code, w.Body.String())
	authenticated := w.Result().Cookies()[0]
	require.NotEqual(t, cookie.Value, authenticated.Value)
	// Login rotated and removed the anonymous session.
	require.Equal(t, 403, request(h, "POST", "/signup", form, cookie).Code)
	r := httptest.NewRequest("GET", "/account", nil)
	r.AddCookie(authenticated)
	s, e = h.readSession(r)
	require.NoError(t, e)
	w = request(h, "GET", "/account", nil, authenticated)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), name)
	require.Contains(t, w.Body.String(), "Town Square")
	chars, e := h.characters(context.Background(), s.UserID)
	require.NoError(t, e)
	require.Len(t, chars, 1)
	require.ErrorIs(t, h.renameCharacter(context.Background(), uuid.New(), chars[0].ID, "Stolen Name"), sql.ErrNoRows)
	w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"rename"}, "character_id": {chars[0].ID.String()}, "character_name": {name + " New"}}, authenticated)
	require.Equal(t, 303, w.Code, w.Body.String())
	for _, extra := range []string{" Two", " Three", " Four", " Five"} {
		w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"character"}, "character_name": {name + extra}}, authenticated)
		require.Equal(t, 303, w.Code, w.Body.String())
	}
	w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"character"}, "character_name": {name + " Six"}}, authenticated)
	require.Equal(t, 422, w.Code)
	require.Contains(t, w.Body.String(), "up to five")
	// Current-password check protects email changes.
	w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"email"}, "email": {"new-" + email}, "current_password": {"wrong"}}, authenticated)
	require.Equal(t, 422, w.Code)
	w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"email"}, "email": {"new-" + email}, "current_password": {"test-password-123"}}, authenticated)
	require.Equal(t, 303, w.Code)
	w = request(h, "POST", "/account", url.Values{"csrf": {s.CSRF}, "action": {"password"}, "current_password": {"test-password-123"}, "password": {"new-test-password"}, "confirm_password": {"new-test-password"}}, authenticated)
	require.Equal(t, 303, w.Code)
	require.Equal(t, "/login", request(h, "GET", "/account", nil, authenticated).Header().Get("Location"))
	loginCookie, ls := start(t, h, "/login")
	w = request(h, "POST", "/login", url.Values{"csrf": {ls.CSRF}, "username": {username}, "password": {"test-password-123"}}, loginCookie)
	require.Equal(t, 422, w.Code)
	w = request(h, "POST", "/login", url.Values{"csrf": {ls.CSRF}, "username": {username}, "password": {"new-test-password"}}, loginCookie)
	require.Equal(t, 303, w.Code)
	loginCookie = w.Result().Cookies()[0]
	r = httptest.NewRequest("GET", "/account", nil)
	r.AddCookie(loginCookie)
	ls, e = h.readSession(r)
	require.NoError(t, e)
	w = request(h, "POST", "/account", url.Values{"csrf": {ls.CSRF}, "action": {"logout"}}, loginCookie)
	require.Equal(t, 303, w.Code)
	require.Equal(t, "/login", request(h, "GET", "/account", nil, loginCookie).Header().Get("Location"))
}

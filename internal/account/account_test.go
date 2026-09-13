package account

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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

type fakeMail struct {
	configured bool
	sent       []mailMsg
	err        error
}
type mailMsg struct{ to, subject, text, html string }

func (f *fakeMail) Configured() bool { return f != nil && f.configured }
func (f *fakeMail) Send(ctx context.Context, to, subject, text, html string) error {
	_ = ctx
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, mailMsg{to, subject, text, html})
	return nil
}

func testHandler(t *testing.T, db *sql.DB) (*Handler, *miniredis.Miniredis, *fakeMail) {
	cache := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: cache.Addr()})
	t.Cleanup(func() { client.Close() })
	cfg := config.LoadFromEnv()
	cfg.Auth.BCryptCost = 4
	cfg.Auth.BaseURL = "https://sid64.quest"
	mailer := &fakeMail{configured: true}
	return New(db, client, auth.NewAuthService(db, client, cfg, logrus.New()), cfg, mailer, logrus.New()), cache, mailer
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
	h, cache, _ := testHandler(t, nil)
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
	h, cache, _ := testHandler(t, nil)
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
func TestForgotDisabledWithoutMail(t *testing.T) {
	h, _, mailer := testHandler(t, nil)
	mailer.configured = false
	require.Equal(t, 404, request(h, "GET", "/forgot", nil, nil).Code)
	require.Equal(t, 404, request(h, "GET", "/reset", nil, nil).Code)
	w := request(h, "GET", "/login", nil, nil)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "Email recovery isn't available yet")
	require.NotContains(t, w.Body.String(), `href="/forgot"`)
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
	h, _, _ := testHandler(t, db)
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
func TestPasswordResetFlow(t *testing.T) {
	cfg := config.LoadFromEnv()
	db, e := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, e)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		t.Skipf("Postgres unavailable: %v", e)
	}
	h, cache, mailer := testHandler(t, db)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	username := "rst_" + suffix
	email := username + "@example.test"
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE username=$1`, username) })
	cookie, s := start(t, h, "/signup")
	name := "Rst " + strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'a' + r - '0'
		}
		return r
	}, suffix)
	w := request(h, "POST", "/signup", url.Values{"csrf": {s.CSRF}, "username": {username}, "email": {email}, "character_name": {name}, "password": {"original-password"}, "confirm_password": {"original-password"}}, cookie)
	require.Equal(t, 303, w.Code, w.Body.String())
	authenticated := w.Result().Cookies()[0]
	loginPage := request(h, "GET", "/login", nil, nil)
	require.Contains(t, loginPage.Body.String(), `href="/forgot"`)

	// Unknown identity still returns the generic notice and does not send mail.
	forgotCookie, fs := start(t, h, "/forgot")
	w = request(h, "POST", "/forgot", url.Values{"csrf": {fs.CSRF}, "identity": {"missing-" + username}}, forgotCookie)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), forgotNotice)
	require.Empty(t, mailer.sent)

	forgotCookie, fs = start(t, h, "/forgot")
	before := len(mailer.sent)
	w = request(h, "POST", "/forgot", url.Values{"csrf": {fs.CSRF}, "identity": {email}}, forgotCookie)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), forgotNotice)
	require.Len(t, mailer.sent, before+1)
	require.Equal(t, email, mailer.sent[before].to)
	token := regexp.MustCompile(`token=([0-9a-f]{64})`).FindStringSubmatch(mailer.sent[before].text)
	require.Len(t, token, 2)

	// Bad and expired tokens are rejected.
	require.Equal(t, 422, request(h, "GET", "/reset?token="+strings.Repeat("a", 64), nil, nil).Code)
	resetCookie, rs := start(t, h, "/reset?token="+token[1])
	require.Equal(t, 422, request(h, "POST", "/reset", url.Values{"csrf": {rs.CSRF}, "token": {token[1]}, "password": {"short"}, "confirm_password": {"short"}}, resetCookie).Code)
	w = request(h, "POST", "/reset", url.Values{"csrf": {rs.CSRF}, "token": {token[1]}, "password": {"reset-password-99"}, "confirm_password": {"reset-password-99"}}, resetCookie)
	require.Equal(t, 303, w.Code, w.Body.String())
	require.Equal(t, "/login?reset=1", w.Header().Get("Location"))
	// Token is single-use.
	require.Equal(t, 422, request(h, "GET", "/reset?token="+token[1], nil, nil).Code)
	// Old browser session is invalidated by password version.
	require.Equal(t, "/login", request(h, "GET", "/account", nil, authenticated).Header().Get("Location"))
	loginCookie, ls := start(t, h, "/login")
	w = request(h, "POST", "/login", url.Values{"csrf": {ls.CSRF}, "username": {username}, "password": {"original-password"}}, loginCookie)
	require.Equal(t, 422, w.Code)
	w = request(h, "POST", "/login", url.Values{"csrf": {ls.CSRF}, "username": {username}, "password": {"reset-password-99"}}, loginCookie)
	require.Equal(t, 303, w.Code)

	cache.FastForward(16 * time.Minute)
	// Rate limit forgot requests.
	forgotCookie, fs = start(t, h, "/forgot")
	for i := 0; i < 5; i++ {
		w = request(h, "POST", "/forgot", url.Values{"csrf": {fs.CSRF}, "identity": {"limit-" + username}}, forgotCookie)
		require.Equal(t, 200, w.Code, w.Body.String())
		forgotCookie, fs = start(t, h, "/forgot")
	}
	w = request(h, "POST", "/forgot", url.Values{"csrf": {fs.CSRF}, "identity": {"limit-" + username}}, forgotCookie)
	require.Equal(t, 429, w.Code)
}

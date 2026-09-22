package account

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func TestAnonymousFormsHaveNoRedisAllocation(t *testing.T) {
	h, cache, _ := testHandler(t, nil)
	for i := 0; i < 100; i++ {
		w := request(h, "GET", "/login", nil, nil)
		require.Equal(t, 200, w.Code)
		require.Len(t, w.Result().Cookies(), 1)
	}
	require.Empty(t, cache.Keys())
	c, s := start(t, h, "/login")
	c.Value = c.Value[:len(c.Value)-1] + "x"
	require.Equal(t, 403, request(h, "POST", "/login", url.Values{"csrf": {s.CSRF}}, c).Code)
}

func securityDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := config.LoadFromEnv()
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	return db
}

func TestRecoveryGenerationRevokesSiblingLinks(t *testing.T) {
	db := securityDB(t)
	h, _, _ := testHandler(t, db)
	id := uuid.New()
	hash, err := h.auth.HashPassword("original-password")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES($1,$2,$3,$4)`, id, "sec_"+id.String()[:8], id.String()+"@example.test", hash)
	require.NoError(t, err)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=$1`, id) })
	u, err := h.loadUser(context.Background(), id)
	require.NoError(t, err)
	first, err := h.createResetToken(context.Background(), u)
	require.NoError(t, err)
	sibling, err := h.createResetToken(context.Background(), u)
	require.NoError(t, err)
	c, s := start(t, h, "/reset?token="+first)
	submit := func(raw string, c *http.Cookie, s *session) int {
		return request(h, "POST", "/reset", url.Values{"csrf": {s.CSRF}, "token": {raw}, "password": {"replacement-password"}, "confirm_password": {"replacement-password"}}, c).Code
	}
	require.Equal(t, 303, submit(first, c, s))
	require.Equal(t, 422, submit(sibling, c, s))
	u, err = h.loadUser(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, int64(2), u.AuthVersion)
	token, err := h.createResetToken(context.Background(), u)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users SET email=$2 WHERE id=$1`, id, "new-"+u.Email)
	require.NoError(t, err)
	require.Equal(t, 422, submit(token, c, s))
	u, err = h.loadUser(context.Background(), id)
	require.NoError(t, err)
	token, err = h.createResetToken(context.Background(), u)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE users SET password_hash=$2 WHERE id=$1`, id, hash)
	require.NoError(t, err)
	require.Equal(t, 422, submit(token, c, s))
}

func TestConcurrentRecoveryHasOneWinner(t *testing.T) {
	db := securityDB(t)
	h, _, _ := testHandler(t, db)
	id := uuid.New()
	_, err := db.Exec(`INSERT INTO users(id,username,email,password_hash) VALUES($1,$2,$3,'old')`, id, "race_"+id.String()[:8], id.String()+"@example.test")
	require.NoError(t, err)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=$1`, id) })
	u, err := h.loadUser(context.Background(), id)
	require.NoError(t, err)
	tokens := make([]string, 2)
	for i := range tokens {
		tokens[i], err = h.createResetToken(context.Background(), u)
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	barrier := make(chan struct{})
	for _, raw := range tokens {
		wg.Add(1)
		go func(raw string) {
			defer wg.Done()
			<-barrier
			form := url.Values{"token": {raw}, "password": {"replacement-password"}, "confirm_password": {"replacement-password"}}
			r := httptest.NewRequest("POST", "/reset", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			p := page{TokenOK: true}
			h.handleReset(w, r, &p)
			if p.Error != "" {
				results <- 422
			} else {
				results <- w.Code
			}
		}(raw)
	}
	close(barrier)
	wg.Wait()
	close(results)
	wins := 0
	for code := range results {
		if code == 303 {
			wins++
		}
	}
	require.Equal(t, 1, wins)
}

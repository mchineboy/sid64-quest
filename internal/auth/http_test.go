package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func TestProxyClientIsolationAndSpoofing(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.Auth.ProxyToken = strings.Repeat("p", 32)
	service := newTestAuthService(t, cfg)
	handler := service.ProtectHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	send := func(ip, secret string) int {
		r := httptest.NewRequest("POST", "/auth", nil)
		r.RemoteAddr = "127.0.0.1:8000"
		r.Header.Set("X-RCK-Client-IP", ip)
		r.Header.Set("X-RCK-Proxy-Token", secret)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 60; i++ {
		require.Equal(t, 204, send("192.0.2.1", cfg.Auth.ProxyToken))
	}
	require.Equal(t, 429, send("192.0.2.1", cfg.Auth.ProxyToken))
	require.Equal(t, 204, send("192.0.2.2", cfg.Auth.ProxyToken))
	require.Equal(t, 403, send("192.0.2.3", ""))
	require.Equal(t, 403, send("192.0.2.3", "wrong"))
	require.Equal(t, 403, send("not-an-ip", cfg.Auth.ProxyToken))
	cfg.Auth.ProxyToken = ""
	r := httptest.NewRequest("POST", "/auth", nil)
	r.RemoteAddr = "192.0.2.4:8000"
	r.Header.Set("X-Forwarded-For", "192.0.2.99")
	r.Header.Set("X-RCK-Client-IP", "192.0.2.99")
	ip, err := service.ClientIP(r)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.4", ip)
}

func TestLoginLimitSharedAcrossClientsAndPaths(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.Auth.ProxyToken = ""
	service := newTestAuthService(t, cfg)
	for i := 0; i < 20; i++ {
		r := httptest.NewRequest("POST", "/login", nil)
		r.RemoteAddr = "192.0.2.1:9000"
		limited, err := service.LimitLogin(r, "Alice")
		require.NoError(t, err)
		require.False(t, limited)
	}
	r := httptest.NewRequest("POST", "/api/auth", nil)
	r.RemoteAddr = "192.0.2.2:9000"
	limited, err := service.LimitLogin(r, " ALICE ")
	require.NoError(t, err)
	require.True(t, limited)
	limited, err = service.LimitLogin(r, "bob")
	require.NoError(t, err)
	require.False(t, limited)
}

func TestRegistrationRejectsTerminalControlsBeforeDatabase(t *testing.T) {
	service := newTestAuthService(t, config.LoadFromEnv())
	for _, name := range []string{"Eve\x1b[2J", "Eve\nAdmin", "Eve\u009b2J", "Eve\x7f"} {
		_, _, err := service.RegisterPlayer("review", "review@example.test", "password123", name)
		require.ErrorContains(t, err, "character name")
	}
}

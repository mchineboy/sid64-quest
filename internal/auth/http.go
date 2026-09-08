package auth

import (
	"context"
	"net"
	"net/http"
	"time"
)

// ProtectHTTP applies body limits and per-client rate limits to pairing endpoints.
// Forwarded addresses are deliberately not trusted; the reverse proxy is local.
func (as *AuthService) ProtectHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "POST" {
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			n, err := as.redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],60) end;return n`, []string{"http-posts:" + host}).Int()
			if err != nil {
				http.Error(w, "Authentication temporarily unavailable", 503)
				return
			}
			if n > 60 {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too many requests; try again shortly", 429)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (as *AuthService) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if as.db.PingContext(ctx) != nil || as.redis.Ping(ctx).Err() != nil {
		http.Error(w, "not ready", 503)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ready\n"))
}

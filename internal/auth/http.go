package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ClientIP accepts the public proxy's address assertion only with its shared
// credential. Caddy overwrites both headers; intermediate proxies preserve them.
// With no proxy credential configured, only the actual TCP peer is trusted.
func (as *AuthService) ClientIP(r *http.Request) (string, error) {
	if secret := as.config.Auth.ProxyToken; secret != "" {
		if len(secret) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-RCK-Proxy-Token")), []byte(secret)) != 1 {
			return "", fmt.Errorf("untrusted HTTP proxy")
		}
		ip := net.ParseIP(r.Header.Get("X-RCK-Client-IP"))
		if ip == nil {
			return "", fmt.Errorf("invalid client address")
		}
		return ip.String(), nil
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("invalid peer address")
	}
	return ip.String(), nil
}

func (as *AuthService) limit(ctx context.Context, key string, maximum int, ttl time.Duration) (bool, error) {
	n, err := as.redis.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]) end;return n`, []string{key}, int(ttl.Seconds())).Int()
	return n > maximum, err
}

// LimitLogin is shared by browser and pairing/API logins. Hash the identity to
// bound Redis key size and avoid storing account names in rate-limit keys.
func (as *AuthService) LimitLogin(r *http.Request, username string) (bool, error) {
	ip, err := as.ClientIP(r)
	if err != nil {
		return false, err
	}
	identity := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	limited, err := as.limit(r.Context(), "login-client:"+ip, 60, 15*time.Minute)
	if err != nil || limited {
		return limited, err
	}
	return as.limit(r.Context(), "login-identity:"+hex.EncodeToString(identity[:]), 20, 15*time.Minute)
}

func (as *AuthService) ProtectHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		// Container probes do not pass through the public proxy.
		if r.Method == "GET" && (r.URL.Path == "/health" || r.URL.Path == "/ready") {
			next.ServeHTTP(w, r)
			return
		}
		ip, err := as.ClientIP(r)
		if err != nil {
			http.Error(w, "Untrusted proxy request", 403)
			return
		}
		if r.Method == "POST" {
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
			limited, err := as.limit(r.Context(), "http-posts:"+ip, 60, time.Minute)
			if err != nil {
				http.Error(w, "Authentication temporarily unavailable", 503)
				return
			}
			if limited {
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

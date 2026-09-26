package auth

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func newTestAuthService(t *testing.T, cfg *config.Config) *AuthService {
	t.Helper()

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return NewAuthService(nil, redisClient, cfg, logger)
}

// MockDB implements a simple in-memory database for testing
type MockDB struct {
	users map[string]map[string]interface{}
}

func NewMockDB() *MockDB {
	return &MockDB{
		users: make(map[string]map[string]interface{}),
	}
}

func (m *MockDB) QueryRow(query string, args ...interface{}) *sql.Row {
	// This is a simplified mock - in real tests you'd use something like sqlmock
	return nil
}

func (m *MockDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}

func TestAuthService_GenerateAuthToken(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}

	authService := newTestAuthService(t, cfg)

	// Test
	sessionID := "test-session-123"
	token, err := authService.GenerateAuthToken(sessionID)

	// Assertions
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Len(t, token, 64) // 32 bytes hex encoded = 64 characters

	// Verify token can be validated
	retrievedSessionID, err := authService.ValidateAuthToken(token)
	require.NoError(t, err)
	assert.Equal(t, sessionID, retrievedSessionID)

	session, err := authService.GetSession(sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, session.ConnectionID)
}

func TestAuthService_ValidateAuthToken_InvalidToken(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}

	authService := newTestAuthService(t, cfg)

	// Test with invalid token
	_, err := authService.ValidateAuthToken("invalid-token")

	// Should return error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired token")

}

func TestAuthService_MarkTokenUsed(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}

	authService := newTestAuthService(t, cfg)

	// Generate a token
	sessionID := "test-session-456"
	token, err := authService.GenerateAuthToken(sessionID)
	require.NoError(t, err)

	// Mark token as used
	err = authService.MarkTokenUsed(token)
	require.NoError(t, err)

	// Used tokens cannot be used to start a second authentication flow.
	_, err = authService.ValidateAuthToken(token)
	assert.ErrorContains(t, err, "token already used")
}

func TestAuthService_GetAuthURL(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BaseURL: "https://example.com",
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	authService := NewAuthService(nil, nil, cfg, logger)

	// Test
	token := "test-token-123"
	url := authService.GetAuthURL(token)

	// Assertions
	expected := "https://example.com/auth?token=test-token-123"
	assert.Equal(t, expected, url)
}

func TestAuthService_PairingChallenge(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{
		TokenExpiry:   5 * time.Minute,
		SessionExpiry: time.Hour,
		BaseURL:       "https://mud.example/",
	}}
	authService := newTestAuthService(t, cfg)

	token, code, err := authService.GenerateAuthChallenge("pairing-session")
	require.NoError(t, err)
	assert.Regexp(t, `^[0-9A-HJ-KM-NP-TV-Z]{4}-[0-9A-HJ-KM-NP-TV-Z]{4}$`, code)

	resolved, err := authService.ResolvePairingCode(strings.ToLower(strings.ReplaceAll(code, "-", "")))
	require.NoError(t, err)
	assert.Equal(t, token, resolved)
	assert.Equal(t, "https://mud.example/p/"+code, authService.GetPairingURL(code))
	assert.Equal(t, "https://mud.example/pair", authService.GetPairingEntryURL())

	_, err = authService.ResolvePairingCode("bad-code")
	require.Error(t, err)
}

func TestAuthService_LinkTokenToSession(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{
		TokenExpiry:   5 * time.Minute,
		SessionExpiry: time.Hour,
	}}
	authService := newTestAuthService(t, cfg)

	token, err := authService.GenerateAuthToken("connection-123")
	require.NoError(t, err)
	userID := uuid.New()
	characterID := uuid.New()
	require.NoError(t, authService.LinkTokenToSession(token, "connection-123", userID, characterID, 1))

	session, err := authService.GetSession("connection-123")
	require.NoError(t, err)
	assert.Equal(t, userID, session.UserID)
	assert.Equal(t, characterID, session.CharacterID)
	assert.Equal(t, token, session.AuthToken)
}

func TestAuthService_HashPassword(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BCryptCost: 4, // Use low cost for faster tests
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	authService := NewAuthService(nil, nil, cfg, logger)

	// Test
	password := "test-password-123"
	hash, err := authService.HashPassword(password)

	// Assertions
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
	assert.Contains(t, hash, "$2a$") // bcrypt hash prefix
}

// Benchmark tests
func BenchmarkAuthService_GenerateAuthToken(b *testing.B) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}

	redisServer := miniredis.RunT(b)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer rdb.Close()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	authService := NewAuthService(nil, rdb, cfg, logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sessionID := uuid.New().String()
		_, err := authService.GenerateAuthToken(sessionID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthService_HashPassword(b *testing.B) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			BCryptCost: 10,
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	authService := NewAuthService(nil, nil, cfg, logger)

	password := "benchmark-password-123"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := authService.HashPassword(password)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ExampleAuthService_usage shows the setup shape used by an integration test.
func ExampleAuthService_usage() {
	// This would typically be in an integration test
	cfg := config.LoadFromEnv()

	// In real tests, you'd set up test database connections
	logger := logrus.New()

	// authService := NewAuthService(db, redis, cfg, logger)
	// token, _ := authService.GenerateAuthToken("session-123")
	// fmt.Printf("Generated token: %s", token[:8]+"...")

	_ = cfg
	_ = logger

}

func TestPairingChallengeCannotBeReplayed(t *testing.T) {
	cfg := config.LoadFromEnv()
	service := newTestAuthService(t, cfg)
	token, _, err := service.GenerateAuthChallenge("replay-test")
	require.NoError(t, err)
	require.NoError(t, service.LinkTokenToSession(token, "replay-test", uuid.New(), uuid.New(), 1))
	require.Error(t, service.LinkTokenToSession(token, "replay-test", uuid.New(), uuid.New(), 1))
}

// Registration stores lowercase usernames; every login path must accept the
// same spelling the player supplied at signup, without changing the password.
func TestAuthenticateRegisteredUsernameCase(t *testing.T) {
	cfg := config.LoadFromEnv()
	cfg.Auth.BCryptCost = 4
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL.ConnectionString())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}
	service := newTestAuthService(t, cfg)
	service.db = db
	username := "IcePic_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	password := " CaseSensitive-123 "
	user, character, err := service.RegisterPlayer(username, username+"@example.test", password, "Ice Pic")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, user.ID) })
	require.Equal(t, strings.ToLower(username), user.Username)

	for _, entered := range []string{username, strings.ToLower(username), strings.ToUpper(username), " " + username + " "} {
		t.Run(entered, func(t *testing.T) {
			loggedIn, err := service.AuthenticateUser(entered, password)
			require.NoError(t, err)
			require.Equal(t, user.ID, loggedIn.ID)
			require.Equal(t, user.Username, loggedIn.Username)

			// Complete the terminal pairing flow with the authenticated account.
			sessionID := uuid.NewString()
			token, _, err := service.GenerateAuthChallenge(sessionID)
			require.NoError(t, err)
			require.NoError(t, service.LinkTokenToSession(token, sessionID, loggedIn.ID, character.ID, loggedIn.AuthVersion))
			session, err := service.GetSession(sessionID)
			require.NoError(t, err)
			require.Equal(t, user.ID, session.UserID)
			require.Equal(t, character.ID, session.CharacterID)
		})
	}
	for _, wrong := range []string{strings.ToLower(password), strings.TrimSpace(password), "wrong-password"} {
		_, err := service.AuthenticateUser(username, wrong)
		require.EqualError(t, err, "invalid username or password")
	}
	_, _, err = service.RegisterPlayer(strings.ToUpper(username), "duplicate-"+username+"@example.test", password, "Other Name")
	require.Error(t, err)
	_, err = db.Exec(`UPDATE users SET is_active=false WHERE id=$1`, user.ID)
	require.NoError(t, err)
	_, err = service.AuthenticateUser(username, password)
	require.EqualError(t, err, "invalid username or password")
}

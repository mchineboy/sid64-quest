package auth

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

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
	
	// Use Redis test client (you might want to use miniredis for real tests)
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use a different DB for tests
	})
	
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	
	authService := NewAuthService(nil, rdb, cfg, logger)
	
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
	
	// Cleanup
	rdb.FlushDB(rdb.Context())
}

func TestAuthService_ValidateAuthToken_InvalidToken(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}
	
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	authService := NewAuthService(nil, rdb, cfg, logger)
	
	// Test with invalid token
	_, err := authService.ValidateAuthToken("invalid-token")
	
	// Should return error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired token")
	
	// Cleanup
	rdb.FlushDB(rdb.Context())
}

func TestAuthService_MarkTokenUsed(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			TokenExpiry: 5 * time.Minute,
		},
	}
	
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	authService := NewAuthService(nil, rdb, cfg, logger)
	
	// Generate a token
	sessionID := "test-session-456"
	token, err := authService.GenerateAuthToken(sessionID)
	require.NoError(t, err)
	
	// Mark token as used
	err = authService.MarkTokenUsed(token)
	require.NoError(t, err)
	
	// Try to validate the used token - should still work initially
	retrievedSessionID, err := authService.ValidateAuthToken(token)
	require.NoError(t, err)
	assert.Equal(t, sessionID, retrievedSessionID)
	
	// Cleanup
	rdb.FlushDB(rdb.Context())
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
	
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})
	defer rdb.FlushDB(rdb.Context())
	
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

// Example test showing how to test with real database
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
	
	// Output: (This is just an example)
}
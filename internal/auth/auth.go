package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

var pairingEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// AuthService handles authentication and session management
type AuthService struct {
	db     *sql.DB
	readTx *sql.Tx
	redis  *redis.Client
	config *config.Config
	logger *logrus.Logger
}

// WithReadTransaction keeps terminal authorization and character selection in
// the core's command transaction without needing another pooled connection.
func (as *AuthService) WithReadTransaction(tx *sql.Tx) *AuthService {
	copy := *as
	copy.readTx = tx
	return &copy
}

// NewAuthService creates a new authentication service
func NewAuthService(db *sql.DB, redis *redis.Client, cfg *config.Config, logger *logrus.Logger) *AuthService {
	return &AuthService{
		db:     db,
		redis:  redis,
		config: cfg,
		logger: logger,
	}
}

// GenerateAuthToken generates a one-time authentication token for a telnet session
func (as *AuthService) GenerateAuthToken(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session ID is required")
	}

	// Generate a random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	token := hex.EncodeToString(tokenBytes)

	// Create the pending session before issuing the token. The gateway uses its
	// connection ID as the session ID, so the browser can later attach an
	// authenticated user to that same connection.
	session := models.Session{
		ID:           sessionID,
		ConnectionID: sessionID,
		LastActivity: time.Now(),
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	sessionKey := fmt.Sprintf("session:%s", sessionID)
	if err := as.redis.Set(context.Background(), sessionKey, sessionData, as.config.Auth.SessionExpiry).Err(); err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}

	// Store the one-time token in Redis with expiration.
	authToken := models.AuthToken{
		Token:     token,
		SessionID: sessionID,
		ExpiresAt: time.Now().Add(as.config.Auth.TokenExpiry),
		Used:      false,
	}

	tokenData, err := json.Marshal(authToken)
	if err != nil {
		as.redis.Del(context.Background(), sessionKey)
		return "", fmt.Errorf("failed to marshal auth token: %w", err)
	}

	key := fmt.Sprintf("auth_token:%s", token)
	if err := as.redis.Set(context.Background(), key, tokenData, as.config.Auth.TokenExpiry).Err(); err != nil {
		as.redis.Del(context.Background(), sessionKey)
		return "", fmt.Errorf("failed to store auth token: %w", err)
	}

	as.logger.WithFields(logrus.Fields{
		"token":      token[:8] + "...", // Log only first 8 chars for security
		"session_id": sessionID,
		"expires_at": authToken.ExpiresAt,
	}).Debug("Generated auth token")

	return token, nil
}

// GenerateAuthChallenge creates the internal authentication token plus a short
// one-time code that can be typed from a phone or encoded in a small QR code.
func (as *AuthService) GenerateAuthChallenge(sessionID string) (string, string, error) {
	token, err := as.GenerateAuthToken(sessionID)
	if err != nil {
		return "", "", err
	}

	for attempt := 0; attempt < 5; attempt++ {
		random := make([]byte, 5)
		if _, err := rand.Read(random); err != nil {
			return "", "", fmt.Errorf("generate pairing code: %w", err)
		}
		rawCode := pairingEncoding.EncodeToString(random)
		key := "pairing_code:" + rawCode
		stored, err := as.redis.SetNX(context.Background(), key, token, as.config.Auth.TokenExpiry).Result()
		if err != nil {
			return "", "", fmt.Errorf("store pairing code: %w", err)
		}
		if stored {
			return token, formatPairingCode(rawCode), nil
		}
	}

	return "", "", fmt.Errorf("could not allocate a unique pairing code")
}

// ResolvePairingCode returns the internal token for a short one-time code.
func (as *AuthService) ResolvePairingCode(code string) (string, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(normalized) != 8 {
		return "", fmt.Errorf("invalid pairing code")
	}

	token, err := as.redis.Get(context.Background(), "pairing_code:"+normalized).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("invalid or expired pairing code")
		}
		return "", fmt.Errorf("retrieve pairing code: %w", err)
	}
	if _, err := as.ValidateAuthToken(token); err != nil {
		return "", err
	}
	return token, nil
}

func formatPairingCode(code string) string {
	if len(code) != 8 {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// ValidateAuthToken validates an authentication token and returns the session ID
func (as *AuthService) ValidateAuthToken(token string) (string, error) {
	key := fmt.Sprintf("auth_token:%s", token)

	tokenData, err := as.redis.Get(context.Background(), key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("invalid or expired token")
		}
		return "", fmt.Errorf("failed to retrieve auth token: %w", err)
	}

	var authToken models.AuthToken
	if err := json.Unmarshal([]byte(tokenData), &authToken); err != nil {
		return "", fmt.Errorf("failed to unmarshal auth token: %w", err)
	}

	// Check if token is expired
	if time.Now().After(authToken.ExpiresAt) {
		as.redis.Del(context.Background(), key)
		return "", fmt.Errorf("token expired")
	}

	// Check if token has already been used
	if authToken.Used {
		return "", fmt.Errorf("token already used")
	}

	return authToken.SessionID, nil
}

// MarkTokenUsed marks an authentication token as used
func (as *AuthService) MarkTokenUsed(token string) error {
	key := fmt.Sprintf("auth_token:%s", token)

	tokenData, err := as.redis.Get(context.Background(), key).Result()
	if err != nil {
		return fmt.Errorf("failed to retrieve auth token: %w", err)
	}

	var authToken models.AuthToken
	if err := json.Unmarshal([]byte(tokenData), &authToken); err != nil {
		return fmt.Errorf("failed to unmarshal auth token: %w", err)
	}

	authToken.Used = true

	updatedData, err := json.Marshal(authToken)
	if err != nil {
		return fmt.Errorf("failed to marshal updated auth token: %w", err)
	}

	// Update the token with remaining TTL
	ttl := as.redis.TTL(context.Background(), key).Val()
	if err := as.redis.Set(context.Background(), key, updatedData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to update auth token: %w", err)
	}

	return nil
}

// AuthenticateUser authenticates a user with username and password
func (as *AuthService) AuthenticateUser(username, password string) (*models.User, error) {
	query := `
		SELECT id, username, email, password_hash, created_at, last_login, is_active, permissions
		FROM users 
		WHERE username = $1 AND is_active = true
	`

	var user models.User
	var lastLogin sql.NullTime
	var permissionsJSON []byte

	err := as.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&lastLogin,
		&user.IsActive,
		&permissionsJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid username or password")
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	// Parse permissions
	if len(permissionsJSON) > 0 {
		if err := json.Unmarshal(permissionsJSON, &user.Permissions); err != nil {
			as.logger.WithError(err).Warn("Failed to unmarshal user permissions")
			user.Permissions = make(map[string]interface{})
		}
	} else {
		user.Permissions = make(map[string]interface{})
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	// Update last login time
	if err := as.updateLastLogin(user.ID); err != nil {
		as.logger.WithError(err).Warn("Failed to update last login time")
	}

	as.logger.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"username": user.Username,
	}).Info("User authenticated successfully")

	return &user, nil
}

// CreateSession creates a new session for an authenticated user
func (as *AuthService) CreateSession(userID uuid.UUID, characterID uuid.UUID, connectionID string) (*models.Session, error) {
	sessionID := uuid.New().String()

	session := models.Session{
		ID:           sessionID,
		CharacterID:  characterID,
		UserID:       userID,
		ConnectionID: connectionID,
		LastActivity: time.Now(),
		AuthToken:    "", // Will be set when linking with auth token
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("session:%s", sessionID)
	if err := as.redis.Set(context.Background(), key, sessionData, as.config.Auth.SessionExpiry).Err(); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	as.logger.WithFields(logrus.Fields{
		"session_id":    sessionID,
		"user_id":       userID,
		"character_id":  characterID,
		"connection_id": connectionID,
	}).Debug("Created session")

	return &session, nil
}

// GetSession retrieves a session by ID
func (as *AuthService) GetSession(sessionID string) (*models.Session, error) {
	key := fmt.Sprintf("session:%s", sessionID)

	sessionData, err := as.redis.Get(context.Background(), key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	var session models.Session
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// UpdateSessionActivity updates the last activity time for a session
func (as *AuthService) UpdateSessionActivity(sessionID string) error {
	session, err := as.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.LastActivity = time.Now()

	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("session:%s", sessionID)
	ttl := as.redis.TTL(context.Background(), key).Val()
	if err := as.redis.Set(context.Background(), key, sessionData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// LinkTokenToSession links an authentication token to a session after successful auth
func (as *AuthService) LinkTokenToSession(token, sessionID string, userID, characterID uuid.UUID) error {
	// Consume the challenge and attach identity atomically. A replay or a
	// disconnected terminal must never authenticate another session.
	const script = `
 local raw=redis.call('GET',KEYS[1]); local current=redis.call('GET',KEYS[2]);
 if not raw or not current then return 0 end;
 local challenge=cjson.decode(raw); local session=cjson.decode(current);
 if challenge.used or challenge.session_id~=ARGV[1] then return 0 end;
 challenge.used=true; session.user_id=ARGV[2]; session.character_id=ARGV[3];
 session.auth_token=ARGV[4]; session.last_activity=ARGV[5];
 local ttl=redis.call('PTTL',KEYS[1]); if ttl<=0 then return 0 end;
 redis.call('SET',KEYS[1],cjson.encode(challenge),'PX',ttl);
 redis.call('SET',KEYS[2],cjson.encode(session),'EX',ARGV[6]); return 1;`
	n, err := as.redis.Eval(context.Background(), script, []string{"auth_token:" + token, "session:" + sessionID}, sessionID, userID.String(), characterID.String(), token, time.Now().Format(time.RFC3339Nano), int(as.config.Auth.SessionExpiry.Seconds())).Int()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("invalid, expired or already used pairing challenge")
	}
	return nil
}

// DeleteSession removes a session
func (as *AuthService) DeleteSession(sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	if err := as.redis.Del(context.Background(), key).Err(); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	as.logger.WithField("session_id", sessionID).Debug("Deleted session")
	return nil
}

// GetUserCharacters retrieves all characters for a user
func (as *AuthService) GetUserCharacters(userID uuid.UUID) ([]*models.Character, error) {
	query := `
		SELECT id, user_id, name, level, experience, health, max_health, 
		       stamina, max_stamina, gold, alignment_lawful, alignment_good,
		       current_room_id, last_rest, is_sleeping, created_at
		FROM characters 
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	var rows *sql.Rows
	var err error
	if as.readTx != nil {
		rows, err = as.readTx.Query(query, userID)
	} else {
		rows, err = as.db.Query(query, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query characters: %w", err)
	}
	defer rows.Close()

	var characters []*models.Character

	for rows.Next() {
		var char models.Character
		var currentRoomID sql.NullString

		err := rows.Scan(
			&char.ID,
			&char.UserID,
			&char.Name,
			&char.Level,
			&char.Experience,
			&char.Health,
			&char.MaxHealth,
			&char.Stamina,
			&char.MaxStamina,
			&char.Gold,
			&char.AlignmentLawful,
			&char.AlignmentGood,
			&currentRoomID,
			&char.LastRest,
			&char.IsSleeping,
			&char.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan character: %w", err)
		}

		if currentRoomID.Valid {
			roomID, err := uuid.Parse(currentRoomID.String)
			if err == nil {
				char.CurrentRoomID = &roomID
			}
		}

		characters = append(characters, &char)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating characters: %w", err)
	}

	return characters, nil
}

// CreateUser creates a new user account
func (as *AuthService) CreateUser(username, email, password string) (*models.User, error) {
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), as.config.Auth.BCryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create the user
	query := `
		INSERT INTO users (username, email, password_hash, permissions)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	permissions := map[string]interface{}{
		"player": true,
	}
	permissionsJSON, _ := json.Marshal(permissions)

	var user models.User
	err = as.db.QueryRow(query, username, email, string(hashedPassword), permissionsJSON).Scan(
		&user.ID,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user.Username = username
	user.Email = email
	user.IsActive = true
	user.Permissions = permissions

	as.logger.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"username": username,
		"email":    email,
	}).Info("Created new user")

	return &user, nil
}

// RegisterPlayer creates an account and its first character as one database
// transaction. New characters begin in Town Square so they can enter the
// playable starter world immediately.
func (as *AuthService) RegisterPlayer(username, email, password, characterName string) (*models.User, *models.Character, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.TrimSpace(email)
	characterName = strings.TrimSpace(characterName)
	if len(username) < 3 || len(username) > 20 {
		return nil, nil, fmt.Errorf("username must be between 3 and 20 characters")
	}
	if len(characterName) < 3 || len(characterName) > 30 {
		return nil, nil, fmt.Errorf("character name must be between 3 and 30 characters")
	}
	if !strings.Contains(email, "@") {
		return nil, nil, fmt.Errorf("a valid email address is required")
	}
	if len(password) < 8 {
		return nil, nil, fmt.Errorf("password must be at least 8 characters")
	}

	for _, character := range username {
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_') {
			return nil, nil, fmt.Errorf("username can only contain letters, numbers, and underscores")
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), as.config.Auth.BCryptCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := as.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	permissions, _ := json.Marshal(map[string]bool{"player": true})
	user := &models.User{Username: username, Email: email, IsActive: true, Permissions: map[string]interface{}{"player": true}}
	if err := tx.QueryRow(`
		INSERT INTO users (username, email, password_hash, permissions)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`, username, email, string(hash), permissions).Scan(&user.ID, &user.CreatedAt); err != nil {
		return nil, nil, fmt.Errorf("create account: %w", err)
	}

	var startingRoomID uuid.UUID
	if err := tx.QueryRow(`SELECT id FROM rooms WHERE name = 'Town Square' ORDER BY created_at LIMIT 1`).Scan(&startingRoomID); err != nil {
		return nil, nil, fmt.Errorf("find starting room: %w", err)
	}

	character := &models.Character{}
	if err := tx.QueryRow(`
		INSERT INTO characters (user_id, name, gold, current_room_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, level, experience, health, max_health, stamina, max_stamina, gold, alignment_lawful, alignment_good, current_room_id, last_rest, is_sleeping, created_at`,
		user.ID, characterName, as.config.Game.StartingGold, startingRoomID).Scan(
		&character.ID, &character.UserID, &character.Name, &character.Level, &character.Experience,
		&character.Health, &character.MaxHealth, &character.Stamina, &character.MaxStamina,
		&character.Gold, &character.AlignmentLawful, &character.AlignmentGood, &character.CurrentRoomID,
		&character.LastRest, &character.IsSleeping, &character.CreatedAt,
	); err != nil {
		return nil, nil, fmt.Errorf("create character: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit registration: %w", err)
	}
	return user, character, nil
}

// HashPassword hashes a password using bcrypt
func (as *AuthService) HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), as.config.Auth.BCryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedPassword), nil
}

// updateLastLogin updates the last login time for a user
func (as *AuthService) updateLastLogin(userID uuid.UUID) error {
	query := `UPDATE users SET last_login = NOW() WHERE id = $1`
	_, err := as.db.Exec(query, userID)
	return err
}

// CleanupExpiredTokens removes expired authentication tokens
func (as *AuthService) CleanupExpiredTokens() error {
	// Redis handles TTL automatically, but we can implement additional cleanup if needed
	as.logger.Debug("Cleaned up expired tokens")
	return nil
}

// CleanupExpiredSessions removes expired sessions
func (as *AuthService) CleanupExpiredSessions() error {
	// Redis handles TTL automatically, but we can implement additional cleanup if needed
	as.logger.Debug("Cleaned up expired sessions")
	return nil
}

// GetAuthURL generates the authentication URL for a token
func (as *AuthService) GetAuthURL(token string) string {
	return fmt.Sprintf("%s/auth?token=%s", as.config.Auth.BaseURL, token)
}

// GetPairingURL returns the short browser URL displayed by terminal clients.
func (as *AuthService) GetPairingURL(code string) string {
	return fmt.Sprintf("%s/p/%s", strings.TrimRight(as.config.Auth.BaseURL, "/"), code)
}

// GetPairingEntryURL returns the fixed page where a player can type the code.
func (as *AuthService) GetPairingEntryURL() string {
	return strings.TrimRight(as.config.Auth.BaseURL, "/") + "/pair"
}

// UserActive rechecks operator disabling for already-connected players.
func (as *AuthService) UserActive(ctx context.Context, id uuid.UUID) (bool, error) {
	var active bool
	var row *sql.Row
	if as.readTx != nil {
		row = as.readTx.QueryRowContext(ctx, `SELECT is_active FROM users WHERE id=$1`, id)
	} else {
		row = as.db.QueryRowContext(ctx, `SELECT is_active FROM users WHERE id=$1`, id)
	}
	err := row.Scan(&active)
	return active, err
}

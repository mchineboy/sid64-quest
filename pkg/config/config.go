package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the MUD system
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Auth     AuthConfig     `yaml:"auth"`
	Discord  DiscordConfig  `yaml:"discord"`
	Game     GameConfig     `yaml:"game"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	TelnetPort     int    `yaml:"telnet_port"`
	PETSCIIPort    int    `yaml:"petscii_port"`
	HTTPPort       int    `yaml:"http_port"`
	Host           string `yaml:"host"`
	ReadTimeout    int    `yaml:"read_timeout"`
	WriteTimeout   int    `yaml:"write_timeout"`
	MaxConnections int    `yaml:"max_connections"`
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	PostgreSQL PostgreSQLConfig `yaml:"postgresql"`
	MongoDB    MongoDBConfig    `yaml:"mongodb"`
}

// PostgreSQLConfig holds PostgreSQL-specific configuration
type PostgreSQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`
	MaxConns int    `yaml:"max_connections"`
}

// MongoDBConfig holds MongoDB-specific configuration
type MongoDBConfig struct {
	URI      string `yaml:"uri"`
	Database string `yaml:"database"`
	Timeout  int    `yaml:"timeout"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	TokenExpiry   time.Duration `yaml:"token_expiry"`
	SessionExpiry time.Duration `yaml:"session_expiry"`
	BaseURL       string        `yaml:"base_url"`
	SecretKey     string        `yaml:"secret_key"`
	BCryptCost    int           `yaml:"bcrypt_cost"`
}

// DiscordConfig holds Discord bot configuration
type DiscordConfig struct {
	Token          string `yaml:"token"`
	GuildID        string `yaml:"guild_id"`
	AdminChannelID string `yaml:"admin_channel_id"`
	LogChannelID   string `yaml:"log_channel_id"`
	ErrorChannelID string `yaml:"error_channel_id"`
}

// GameConfig holds game-specific configuration
type GameConfig struct {
	TickRate         int           `yaml:"tick_rate"`
	MaxPlayers       int           `yaml:"max_players"`
	RestInterval     time.Duration `yaml:"rest_interval"`
	StaminaDrain     int           `yaml:"stamina_drain"`
	DeathPenalty     int           `yaml:"death_penalty"`
	MaxInventorySize int           `yaml:"max_inventory_size"`
	StartingGold     int           `yaml:"starting_gold"`
	StartingHealth   int           `yaml:"starting_health"`
	StartingStamina  int           `yaml:"starting_stamina"`
}

// LoadFromEnv loads configuration from environment variables with defaults
func LoadFromEnv() *Config {
	return &Config{
		Server: ServerConfig{
			TelnetPort:     getEnvInt("TELNET_PORT", 2323),
			PETSCIIPort:    getEnvInt("PETSCII_PORT", 6464),
			HTTPPort:       getEnvInt("HTTP_PORT", 8080),
			Host:           getEnvString("HOST", "0.0.0.0"),
			ReadTimeout:    getEnvInt("READ_TIMEOUT", 30),
			WriteTimeout:   getEnvInt("WRITE_TIMEOUT", 30),
			MaxConnections: getEnvInt("MAX_CONNECTIONS", 1000),
		},
		Database: DatabaseConfig{
			PostgreSQL: PostgreSQLConfig{
				Host:     getEnvString("POSTGRES_HOST", "localhost"),
				Port:     getEnvInt("POSTGRES_PORT", 5432),
				Database: getEnvString("POSTGRES_DB", "race_condition_kingdom"),
				Username: getEnvString("POSTGRES_USER", "mud_user"),
				Password: getEnvString("POSTGRES_PASSWORD", ""),
				SSLMode:  getEnvString("POSTGRES_SSL_MODE", "disable"),
				MaxConns: getEnvInt("POSTGRES_MAX_CONNS", 25),
			},
			MongoDB: MongoDBConfig{
				URI:      getEnvString("MONGODB_URI", "mongodb://localhost:27017"),
				Database: getEnvString("MONGODB_DB", "race_condition_kingdom_logs"),
				Timeout:  getEnvInt("MONGODB_TIMEOUT", 10),
			},
		},
		Redis: RedisConfig{
			Host:     getEnvString("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnvString("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			PoolSize: getEnvInt("REDIS_POOL_SIZE", 10),
		},
		Auth: AuthConfig{
			TokenExpiry:   time.Duration(getEnvInt("AUTH_TOKEN_EXPIRY", 300)) * time.Second,
			SessionExpiry: time.Duration(getEnvInt("AUTH_SESSION_EXPIRY", 86400)) * time.Second,
			BaseURL:       getEnvString("AUTH_BASE_URL", "http://localhost:8080"),
			SecretKey:     getEnvString("AUTH_SECRET_KEY", "change-me-in-production"),
			BCryptCost:    getEnvInt("BCRYPT_COST", 12),
		},
		Discord: DiscordConfig{
			Token:          getEnvString("DISCORD_TOKEN", ""),
			GuildID:        getEnvString("DISCORD_GUILD_ID", ""),
			AdminChannelID: getEnvString("DISCORD_ADMIN_CHANNEL", ""),
			LogChannelID:   getEnvString("DISCORD_LOG_CHANNEL", ""),
			ErrorChannelID: getEnvString("DISCORD_ERROR_CHANNEL", ""),
		},
		Game: GameConfig{
			TickRate:         getEnvInt("GAME_TICK_RATE", 60),
			MaxPlayers:       getEnvInt("GAME_MAX_PLAYERS", 500),
			RestInterval:     time.Duration(getEnvInt("GAME_REST_INTERVAL", 3600)) * time.Second,
			StaminaDrain:     getEnvInt("GAME_STAMINA_DRAIN", 1),
			DeathPenalty:     getEnvInt("GAME_DEATH_PENALTY", 100),
			MaxInventorySize: getEnvInt("GAME_MAX_INVENTORY", 50),
			StartingGold:     getEnvInt("GAME_STARTING_GOLD", 100),
			StartingHealth:   getEnvInt("GAME_STARTING_HEALTH", 100),
			StartingStamina:  getEnvInt("GAME_STARTING_STAMINA", 100),
		},
	}
}

// PostgreSQLConnectionString returns a formatted PostgreSQL connection string
func (c *PostgreSQLConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode)
}

// RedisAddress returns a formatted Redis address
func (c *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

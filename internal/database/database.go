package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	_ "github.com/lib/pq"

	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

// Database represents our hybrid database system
type Database struct {
	PostgreSQL *sql.DB
	Redis      *redis.Client
	MongoDB    *mongo.Client
	Config     *config.Config
	Logger     *logrus.Logger
}

// New creates a new database instance with all connections
func New(cfg *config.Config, logger *logrus.Logger) (*Database, error) {
	db := &Database{
		Config: cfg,
		Logger: logger,
	}

	// Initialize PostgreSQL
	if err := db.initPostgreSQL(); err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	// Initialize Redis
	if err := db.initRedis(); err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Initialize MongoDB
	if err := db.initMongoDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

	return db, nil
}

// initPostgreSQL initializes the PostgreSQL connection
func (d *Database) initPostgreSQL() error {
	connStr := d.Config.Database.PostgreSQL.ConnectionString()
	
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(d.Config.Database.PostgreSQL.MaxConns)
	db.SetMaxIdleConns(d.Config.Database.PostgreSQL.MaxConns / 2)
	db.SetConnMaxLifetime(time.Hour)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	d.PostgreSQL = db
	d.Logger.Info("PostgreSQL connection established")
	return nil
}

// initRedis initializes the Redis connection
func (d *Database) initRedis() error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     d.Config.Redis.Address(),
		Password: d.Config.Redis.Password,
		DB:       d.Config.Redis.DB,
		PoolSize: d.Config.Redis.PoolSize,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping Redis: %w", err)
	}

	d.Redis = rdb
	d.Logger.Info("Redis connection established")
	return nil
}

// initMongoDB initializes the MongoDB connection
func (d *Database) initMongoDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 
		time.Duration(d.Config.Database.MongoDB.Timeout)*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(d.Config.Database.MongoDB.URI))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test the connection
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	d.MongoDB = client
	d.Logger.Info("MongoDB connection established")
	return nil
}

// Close closes all database connections
func (d *Database) Close() error {
	var errors []error

	// Close PostgreSQL
	if d.PostgreSQL != nil {
		if err := d.PostgreSQL.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close PostgreSQL: %w", err))
		}
	}

	// Close Redis
	if d.Redis != nil {
		if err := d.Redis.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close Redis: %w", err))
		}
	}

	// Close MongoDB
	if d.MongoDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.MongoDB.Disconnect(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to close MongoDB: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing databases: %v", errors)
	}

	d.Logger.Info("All database connections closed")
	return nil
}

// Health checks the health of all database connections
func (d *Database) Health(ctx context.Context) map[string]error {
	health := make(map[string]error)

	// Check PostgreSQL
	if d.PostgreSQL != nil {
		health["postgresql"] = d.PostgreSQL.PingContext(ctx)
	}

	// Check Redis
	if d.Redis != nil {
		health["redis"] = d.Redis.Ping(ctx).Err()
	}

	// Check MongoDB
	if d.MongoDB != nil {
		health["mongodb"] = d.MongoDB.Ping(ctx, nil)
	}

	return health
}

// GetPostgreSQLDB returns the PostgreSQL database connection
func (d *Database) GetPostgreSQLDB() *sql.DB {
	return d.PostgreSQL
}

// GetRedisClient returns the Redis client
func (d *Database) GetRedisClient() *redis.Client {
	return d.Redis
}

// GetMongoDatabase returns the MongoDB database
func (d *Database) GetMongoDatabase() *mongo.Database {
	return d.MongoDB.Database(d.Config.Database.MongoDB.Database)
}

// Transaction executes a function within a PostgreSQL transaction
func (d *Database) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.PostgreSQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return err
}
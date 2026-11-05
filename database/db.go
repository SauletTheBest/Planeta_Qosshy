package database

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect() {
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not loaded, falling back to environment variables")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	var db *gorm.DB
	var err error

	// GORM's default logger printed whole SQL with the values filled in, e.g.
	//   SELECT * FROM users WHERE verification_token = 'secret-from-the-email'
	// so secrets from email links ended up in the log. Now it prints "$1" instead,
	// and "no such row" (a normal answer, not a failure) is not logged at all.
	gormLogger := logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
		SlowThreshold:             500 * time.Millisecond, // still warn about slow queries
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false, // colour codes are garbage in Docker logs
	})

	maxRetries := 10
	for i := 1; i <= maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
		if err == nil {
			log.Println("Successfully connected to PostgreSQL database!")
			break
		}
		log.Printf("Failed to connect to database (attempt %d/%d): %v", i, maxRetries, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to database after %d attempts: %v", maxRetries, err)
	}

	// The schema is owned by versioned SQL files in migrations/, not by AutoMigrate:
	// AutoMigrate never drops or renames columns and leaves no history of what changed.
	// Before, a failed migration only logged and left DB nil, so the first request panicked.
	if err := runMigrations(dsn); err != nil {
		log.Fatalf("Database migration failed: %v", err)
	}

	// Connection pool. Without limits a traffic spike opens connection after connection
	// until Postgres refuses with "too many clients" (its max_connections is 100 by default).
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("database pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(intEnv("DB_MAX_OPEN_CONNS", 20))
	sqlDB.SetMaxIdleConns(10)                  // kept open between requests, ready to use
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // recycled now and then (e.g. after a DB failover)

	DB = db
}

// Migrate brings the schema at dsn up to date. Used by the integration tests,
// which run against their own throw-away database.
func Migrate(dsn string) error { return runMigrations(dsn) }

// Ping checks that the database answers, for the /readyz health check.
func Ping(ctx context.Context) error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the pool at shutdown, so Postgres doesn't wait for dead connections.
func Close() {
	if sqlDB, err := DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func intEnv(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

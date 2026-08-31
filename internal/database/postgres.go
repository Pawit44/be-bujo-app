// Package database opens the Postgres connection and runs migrations.
package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"bujo/internal/models"
)

// Connect opens the Postgres connection, tunes the pool, and runs
// migrations. It retries briefly on startup since the database container
// (in local/dev setups using docker-compose) can still be coming up when
// the app starts.
func Connect(databaseURL string) *gorm.DB {
	var db *gorm.DB
	var err error

	const maxAttempts = 10
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		db, err = gorm.Open(postgres.Open(databaseURL), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err == nil {
			break
		}
		log.Printf("database: connect attempt %d/%d failed: %v", attempt, maxAttempts, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("database: could not connect after %d attempts: %v", maxAttempts, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("database: could not access underlying connection pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := db.AutoMigrate(&models.User{}, &models.Session{}, &models.Collection{}, &models.Entry{}); err != nil {
		log.Fatalf("database: migration failed: %v", err)
	}

	return db
}

// Ping is used by health checks that want to verify the database, not just
// that the process is up.
func Ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return sqlDB.Ping()
}

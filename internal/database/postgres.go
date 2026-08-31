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
func Connect(databaseURL string, autoMigrate bool) *gorm.DB {
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
	// Keep the whole pool warm rather than only a fifth of it. Against a
	// managed database a region away, opening a connection costs a TCP plus
	// TLS handshake — several times the cost of the query it is about to
	// run — so idle connections that get reused are the difference between a
	// fast request and a slow one. Idle conns are capped to max-open so a
	// burst never has to dial mid-request.
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if autoMigrate {
		// Worth timing: this is by far the slowest thing the process does at
		// startup, and the log line is what tells you whether turning it off
		// in production is worth doing.
		started := time.Now()
		if err := db.AutoMigrate(&models.User{}, &models.Session{}, &models.Collection{}, &models.Folder{}, &models.Entry{}); err != nil {
			log.Fatalf("database: migration failed: %v", err)
		}
		log.Printf("database: schema reconciled in %v (set DB_AUTO_MIGRATE=false to skip once the schema is settled)", time.Since(started).Round(time.Millisecond))

		ensureIndexes(db)
	}

	return db
}

// ensureIndexes adds the composite indexes the read paths actually filter on.
// GORM's struct tags give one index per column, but every hot query narrows on
// several at once (owner + which spread + which month), and Postgres can only
// use one single-column index per scan — so those queries were reading far
// more rows than they returned. These are created concurrently and are
// idempotent, so a redeploy is a no-op and a first deploy never blocks writes.
func ensureIndexes(db *gorm.DB) {
	statements := []string{
		// Future/monthly log reads and the index page's per-month counts.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_entries_user_kind_month
			ON entries (user_id, log_kind, month) WHERE deleted_at IS NULL`,
		// Weekly log reads, which scan a date range inside one user's rows.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_entries_user_kind_date
			ON entries (user_id, log_kind, date) WHERE deleted_at IS NULL`,
		// Collection spreads.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_entries_user_collection
			ON entries (user_id, collection_id) WHERE deleted_at IS NULL`,
		// The index page's "recent activity" list.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_entries_user_updated
			ON entries (user_id, updated_at DESC) WHERE deleted_at IS NULL`,
		// Sidebar collection list.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_collections_user_order
			ON collections (user_id, pinned DESC, position, id) WHERE deleted_at IS NULL`,
		// Session lookup on every authenticated request.
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_token_hash
			ON sessions (token_hash)`,
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			// Not fatal: the app is correct without these, just slower, and a
			// managed database may refuse DDL to the application role.
			log.Printf("database: index setup skipped (%v)", err)
		}
	}
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

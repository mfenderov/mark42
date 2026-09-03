package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"

	// Import migrations to register them with goose
	_ "github.com/mfenderov/mark42/internal/storage/migrations"
)

func init() {
	// Set goose to use our custom table name and dialect
	goose.SetTableName("goose_db_version")
	_ = goose.SetDialect("sqlite3")
}

// gooseDir returns an empty dir so goose's dir-based SQL collection finds nothing.
// All migrations are Go migrations registered in code, never .sql files.
func gooseDir() (string, func()) {
	dir, err := os.MkdirTemp("", "mark42-goose-*")
	if err != nil {
		return ".", func() {}
	}
	return dir, func() { os.RemoveAll(dir) }
}

// Migrate runs all pending migrations using goose.
func (s *Store) Migrate() error {
	// Get the underlying *sql.DB for goose
	db := s.db.DB

	// Set logger
	goose.SetLogger(goose.NopLogger())

	// Run migrations
	dir, cleanup := gooseDir()
	defer cleanup()
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose migration failed: %w", err)
	}

	return nil
}

// MigrateWithLogging runs migrations with logging enabled.
func (s *Store) MigrateWithLogging() error {
	db := s.db.DB

	goose.SetLogger(log.Default())

	dir, cleanup := gooseDir()
	defer cleanup()
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose migration failed: %w", err)
	}

	return nil
}

// GetSchemaVersion returns the current schema version.
func (s *Store) GetSchemaVersion() (int64, error) {
	db := s.db.DB
	version, err := goose.GetDBVersion(db)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}
	return version, nil
}

// MigrateDown rolls back the last migration.
func (s *Store) MigrateDown() error {
	db := s.db.DB

	dir, cleanup := gooseDir()
	defer cleanup()
	if err := goose.Down(db, dir); err != nil {
		return fmt.Errorf("goose rollback failed: %w", err)
	}

	return nil
}

// MigrateTo migrates to a specific version.
func (s *Store) MigrateTo(version int64) error {
	db := s.db.DB

	current, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if version > current {
		dir, cleanup := gooseDir()
		defer cleanup()
		if err := goose.UpTo(db, dir, version); err != nil {
			return fmt.Errorf("goose migrate up failed: %w", err)
		}
	} else if version < current {
		dir, cleanup := gooseDir()
		defer cleanup()
		if err := goose.DownTo(db, dir, version); err != nil {
			return fmt.Errorf("goose migrate down failed: %w", err)
		}
	}

	return nil
}

// MigrateStatus returns the status of all migrations.
func (s *Store) MigrateStatus() error {
	db := s.db.DB
	dir, cleanup := gooseDir()
	defer cleanup()
	return goose.Status(db, dir)
}

// RunMigrationFunc runs a custom migration function (for testing).
func RunMigrationFunc(db *sql.DB, up func(context.Context, *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := up(context.Background(), tx); err != nil {
		return err
	}

	return tx.Commit()
}

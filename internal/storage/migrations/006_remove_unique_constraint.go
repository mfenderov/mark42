package migrations

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upRemoveUniqueConstraint, downRemoveUniqueConstraint)
}

func upRemoveUniqueConstraint(ctx context.Context, tx *sql.Tx) error {
	// Check if entities table has UNIQUE constraint on name column
	// by looking at the table's CREATE statement
	var createSQL string
	err := tx.QueryRowContext(ctx, `
		SELECT sql FROM sqlite_master WHERE type='table' AND name='entities'
	`).Scan(&createSQL)
	if err != nil {
		return err
	}

	// If the schema already lacks UNIQUE constraint, skip this migration
	if !strings.Contains(strings.ToUpper(createSQL), "UNIQUE") {
		return nil
	}

	// SQLite doesn't support dropping constraints, so we recreate the table
	_, err = tx.ExecContext(ctx, `
		-- Create new table without UNIQUE constraint
		-- Note: supersedes_id has no FK constraint to avoid SQLite table rename issues
		CREATE TABLE IF NOT EXISTS entities_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			supersedes_id INTEGER,
			is_latest BOOLEAN DEFAULT 1,
			version INTEGER DEFAULT 1,
			container_tag TEXT
		);

		-- Copy existing data
		INSERT INTO entities_new (id, name, entity_type, created_at, supersedes_id, is_latest, version, container_tag)
		SELECT id, name, entity_type, created_at, supersedes_id, COALESCE(is_latest, 1), COALESCE(version, 1), container_tag
		FROM entities;

		-- Drop old table
		DROP TABLE entities;

		-- Rename new table
		ALTER TABLE entities_new RENAME TO entities;

		-- Recreate indexes
		CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
		CREATE INDEX IF NOT EXISTS idx_entities_latest ON entities(name, is_latest);
		CREATE INDEX IF NOT EXISTS idx_entities_container ON entities(container_tag);

		-- Recreate FTS triggers
		DROP TRIGGER IF EXISTS entities_ai;
		DROP TRIGGER IF EXISTS entities_ad;
		DROP TRIGGER IF EXISTS entities_au;

		CREATE TRIGGER entities_ai AFTER INSERT ON entities BEGIN
			INSERT INTO entities_fts(rowid, name, entity_type)
			VALUES (new.id, new.name, new.entity_type);
		END;

		CREATE TRIGGER entities_ad AFTER DELETE ON entities BEGIN
			INSERT INTO entities_fts(entities_fts, rowid, name, entity_type)
			VALUES('delete', old.id, old.name, old.entity_type);
		END;

		CREATE TRIGGER entities_au AFTER UPDATE ON entities BEGIN
			INSERT INTO entities_fts(entities_fts, rowid, name, entity_type)
			VALUES('delete', old.id, old.name, old.entity_type);
			INSERT INTO entities_fts(rowid, name, entity_type)
			VALUES (new.id, new.name, new.entity_type);
		END;
	`)
	return err
}

func downRemoveUniqueConstraint(ctx context.Context, tx *sql.Tx) error {
	// Reverting would require recreating with UNIQUE constraint
	// This is a one-way migration for versioning support
	return nil
}

package storage

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a database migration.
type Migration struct {
	Version     int
	Description string
	Up          func(*sql.DB) error
}

// migrations defines all schema migrations in order.
var migrations = []Migration{
	{
		Version:     1,
		Description: "Phase 2: Add fact_type to observations",
		Up: func(db *sql.DB) error {
			// Add fact_type column: 'static' (permanent), 'dynamic' (session), 'session_turn'
			_, err := db.Exec(`
				ALTER TABLE observations ADD COLUMN fact_type TEXT DEFAULT 'dynamic';
			`)
			return err
		},
	},
	{
		Version:     2,
		Description: "Phase 2: Add importance and decay to observations",
		Up: func(db *sql.DB) error {
			// Add importance score and TTL for memory decay
			// Note: SQLite doesn't allow non-constant defaults in ALTER TABLE
			// so last_accessed defaults to NULL (set on access)
			queries := []string{
				`ALTER TABLE observations ADD COLUMN importance REAL DEFAULT 1.0`,
				`ALTER TABLE observations ADD COLUMN forget_after TIMESTAMP`,
				`ALTER TABLE observations ADD COLUMN last_accessed TIMESTAMP`,
			}
			for _, q := range queries {
				if _, err := db.Exec(q); err != nil {
					// Column may already exist
					continue
				}
			}
			return nil
		},
	},
	{
		Version:     3,
		Description: "Phase 2: Add versioning to entities",
		Up: func(db *sql.DB) error {
			// Add version chain support
			_, err := db.Exec(`
				ALTER TABLE entities ADD COLUMN supersedes_id INTEGER REFERENCES entities(id);
				ALTER TABLE entities ADD COLUMN is_latest BOOLEAN DEFAULT 1;
				ALTER TABLE entities ADD COLUMN version INTEGER DEFAULT 1;
			`)
			return err
		},
	},
	{
		Version:     4,
		Description: "Phase 2: Add container_tag for multi-project scoping",
		Up: func(db *sql.DB) error {
			_, err := db.Exec(`
				ALTER TABLE entities ADD COLUMN container_tag TEXT;
				CREATE INDEX IF NOT EXISTS idx_entities_container ON entities(container_tag);
			`)
			return err
		},
	},
	{
		Version:     5,
		Description: "Phase 2: Add observation_embeddings table for vector search",
		Up: func(db *sql.DB) error {
			_, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS observation_embeddings (
					observation_id INTEGER PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
					embedding BLOB NOT NULL,
					model TEXT NOT NULL,
					dimensions INTEGER NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_embeddings_model ON observation_embeddings(model);
			`)
			return err
		},
	},
	{
		Version:     6,
		Description: "Phase 2.3: Remove UNIQUE constraint on entities.name for versioning",
		Up: func(db *sql.DB) error {
			// SQLite doesn't support dropping constraints, so we recreate the table
			_, err := db.Exec(`
				-- Create new table without UNIQUE constraint
				CREATE TABLE IF NOT EXISTS entities_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					entity_type TEXT NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					supersedes_id INTEGER REFERENCES entities_new(id),
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
		},
	},
}

// getCurrentVersion returns the current schema version.
func (s *Store) getCurrentVersion() (int, error) {
	// Check if schema_version table exists
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&count)
	if err != nil {
		return 0, err
	}

	if count == 0 {
		// Create schema_version table
		_, err := s.db.Exec(`
			CREATE TABLE schema_version (
				version INTEGER PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				description TEXT
			);
			INSERT INTO schema_version (version, description) VALUES (0, 'Initial schema');
		`)
		if err != nil {
			return 0, fmt.Errorf("failed to create schema_version table: %w", err)
		}
		return 0, nil
	}

	var version int
	err = s.db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Migrate runs all pending migrations.
func (s *Store) Migrate() error {
	currentVersion, err := s.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue // Already applied
		}

		log.Printf("Applying migration %d: %s", m.Version, m.Description)

		if err := m.Up(s.db.DB); err != nil {
			// Some ALTER TABLE statements may fail if column exists
			// This is OK for idempotent migrations
			log.Printf("Migration %d warning (may be already applied): %v", m.Version, err)
		}

		// Record migration
		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO schema_version (version, description)
			VALUES (?, ?)
		`, m.Version, m.Description)
		if err != nil {
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}
	}

	return nil
}

// GetSchemaVersion returns the current schema version.
func (s *Store) GetSchemaVersion() (int, error) {
	return s.getCurrentVersion()
}

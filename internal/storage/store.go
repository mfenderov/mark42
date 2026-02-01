package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store manages the SQLite database for memory storage.
type Store struct {
	db   *sql.DB
	path string
}

// NewStore creates a new Store, initializing the database and schema.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &Store{db: db, path: path}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// ListTables returns all table names in the database.
func (s *Store) ListTables() []string {
	rows, err := s.db.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' OR type='virtual table'
		ORDER BY name
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	return tables
}

func (s *Store) initSchema() error {
	schema := `
	-- Core entities table
	CREATE TABLE IF NOT EXISTS entities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		entity_type TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
	CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(entity_type);

	-- Observations attached to entities
	CREATE TABLE IF NOT EXISTS observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
		content TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(entity_id, content)
	);

	CREATE INDEX IF NOT EXISTS idx_observations_entity ON observations(entity_id);

	-- Relations between entities
	CREATE TABLE IF NOT EXISTS relations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
		to_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
		relation_type TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(from_entity_id, to_entity_id, relation_type)
	);

	CREATE INDEX IF NOT EXISTS idx_relations_from ON relations(from_entity_id);
	CREATE INDEX IF NOT EXISTS idx_relations_to ON relations(to_entity_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create base schema: %w", err)
	}

	// Create FTS5 virtual tables separately (they can't use IF NOT EXISTS)
	if err := s.initFTS(); err != nil {
		return err
	}

	return nil
}

func (s *Store) initFTS() error {
	// Check if FTS tables exist
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='observations_fts'
	`).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil // FTS already initialized
	}

	ftsSchema := `
	-- FTS5 index for observations
	CREATE VIRTUAL TABLE observations_fts USING fts5(
		content,
		content='observations',
		content_rowid='id',
		tokenize='porter unicode61'
	);

	-- FTS5 index for entity names
	CREATE VIRTUAL TABLE entities_fts USING fts5(
		name,
		entity_type,
		content='entities',
		content_rowid='id',
		tokenize='porter unicode61'
	);

	-- Triggers to keep FTS in sync with observations
	CREATE TRIGGER observations_ai AFTER INSERT ON observations BEGIN
		INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
	END;

	CREATE TRIGGER observations_ad AFTER DELETE ON observations BEGIN
		INSERT INTO observations_fts(observations_fts, rowid, content)
		VALUES('delete', old.id, old.content);
	END;

	CREATE TRIGGER observations_au AFTER UPDATE ON observations BEGIN
		INSERT INTO observations_fts(observations_fts, rowid, content)
		VALUES('delete', old.id, old.content);
		INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
	END;

	-- Triggers to keep FTS in sync with entities
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
	`

	if _, err := s.db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("failed to create FTS schema: %w", err)
	}

	return nil
}

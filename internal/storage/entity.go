package storage

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when an entity is not found.
var ErrNotFound = errors.New("not found")

// Entity represents a node in the knowledge graph.
type Entity struct {
	ID           int64
	Name         string
	Type         string
	Observations []string
	CreatedAt    time.Time
	// Versioning fields (Phase 2.3)
	Version      int
	IsLatest     bool
	SupersedesID int64 // ID of previous version (0 if none)
}

// ErrEntityExists is returned when attempting to create an entity that already exists.
var ErrEntityExists = errors.New("entity already exists")

// CreateEntity creates a new entity with optional observations.
// Returns ErrEntityExists if an entity with this name already exists.
func (s *Store) CreateEntity(name, entityType string, observations []string) (*Entity, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if entity already exists (no UNIQUE constraint, must check manually)
	var existingID int64
	err = tx.QueryRow("SELECT id FROM entities WHERE name = ?", name).Scan(&existingID)
	if err == nil {
		return nil, ErrEntityExists
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Insert entity
	result, err := tx.Exec(
		"INSERT INTO entities (name, entity_type) VALUES (?, ?)",
		name, entityType,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Insert observations
	for _, obs := range observations {
		_, err := tx.Exec(
			"INSERT INTO observations (entity_id, content) VALUES (?, ?)",
			id, obs,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Entity{
		ID:           id,
		Name:         name,
		Type:         entityType,
		Observations: observations,
		CreatedAt:    time.Now(),
	}, nil
}

// CreateOrUpdateEntity creates a new entity or a new version if one exists.
// If an entity with the same name exists, creates a new version and marks old as not latest.
func (s *Store) CreateOrUpdateEntity(name, entityType string, observations []string) (*Entity, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check for existing entity
	var existingID int64
	var existingVersion int
	err = tx.QueryRow(
		"SELECT id, COALESCE(version, 1) FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)",
		name,
	).Scan(&existingID, &existingVersion)

	var supersedesID int64
	var newVersion int

	if err == sql.ErrNoRows {
		// No existing entity - create first version
		supersedesID = 0
		newVersion = 1
	} else if err != nil {
		return nil, err
	} else {
		// Existing entity - mark it as not latest
		_, err = tx.Exec(
			"UPDATE entities SET is_latest = 0 WHERE id = ?",
			existingID,
		)
		if err != nil {
			return nil, err
		}
		supersedesID = existingID
		newVersion = existingVersion + 1
	}

	// Insert new entity/version
	result, err := tx.Exec(
		"INSERT INTO entities (name, entity_type, version, is_latest, supersedes_id) VALUES (?, ?, ?, 1, ?)",
		name, entityType, newVersion, sql.NullInt64{Int64: supersedesID, Valid: supersedesID > 0},
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Insert observations
	for _, obs := range observations {
		_, err := tx.Exec(
			"INSERT INTO observations (entity_id, content) VALUES (?, ?)",
			id, obs,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Entity{
		ID:           id,
		Name:         name,
		Type:         entityType,
		Observations: observations,
		CreatedAt:    time.Now(),
		Version:      newVersion,
		IsLatest:     true,
		SupersedesID: supersedesID,
	}, nil
}

// GetEntityHistory returns all versions of an entity, newest first.
func (s *Store) GetEntityHistory(name string) ([]*Entity, error) {
	rows, err := s.db.Query(`
		SELECT id, name, entity_type, created_at, COALESCE(version, 1), COALESCE(is_latest, 1), COALESCE(supersedes_id, 0)
		FROM entities
		WHERE name = ?
		ORDER BY version DESC
	`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		var supersedesID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.CreatedAt, &e.Version, &e.IsLatest, &supersedesID); err != nil {
			return nil, err
		}
		if supersedesID.Valid {
			e.SupersedesID = supersedesID.Int64
		}
		entities = append(entities, &e)
	}

	if len(entities) == 0 {
		return nil, ErrNotFound
	}

	return entities, nil
}

// GetEntity retrieves an entity by name, including its observations.
// Returns the latest version only.
func (s *Store) GetEntity(name string) (*Entity, error) {
	var entity Entity
	var supersedesID sql.NullInt64
	err := s.db.QueryRow(
		`SELECT id, name, entity_type, created_at, COALESCE(version, 1), COALESCE(is_latest, 1), COALESCE(supersedes_id, 0)
		 FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)`,
		name,
	).Scan(&entity.ID, &entity.Name, &entity.Type, &entity.CreatedAt, &entity.Version, &entity.IsLatest, &supersedesID)
	if supersedesID.Valid {
		entity.SupersedesID = supersedesID.Int64
	}

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Load observations
	rows, err := s.db.Query(
		"SELECT content FROM observations WHERE entity_id = ? ORDER BY created_at",
		entity.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		entity.Observations = append(entity.Observations, content)
	}

	return &entity, nil
}

// ListEntities returns all entities, optionally filtered by type.
// Only returns latest versions.
func (s *Store) ListEntities(entityType string) ([]*Entity, error) {
	var rows *sql.Rows
	var err error

	if entityType == "" {
		rows, err = s.db.Query(
			`SELECT id, name, entity_type, created_at, COALESCE(version, 1), COALESCE(is_latest, 1)
			 FROM entities WHERE is_latest = 1 OR is_latest IS NULL ORDER BY name`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, name, entity_type, created_at, COALESCE(version, 1), COALESCE(is_latest, 1)
			 FROM entities WHERE entity_type = ? AND (is_latest = 1 OR is_latest IS NULL) ORDER BY name`,
			entityType,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		var e Entity
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.CreatedAt, &e.Version, &e.IsLatest); err != nil {
			return nil, err
		}
		entities = append(entities, &e)
	}

	return entities, nil
}

// DeleteEntity removes an entity and its observations (via CASCADE).
func (s *Store) DeleteEntity(name string) error {
	result, err := s.db.Exec("DELETE FROM entities WHERE name = ?", name)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// CountObservations returns the total number of observations (for testing).
func (s *Store) CountObservations() int {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM observations").Scan(&count)
	return count
}

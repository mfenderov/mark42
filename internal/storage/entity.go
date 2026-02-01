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
}

// CreateEntity creates a new entity with optional observations.
func (s *Store) CreateEntity(name, entityType string, observations []string) (*Entity, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

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

// GetEntity retrieves an entity by name, including its observations.
func (s *Store) GetEntity(name string) (*Entity, error) {
	var entity Entity
	err := s.db.QueryRow(
		"SELECT id, name, entity_type, created_at FROM entities WHERE name = ?",
		name,
	).Scan(&entity.ID, &entity.Name, &entity.Type, &entity.CreatedAt)

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
func (s *Store) ListEntities(entityType string) ([]*Entity, error) {
	var rows *sql.Rows
	var err error

	if entityType == "" {
		rows, err = s.db.Query(
			"SELECT id, name, entity_type, created_at FROM entities ORDER BY name",
		)
	} else {
		rows, err = s.db.Query(
			"SELECT id, name, entity_type, created_at FROM entities WHERE entity_type = ? ORDER BY name",
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
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.CreatedAt); err != nil {
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

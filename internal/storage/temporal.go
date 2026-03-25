package storage

import (
	"database/sql"
	"time"
)

// ObservationWithValidity represents an observation including its temporal validity window.
type ObservationWithValidity struct {
	Content      string         `db:"content"`
	FactType     string         `db:"fact_type"`
	ValidFromStr string         `db:"valid_from"`
	ValidUntil   sql.NullString `db:"valid_until"`
}

// ValidFrom parses the ValidFromStr field as a time.Time.
// Accepts SQLite datetime format ("2006-01-02 15:04:05") or RFC3339.
// Returns zero time.Time if parsing fails — callers must check for zero if exactness matters.
func (o *ObservationWithValidity) ValidFrom() time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", o.ValidFromStr)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, o.ValidFromStr)
	}
	return t
}

// InvalidateObservation marks a specific observation as expired by setting valid_until to now.
// Returns ErrNotFound if no matching observation exists on the latest version of the entity.
func (s *Store) InvalidateObservation(entityName, content string) error {
	result, err := s.db.Exec(`
		UPDATE observations
		SET valid_until = CURRENT_TIMESTAMP
		WHERE entity_id = (SELECT id FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL))
		  AND content = ?
		  AND valid_until IS NULL
	`, entityName, content)
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

// GetObservationHistory returns all observations for an entity, including expired ones.
// Results are ordered newest valid_from first.
func (s *Store) GetObservationHistory(entityName string) ([]ObservationWithValidity, error) {
	var entityID int64
	err := s.db.QueryRow(
		"SELECT id FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)",
		entityName,
	).Scan(&entityID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var results []ObservationWithValidity
	err = s.db.Select(&results, `
		SELECT content,
		       COALESCE(fact_type, 'dynamic') AS fact_type,
		       valid_from,
		       valid_until
		FROM observations
		WHERE entity_id = ?
		ORDER BY valid_from DESC
	`, entityID)
	if err != nil {
		return nil, err
	}
	return results, nil
}

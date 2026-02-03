package storage

import (
	"time"
)

// DecayConfig holds configuration for memory decay operations.
type DecayConfig struct {
	SoftDecayThreshold   float64 // Importance below which to apply soft decay
	ArchiveAfterDays     int     // Days after which to archive low-importance memories
	ForgetAfterDays      int     // Days after which to delete expired memories
	MinImportanceToKeep  float64 // Minimum importance to avoid archival
}

// DefaultDecayConfig returns the default decay configuration.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		SoftDecayThreshold:  0.3,
		ArchiveAfterDays:    90,
		ForgetAfterDays:     180,
		MinImportanceToKeep: 0.1,
	}
}

// DecayResult holds the result of a decay operation.
type DecayResult struct {
	SoftDecayed int // Number of observations with lowered importance
	Archived    int // Number of observations moved to archive
	Forgotten   int // Number of observations deleted
}

// ApplySoftDecay applies decay to importance scores based on recency.
// Observations not accessed recently have their importance reduced.
func (s *Store) ApplySoftDecay(threshold float64) (int, error) {
	cfg := DefaultImportanceConfig()

	// Apply decay factor to importance based on days since last access
	result, err := s.db.Exec(`
		UPDATE observations
		SET importance = importance * (
			SELECT CASE
				WHEN julianday('now') - julianday(COALESCE(last_accessed, created_at)) > 0
				THEN exp(-(julianday('now') - julianday(COALESCE(last_accessed, created_at))) / ?)
				ELSE 1.0
			END
		)
		WHERE importance >= ? AND importance < 1.0
		AND entity_id IN (SELECT id FROM entities WHERE is_latest = 1)
	`, cfg.DecayConstant, threshold)

	if err != nil {
		return 0, err
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// ArchivedObservation represents an observation that has been archived.
type ArchivedObservation struct {
	ID               int64     `db:"id"`
	OriginalEntityID int64     `db:"original_entity_id"`
	EntityName       string    `db:"entity_name"`
	Content          string    `db:"content"`
	FactType         string    `db:"fact_type"`
	Importance       float64   `db:"importance"`
	ArchivedAt       time.Time `db:"archived_at"`
}

// GetArchiveCount returns the number of archived observations.
func (s *Store) GetArchiveCount() (int, error) {
	var count int
	err := s.db.Get(&count, `
		SELECT COUNT(*) FROM archived_observations
	`)
	if err != nil {
		// Table might not exist yet
		return 0, nil
	}
	return count, nil
}

// ArchiveOldMemories moves low-importance, old observations to the archive table.
// Returns the number of archived observations.
func (s *Store) ArchiveOldMemories(cfg DecayConfig) (int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -cfg.ArchiveAfterDays)

	// First, insert into archive (the table is created by migration)
	result, err := s.db.Exec(`
		INSERT INTO archived_observations (original_entity_id, entity_name, content, fact_type, importance, archived_at)
		SELECT o.entity_id, e.name, o.content, o.fact_type, o.importance, datetime('now')
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
		AND o.importance < ?
		AND COALESCE(o.last_accessed, o.created_at) < ?
		AND o.fact_type != 'static'
	`, cfg.MinImportanceToKeep, cutoffDate.Format("2006-01-02 15:04:05"))

	if err != nil {
		return 0, err
	}

	archived, _ := result.RowsAffected()
	if archived == 0 {
		return 0, nil
	}

	// Then delete the original observations
	_, err = s.db.Exec(`
		DELETE FROM observations
		WHERE id IN (
			SELECT o.id FROM observations o
			JOIN entities e ON e.id = o.entity_id
			WHERE e.is_latest = 1
			AND o.importance < ?
			AND COALESCE(o.last_accessed, o.created_at) < ?
			AND o.fact_type != 'static'
		)
	`, cfg.MinImportanceToKeep, cutoffDate.Format("2006-01-02 15:04:05"))

	return int(archived), err
}

// ForgetExpiredMemories deletes observations that have passed their forget_after date.
// Returns the number of deleted observations.
func (s *Store) ForgetExpiredMemories() (int, error) {
	result, err := s.db.Exec(`
		DELETE FROM observations
		WHERE forget_after IS NOT NULL
		AND forget_after < datetime('now')
	`)

	if err != nil {
		return 0, err
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// ForgetOldArchivedMemories deletes archived observations older than the specified days.
func (s *Store) ForgetOldArchivedMemories(days int) (int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -days)

	result, err := s.db.Exec(`
		DELETE FROM archived_observations
		WHERE archived_at < ?
	`, cutoffDate.Format("2006-01-02 15:04:05"))

	if err != nil {
		return 0, err
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// DecayStats holds statistics about memory decay status.
type DecayStats struct {
	TotalObservations int     `db:"total"`
	LowImportance     int     `db:"low_importance"`
	ArchivedCount     int     `db:"archived_count"`
	ExpiredCount      int     `db:"expired_count"`
	AvgImportance     float64 `db:"avg_importance"`
}

// GetDecayStats returns statistics about memory decay status.
func (s *Store) GetDecayStats() (*DecayStats, error) {
	var stats DecayStats

	// Count observations
	err := s.db.Get(&stats.TotalObservations, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
	`)
	if err != nil {
		return nil, err
	}

	// Count low importance
	err = s.db.Get(&stats.LowImportance, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance < 0.3
	`)
	if err != nil {
		return nil, err
	}

	// Count archived
	stats.ArchivedCount, _ = s.GetArchiveCount()

	// Count expired (past forget_after)
	err = s.db.Get(&stats.ExpiredCount, `
		SELECT COUNT(*) FROM observations
		WHERE forget_after IS NOT NULL AND forget_after < datetime('now')
	`)
	if err != nil {
		stats.ExpiredCount = 0
	}

	// Average importance
	err = s.db.Get(&stats.AvgImportance, `
		SELECT COALESCE(AVG(importance), 0) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
	`)
	if err != nil {
		stats.AvgImportance = 0
	}

	return &stats, nil
}

// SetForgetAfter sets the forget_after date for observations of an entity.
func (s *Store) SetForgetAfter(entityName string, forgetAfter time.Time) error {
	_, err := s.db.Exec(`
		UPDATE observations
		SET forget_after = ?
		WHERE entity_id = (SELECT id FROM entities WHERE name = ? AND is_latest = 1)
	`, forgetAfter.Format("2006-01-02 15:04:05"), entityName)
	return err
}

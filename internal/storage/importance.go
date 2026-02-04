package storage

import (
	"math"
	"time"
)

// ImportanceConfig holds configuration for importance scoring.
type ImportanceConfig struct {
	DecayConstant    float64 // Days for significant decay (default: 30)
	RecencyWeight    float64 // Weight for recency factor (0-1)
	FrequencyWeight  float64 // Weight for access frequency (0-1)
	CentralityWeight float64 // Weight for relation centrality (0-1)
}

// DefaultImportanceConfig returns the default importance scoring configuration.
func DefaultImportanceConfig() ImportanceConfig {
	return ImportanceConfig{
		DecayConstant:    30,  // 50% decay after ~21 days
		RecencyWeight:    0.4, // 40% weight on recency
		FrequencyWeight:  0.3, // 30% weight on frequency
		CentralityWeight: 0.3, // 30% weight on centrality
	}
}

// CalculateRecencyDecay calculates exponential decay based on days since last access.
// Returns a value between 0 and 1, where 1 is "just accessed" and 0 approaches "very old".
// Formula: e^(-days / decayConstant)
func CalculateRecencyDecay(daysSinceAccess, decayConstant float64) float64 {
	if daysSinceAccess <= 0 {
		return 1.0
	}
	return math.Exp(-daysSinceAccess / decayConstant)
}

// CalculateFrequencyScore returns a score based on access count.
// Uses logarithmic scaling to provide diminishing returns for frequent access.
// Formula: 1 + log(1 + accessCount) / 10
// Returns values starting at ~1.0 for 0 accesses, increasing slowly.
func CalculateFrequencyScore(accessCount int) float64 {
	if accessCount < 0 {
		accessCount = 0
	}
	// log(1 + count) gives 0 for 0 accesses, ~2.3 for 10, ~4.6 for 100
	// Divide by 10 to normalize, add 1 for base
	return 1.0 + math.Log(float64(1+accessCount))/10.0
}

// CalculateCentralityScore returns a score based on relation count relative to max.
// Well-connected entities are more likely to be relevant.
// Formula: 0.5 + 0.5 * (relationCount / maxRelations)
// Returns 0.5 for isolated nodes, 1.0 for most connected.
func CalculateCentralityScore(relationCount, maxRelations int) float64 {
	if maxRelations <= 0 {
		return 0.75 // Default for empty graph
	}
	ratio := float64(relationCount) / float64(maxRelations)
	if ratio > 1 {
		ratio = 1
	}
	return 0.5 + 0.5*ratio
}

// CalculateImportance computes the overall importance score for an observation.
// Combines base score with recency, frequency, and centrality factors.
func CalculateImportance(
	baseScore float64,
	daysSinceAccess float64,
	accessCount int,
	relationCount int,
	maxRelations int,
	cfg ImportanceConfig,
) float64 {
	recency := CalculateRecencyDecay(daysSinceAccess, cfg.DecayConstant)
	frequency := CalculateFrequencyScore(accessCount)
	centrality := CalculateCentralityScore(relationCount, maxRelations)

	// Weighted combination
	combined := (cfg.RecencyWeight * recency) +
		(cfg.FrequencyWeight * frequency) +
		(cfg.CentralityWeight * centrality)

	return baseScore * combined
}

// UpdateLastAccessed updates the last_accessed timestamp for all observations of an entity.
func (s *Store) UpdateLastAccessed(entityName string) error {
	_, err := s.db.Exec(`
		UPDATE observations
		SET last_accessed = CURRENT_TIMESTAMP
		WHERE entity_id = (SELECT id FROM entities WHERE name = ? AND is_latest = 1)
	`, entityName)
	return err
}

// GetLastAccessed returns the last_accessed time for an entity's observations.
func (s *Store) GetLastAccessed(entityName string) (time.Time, error) {
	var accessedStr string
	err := s.db.Get(&accessedStr, `
		SELECT COALESCE(MAX(last_accessed), created_at) as last_accessed
		FROM observations
		WHERE entity_id = (SELECT id FROM entities WHERE name = ? AND is_latest = 1)
	`, entityName)
	if err != nil {
		return time.Time{}, err
	}

	// SQLite stores timestamps as strings, parse them
	accessed, err := time.Parse("2006-01-02 15:04:05", accessedStr)
	if err != nil {
		// Try alternative format
		accessed, err = time.Parse(time.RFC3339, accessedStr)
	}
	return accessed, err
}

// ObservationImportance represents an observation with its importance score.
type ObservationImportance struct {
	ObservationID int64   `db:"observation_id"`
	EntityName    string  `db:"entity_name"`
	Content       string  `db:"content"`
	Importance    float64 `db:"importance"`
	FactType      string  `db:"fact_type"`
}

// RecalculateImportance recalculates importance scores for all observations.
// Returns the number of observations updated.
func (s *Store) RecalculateImportance() (int, error) {
	cfg := DefaultImportanceConfig()

	// Get max relations for centrality calculation
	var maxRelations int
	err := s.db.Get(&maxRelations, `
		SELECT COALESCE(MAX(rel_count), 0)
		FROM (
			SELECT COUNT(*) as rel_count
			FROM relations
			GROUP BY from_entity_id
		)
	`)
	if err != nil {
		maxRelations = 1 // Avoid division by zero
	}
	if maxRelations == 0 {
		maxRelations = 1
	}

	// Get all observations with their metadata
	rows, err := s.db.Query(`
		SELECT o.id, o.importance, o.fact_type,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since,
		       (SELECT COUNT(*) FROM relations WHERE from_entity_id = o.entity_id OR to_entity_id = o.entity_id) as relation_count
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	updated := 0
	for rows.Next() {
		var id int64
		var baseImportance float64
		var factType string
		var daysSince float64
		var relationCount int

		if err := rows.Scan(&id, &baseImportance, &factType, &daysSince, &relationCount); err != nil {
			continue
		}

		// Static facts get a bonus (they're user-defined and permanent)
		baseScore := baseImportance
		if factType == string(FactTypeStatic) {
			baseScore = math.Max(baseScore, 0.8) // Minimum 0.8 for static facts
		}

		// Calculate new importance (access count not tracked separately, use 0)
		newImportance := CalculateImportance(
			baseScore,
			daysSince,
			0, // Access count (could be added to schema if needed)
			relationCount,
			maxRelations,
			cfg,
		)

		// Update if changed significantly (avoid unnecessary writes)
		if math.Abs(newImportance-baseImportance) > 0.01 {
			_, err := s.db.Exec(
				"UPDATE observations SET importance = ? WHERE id = ?",
				newImportance, id,
			)
			if err == nil {
				updated++
			}
		}
	}

	return updated, nil
}

// SetObservationImportance sets the importance score for a specific observation.
func (s *Store) SetObservationImportance(entityName, content string, importance float64) error {
	_, err := s.db.Exec(`
		UPDATE observations
		SET importance = ?
		WHERE entity_id = (SELECT id FROM entities WHERE name = ?)
		AND content = ?
	`, importance, entityName, content)
	return err
}

// GetObservationsByImportance returns observations with importance above the threshold.
func (s *Store) GetObservationsByImportance(minImportance float64) ([]ObservationWithMeta, error) {
	var results []ObservationWithMeta
	err := s.db.Select(&results, `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance >= ?
		ORDER BY o.importance DESC
	`, minImportance)
	return results, err
}

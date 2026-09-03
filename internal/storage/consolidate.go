package storage

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type obsWithEmbedding struct {
	id        int64
	content   string
	embedding []float64 // nil if no embedding stored
}

// ConsolidateWithSimilarity merges duplicate or near-duplicate observations for an entity
// using embedding cosine similarity when available, falling back to substring containment.
// For each duplicate pair, the shorter observation is expired (longer is kept).
// threshold is the minimum cosine similarity to consider two observations duplicates.
// Returns a summary string describing what was consolidated.
func (s *Store) ConsolidateWithSimilarity(entityName string, threshold float64) (string, error) {
	entity, err := s.GetEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("entity not found: %w", err)
	}

	observations, err := s.loadObservationsWithEmbeddings(entity.ID)
	if err != nil {
		return "", err
	}

	if len(observations) <= 1 {
		return fmt.Sprintf("%s: nothing to consolidate (%d observations)", entityName, len(observations)), nil
	}

	expired := 0
	for content := range findDuplicatesToExpire(observations, threshold) {
		if err := s.InvalidateObservation(entityName, content); err != nil {
			return "", fmt.Errorf("expiring observation: %w", err)
		}
		expired++
	}

	return fmt.Sprintf("Consolidated entity %s: removed %d duplicate observations", entityName, expired), nil
}

// loadObservationsWithEmbeddings loads all valid observations with their embeddings in one query.
func (s *Store) loadObservationsWithEmbeddings(entityID int64) ([]obsWithEmbedding, error) {
	rows, err := s.db.Query(`
		SELECT o.id, o.content, oe.embedding
		FROM observations o
		LEFT JOIN observation_embeddings oe ON oe.observation_id = o.id
		WHERE o.entity_id = ? AND o.valid_until IS NULL
		ORDER BY o.created_at
	`, entityID)
	if err != nil {
		return nil, fmt.Errorf("loading observations with embeddings: %w", err)
	}
	defer rows.Close()

	var observations []obsWithEmbedding
	for rows.Next() {
		var obs obsWithEmbedding
		var blob []byte
		if err := rows.Scan(&obs.id, &obs.content, &blob); err != nil {
			return nil, fmt.Errorf("scanning observation: %w", err)
		}
		if blob != nil {
			obs.embedding = decodeEmbedding(blob)
		}
		observations = append(observations, obs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating observations: %w", err)
	}
	return observations, nil
}

// findDuplicatesToExpire returns the contents of duplicate observations to
// expire: for each duplicate pair the shorter observation is expired (ties
// expire the later one). Cosine similarity is used when both observations
// have embeddings, substring containment otherwise.
func findDuplicatesToExpire(observations []obsWithEmbedding, threshold float64) map[string]bool {
	toExpire := make(map[string]bool)
	seen := make(map[int64]bool) // already marked for expiry (prevents double-processing)

	for i := 0; i < len(observations); i++ {
		if seen[observations[i].id] {
			continue
		}
		for j := i + 1; j < len(observations); j++ {
			if seen[observations[j].id] || !isDuplicate(observations[i], observations[j], threshold) {
				continue
			}
			if len(observations[i].content) >= len(observations[j].content) {
				toExpire[observations[j].content] = true
				seen[observations[j].id] = true
			} else {
				toExpire[observations[i].content] = true
				seen[observations[i].id] = true
			}
		}
	}
	return toExpire
}

// isDuplicate reports whether two observations are duplicates: cosine
// similarity above threshold when both have embeddings, substring
// containment otherwise.
func isDuplicate(a, b obsWithEmbedding, threshold float64) bool {
	if a.embedding != nil && b.embedding != nil {
		return CosineSimilarity(a.embedding, b.embedding) > threshold
	}
	lowerA := strings.ToLower(a.content)
	lowerB := strings.ToLower(b.content)
	return strings.Contains(lowerA, lowerB) || strings.Contains(lowerB, lowerA)
}

// ConsolidateObservations merges duplicate or near-duplicate observations for an entity.
// Uses string containment as a similarity heuristic: if one observation is a substring
// of another, the shorter one is removed (the longer one is more comprehensive).
// Returns a summary of what was consolidated.
func (s *Store) ConsolidateObservations(entityName string) (string, error) {
	entity, err := s.GetEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("entity not found: %w", err)
	}

	if len(entity.Observations) <= 1 {
		return fmt.Sprintf("%s: nothing to consolidate (%d observations)", entityName, len(entity.Observations)), nil
	}

	observations := entity.Observations
	uniqueDeletes := findContainedDuplicates(observations)

	// Delete duplicate observations atomically.
	// FTS5 triggers fire inside the transaction — observations_fts stays in sync.
	tx, err := s.db.Beginx()
	if err != nil {
		return "", fmt.Errorf("beginning consolidation transaction: %w", err)
	}
	defer tx.Rollback()

	deleted, err := deleteObservationsTx(tx, entity.ID, uniqueDeletes)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing consolidation transaction: %w", err)
	}

	return fmt.Sprintf("%s: consolidated %d redundant observations (kept %d)",
		entityName, deleted, len(observations)-deleted), nil
}

// findContainedDuplicates returns the deduplicated list of observations that
// are a substring of another observation (the shorter one is redundant).
func findContainedDuplicates(observations []string) []string {
	seen := make(map[string]bool)
	var uniqueDeletes []string

	for i := 0; i < len(observations); i++ {
		for j := i + 1; j < len(observations); j++ {
			lowerI := strings.ToLower(observations[i])
			lowerJ := strings.ToLower(observations[j])

			var redundant string
			switch {
			case strings.Contains(lowerJ, lowerI):
				redundant = observations[i]
			case strings.Contains(lowerI, lowerJ):
				redundant = observations[j]
			}
			if redundant != "" && !seen[redundant] {
				seen[redundant] = true
				uniqueDeletes = append(uniqueDeletes, redundant)
			}
		}
	}
	return uniqueDeletes
}

// deleteObservationsTx deletes observations by content inside a transaction.
func deleteObservationsTx(tx *sqlx.Tx, entityID int64, observations []string) (int, error) {
	deleted := 0
	for _, obs := range observations {
		result, err := tx.Exec(
			"DELETE FROM observations WHERE entity_id = ? AND content = ?",
			entityID, obs,
		)
		if err != nil {
			return 0, fmt.Errorf("deleting observation: %w", err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			deleted++
		}
	}
	return deleted, nil
}

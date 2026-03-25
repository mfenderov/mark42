package storage

import (
	"fmt"
	"strings"
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

	// Load all valid observations with their embeddings in one query
	rows, err := s.db.Query(`
		SELECT o.id, o.content, oe.embedding
		FROM observations o
		LEFT JOIN observation_embeddings oe ON oe.observation_id = o.id
		WHERE o.entity_id = ? AND o.valid_until IS NULL
		ORDER BY o.created_at
	`, entity.ID)
	if err != nil {
		return "", fmt.Errorf("loading observations with embeddings: %w", err)
	}
	defer rows.Close()

	var observations []obsWithEmbedding
	for rows.Next() {
		var obs obsWithEmbedding
		var blob []byte
		if err := rows.Scan(&obs.id, &obs.content, &blob); err != nil {
			return "", fmt.Errorf("scanning observation: %w", err)
		}
		if blob != nil {
			obs.embedding = decodeEmbedding(blob)
		}
		observations = append(observations, obs)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating observations: %w", err)
	}

	if len(observations) <= 1 {
		return fmt.Sprintf("%s: nothing to consolidate (%d observations)", entityName, len(observations)), nil
	}

	// Track which observations should be expired (by content)
	toExpire := make(map[string]bool)

	for i := 0; i < len(observations); i++ {
		if toExpire[observations[i].content] {
			continue
		}
		for j := i + 1; j < len(observations); j++ {
			if toExpire[observations[j].content] {
				continue
			}

			a := observations[i]
			b := observations[j]
			isDuplicate := false

			// Use cosine similarity if both have embeddings
			if a.embedding != nil && b.embedding != nil {
				sim := CosineSimilarity(a.embedding, b.embedding)
				isDuplicate = sim > threshold
			} else {
				// Fallback: substring containment
				lowerA := strings.ToLower(a.content)
				lowerB := strings.ToLower(b.content)
				isDuplicate = strings.Contains(lowerA, lowerB) || strings.Contains(lowerB, lowerA)
			}

			if isDuplicate {
				// Expire the shorter one; if equal length, expire b (later encountered = b)
				if len(a.content) >= len(b.content) {
					toExpire[b.content] = true
				} else {
					toExpire[a.content] = true
				}
			}
		}
	}

	expired := 0
	for content := range toExpire {
		if err := s.InvalidateObservation(entityName, content); err != nil {
			return "", fmt.Errorf("expiring observation: %w", err)
		}
		expired++
	}

	return fmt.Sprintf("Consolidated entity %s: removed %d duplicate observations", entityName, expired), nil
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

	// Find observations where one is a substring of another
	var toDelete []string
	observations := entity.Observations

	for i := 0; i < len(observations); i++ {
		for j := i + 1; j < len(observations); j++ {
			lowerI := strings.ToLower(observations[i])
			lowerJ := strings.ToLower(observations[j])

			if strings.Contains(lowerJ, lowerI) {
				// observations[i] is contained in observations[j] — remove the shorter one
				toDelete = append(toDelete, observations[i])
			} else if strings.Contains(lowerI, lowerJ) {
				// observations[j] is contained in observations[i] — remove the shorter one
				toDelete = append(toDelete, observations[j])
			}
		}
	}

	// Deduplicate toDelete list
	seen := make(map[string]bool)
	var uniqueDeletes []string
	for _, d := range toDelete {
		if !seen[d] {
			seen[d] = true
			uniqueDeletes = append(uniqueDeletes, d)
		}
	}

	// Delete duplicate observations atomically.
	// FTS5 triggers fire inside the transaction — observations_fts stays in sync.
	tx, err := s.db.Beginx()
	if err != nil {
		return "", fmt.Errorf("beginning consolidation transaction: %w", err)
	}
	defer tx.Rollback()

	deleted := 0
	for _, obs := range uniqueDeletes {
		result, execErr := tx.Exec(
			"DELETE FROM observations WHERE entity_id = ? AND content = ?",
			entity.ID, obs,
		)
		if execErr != nil {
			return "", fmt.Errorf("deleting observation: %w", execErr)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			deleted++
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing consolidation transaction: %w", err)
	}

	return fmt.Sprintf("%s: consolidated %d redundant observations (kept %d)",
		entityName, deleted, len(observations)-deleted), nil
}

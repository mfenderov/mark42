package storage

import (
	"context"
	"database/sql"
	"time"
)

// DefaultSupersessionThreshold is the cosine similarity threshold above which an observation
// is considered superseded by new content and eligible for automatic expiry.
const DefaultSupersessionThreshold = 0.85

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

// DetectAndExpireSuperseded checks existing observations for the entity against the new content
// using cosine similarity. Any observation whose similarity to newContent exceeds threshold
// (and is not identical to newContent) is expired by setting valid_until to now.
// Returns the list of expired observation contents.
// Returns nil, nil if embedder is nil or entity is not found.
func (s *Store) DetectAndExpireSuperseded(entityName, newContent string, embedder Embedder, threshold float64) ([]string, error) {
	return s.detectAndExpireSuperseded(entityName, []string{newContent}, embedder, threshold)
}

// DetectAndExpireSupersededBatch is like DetectAndExpireSuperseded but for a
// batch of observations written together: batch members never expire each
// other (issue #32). An older observation is expired when ANY new content
// exceeds the similarity threshold.
func (s *Store) DetectAndExpireSupersededBatch(entityName string, newContents []string, embedder Embedder, threshold float64) ([]string, error) {
	return s.detectAndExpireSuperseded(entityName, newContents, embedder, threshold)
}

func (s *Store) detectAndExpireSuperseded(entityName string, newContents []string, embedder Embedder, threshold float64) ([]string, error) {
	if embedder == nil || len(newContents) == 0 {
		return nil, nil
	}

	entityID, err := s.latestEntityID(entityName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Embed each new content; a failed embedding skips detection for that
	// content only (matches the previous per-content call semantics).
	newEmbeddings := make([][]float64, 0, len(newContents))
	for _, content := range newContents {
		emb, err := embedder.CreateEmbedding(context.Background(), content)
		if err != nil {
			continue
		}
		newEmbeddings = append(newEmbeddings, emb)
	}

	protected := make(map[string]bool, len(newContents))
	for _, c := range newContents {
		protected[c] = true
	}

	candidates, err := s.supersedeCandidates(entityID)
	if err != nil {
		return nil, err
	}

	return s.expireSimilar(candidates, newEmbeddings, protected, threshold)
}

// expireSimilar expires candidates similar to any new embedding, skipping
// protected (just-written) contents. Returns the expired contents.
func (s *Store) expireSimilar(candidates []supersedeCandidate, newEmbeddings [][]float64, protected map[string]bool, threshold float64) ([]string, error) {
	var expired []string
	for _, c := range candidates {
		if protected[c.content] || !exceedsThreshold(newEmbeddings, decodeEmbedding(c.blob), threshold) {
			continue
		}
		if _, err := s.db.Exec(
			"UPDATE observations SET valid_until = CURRENT_TIMESTAMP WHERE id = ?",
			c.id,
		); err != nil {
			return nil, err
		}
		expired = append(expired, c.content)
	}
	return expired, nil
}

// latestEntityID resolves the ID of the latest version of an entity.
func (s *Store) latestEntityID(entityName string) (int64, error) {
	var entityID int64
	err := s.db.QueryRow(
		"SELECT id FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)",
		entityName,
	).Scan(&entityID)
	return entityID, err
}

type supersedeCandidate struct {
	id      int64
	content string
	blob    []byte
}

// supersedeCandidates loads valid observations with embeddings for an entity.
func (s *Store) supersedeCandidates(entityID int64) ([]supersedeCandidate, error) {
	rows, err := s.db.Query(`
		SELECT o.id, o.content, oe.embedding
		FROM observations o
		JOIN observation_embeddings oe ON oe.observation_id = o.id
		WHERE o.entity_id = ? AND o.valid_until IS NULL
	`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []supersedeCandidate
	for rows.Next() {
		var c supersedeCandidate
		if err := rows.Scan(&c.id, &c.content, &c.blob); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// exceedsThreshold reports whether any new embedding is similar enough to the stored one.
func exceedsThreshold(newEmbeddings [][]float64, stored []float64, threshold float64) bool {
	for _, emb := range newEmbeddings {
		if CosineSimilarity(emb, stored) > threshold {
			return true
		}
	}
	return false
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

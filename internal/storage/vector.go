package storage

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// VectorResult represents a vector search result.
type VectorResult struct {
	EntityName string
	EntityType string
	Content    string
	Score      float64 // Cosine similarity (0-1)
}

// getObservationID returns the ID of an observation by entity and content.
func (s *Store) getObservationID(entityID int64, content string) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		"SELECT id FROM observations WHERE entity_id = ? AND content = ?",
		entityID, content,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("observation not found: %w", err)
	}
	return id, nil
}

// StoreEmbedding stores an embedding vector for an observation.
func (s *Store) StoreEmbedding(observationID int64, embedding []float64, model string) error {
	// Encode embedding as binary (more efficient than JSON)
	blob := encodeEmbedding(embedding)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO observation_embeddings (observation_id, embedding, model, dimensions)
		VALUES (?, ?, ?, ?)
	`, observationID, blob, model, len(embedding))

	if err != nil {
		return fmt.Errorf("storing embedding: %w", err)
	}
	return nil
}

// GetEmbedding retrieves the embedding for an observation.
func (s *Store) GetEmbedding(observationID int64) ([]float64, error) {
	var blob []byte
	err := s.db.QueryRow(
		"SELECT embedding FROM observation_embeddings WHERE observation_id = ?",
		observationID,
	).Scan(&blob)
	if err != nil {
		return nil, fmt.Errorf("getting embedding: %w", err)
	}
	return decodeEmbedding(blob), nil
}

// HasEmbedding checks if an observation has a stored embedding.
func (s *Store) HasEmbedding(observationID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM observation_embeddings WHERE observation_id = ?",
		observationID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// VectorSearch finds observations similar to the query embedding.
func (s *Store) VectorSearch(queryEmbedding []float64, limit int) ([]VectorResult, error) {
	// Load all embeddings (for small knowledge graphs this is fine)
	// For larger datasets, consider approximate nearest neighbor indices
	rows, err := s.db.Query(`
		SELECT oe.observation_id, oe.embedding, o.content, e.name, e.entity_type
		FROM observation_embeddings oe
		JOIN observations o ON o.id = oe.observation_id
		JOIN entities e ON e.id = o.entity_id
	`)
	if err != nil {
		return nil, fmt.Errorf("loading embeddings: %w", err)
	}
	defer rows.Close()

	var results []VectorResult
	for rows.Next() {
		var obsID int64
		var blob []byte
		var content, entityName, entityType string

		if err := rows.Scan(&obsID, &blob, &content, &entityName, &entityType); err != nil {
			return nil, err
		}

		embedding := decodeEmbedding(blob)
		similarity := CosineSimilarity(queryEmbedding, embedding)

		results = append(results, VectorResult{
			EntityName: entityName,
			EntityType: entityType,
			Content:    content,
			Score:      similarity,
		})
	}

	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetObservationsWithoutEmbeddings returns observations that need embeddings.
func (s *Store) GetObservationsWithoutEmbeddings() ([]ObservationWithID, error) {
	rows, err := s.db.Query(`
		SELECT o.id, o.content, e.name, e.entity_type
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		LEFT JOIN observation_embeddings oe ON oe.observation_id = o.id
		WHERE oe.observation_id IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ObservationWithID
	for rows.Next() {
		var obs ObservationWithID
		if err := rows.Scan(&obs.ID, &obs.Content, &obs.EntityName, &obs.EntityType); err != nil {
			return nil, err
		}
		results = append(results, obs)
	}
	return results, nil
}

// ObservationWithID represents an observation with its database ID.
type ObservationWithID struct {
	ID         int64
	Content    string
	EntityName string
	EntityType string
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns value in range [-1, 1] where 1 means identical direction.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// encodeEmbedding converts a float64 slice to a binary blob.
func encodeEmbedding(embedding []float64) []byte {
	buf := make([]byte, len(embedding)*8)
	for i, v := range embedding {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}
	return buf
}

// decodeEmbedding converts a binary blob to a float64 slice.
func decodeEmbedding(blob []byte) []float64 {
	n := len(blob) / 8
	embedding := make([]float64, n)
	for i := range n {
		embedding[i] = math.Float64frombits(binary.LittleEndian.Uint64(blob[i*8:]))
	}
	return embedding
}

// BatchStoreEmbeddings stores multiple embeddings efficiently.
func (s *Store) BatchStoreEmbeddings(observations []ObservationWithID, embeddings [][]float64, model string) error {
	if len(observations) != len(embeddings) {
		return fmt.Errorf("observations and embeddings count mismatch: %d vs %d", len(observations), len(embeddings))
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO observation_embeddings (observation_id, embedding, model, dimensions)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, obs := range observations {
		blob := encodeEmbedding(embeddings[i])
		if _, err := stmt.Exec(obs.ID, blob, model, len(embeddings[i])); err != nil {
			return fmt.Errorf("storing embedding for obs %d: %w", obs.ID, err)
		}
	}

	return tx.Commit()
}

// EmbeddingStats returns statistics about stored embeddings.
func (s *Store) EmbeddingStats() (total int, withEmbeddings int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM observations").Scan(&total)
	if err != nil {
		return 0, 0, err
	}

	err = s.db.QueryRow("SELECT COUNT(*) FROM observation_embeddings").Scan(&withEmbeddings)
	if err != nil {
		if err == sql.ErrNoRows {
			return total, 0, nil
		}
		return 0, 0, err
	}

	return total, withEmbeddings, nil
}

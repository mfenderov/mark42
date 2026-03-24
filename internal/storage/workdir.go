package storage

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

// SetContainerTag sets the container_tag for an entity.
// Container tags are used for working directory awareness (multi-project scoping).
func (s *Store) SetContainerTag(entityName, containerTag string) error {
	result, err := s.db.Exec(`
		UPDATE entities SET container_tag = ?
		WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)
	`, containerTag, entityName)
	if err != nil {
		return fmt.Errorf("failed to set container tag: %w", err)
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

// GetContainerTag retrieves the container_tag for an entity.
func (s *Store) GetContainerTag(entityName string) (string, error) {
	var tag sql.NullString
	err := s.db.Get(&tag, `
		SELECT container_tag FROM entities
		WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)
	`, entityName)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return tag.String, nil
}

// BatchGetContainerTags retrieves container tags for multiple entities in one query.
// Returns a map of entity name → tag (missing entities are not in the map; entities
// with no tag have an empty string value).
func (s *Store) BatchGetContainerTags(entityNames []string) (map[string]string, error) {
	if len(entityNames) == 0 {
		return map[string]string{}, nil
	}

	placeholders := strings.Repeat("?,", len(entityNames))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(`
		SELECT name, COALESCE(container_tag, '') as tag
		FROM entities
		WHERE name IN (%s) AND (is_latest = 1 OR is_latest IS NULL)
	`, placeholders)

	args := make([]any, len(entityNames))
	for i, n := range entityNames {
		args[i] = n
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch get container tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string, len(entityNames))
	for rows.Next() {
		var name, tag string
		if err := rows.Scan(&name, &tag); err != nil {
			return nil, err
		}
		result[name] = tag
	}
	return result, rows.Err()
}

// GetEntitiesByContainerTag retrieves all entities with a specific container tag.
func (s *Store) GetEntitiesByContainerTag(containerTag string) ([]*Entity, error) {
	var entities []Entity
	err := s.db.Select(&entities, `
		SELECT id, name, entity_type, created_at,
		       COALESCE(version, 1) as version,
		       COALESCE(is_latest, 1) as is_latest,
		       COALESCE(supersedes_id, 0) as supersedes_id
		FROM entities
		WHERE container_tag = ? AND (is_latest = 1 OR is_latest IS NULL)
		ORDER BY name
	`, containerTag)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice
	result := make([]*Entity, len(entities))
	for i := range entities {
		result[i] = &entities[i]
	}
	return result, nil
}

// CreateEntityWithContainer creates an entity with a container tag in a single transaction.
func (s *Store) CreateEntityWithContainer(name, entityType string, observations []string, containerTag string) (*Entity, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if entity already exists
	var existingID int64
	err = tx.QueryRow("SELECT id FROM entities WHERE name = ?", name).Scan(&existingID)
	if err == nil {
		return nil, ErrEntityExists
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Insert entity with container tag
	result, err := tx.Exec(
		"INSERT INTO entities (name, entity_type, container_tag) VALUES (?, ?, ?)",
		name, entityType, containerTag,
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
	}, nil
}

// HybridSearchWithBoost performs hybrid search with container tag boosting.
// Results from entities with matching containerTag receive a score multiplier.
func (s *Store) HybridSearchWithBoost(ctx context.Context, query string, queryEmbedding []float64, limit int, containerTag string, boostFactor float64) ([]FusedResult, error) {
	// Perform standard hybrid search
	results, err := s.HybridSearch(ctx, query, queryEmbedding, limit*2) // Get extra for re-ranking
	if err != nil {
		return nil, err
	}

	// Apply container tag boost (batch fetch to avoid N+1)
	if containerTag != "" {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.EntityName
		}
		tags, err := s.BatchGetContainerTags(names)
		if err != nil {
			return nil, fmt.Errorf("fetching container tags for boost: %w", err)
		}
		for i := range results {
			if tags[results[i].EntityName] == containerTag {
				results[i].FusionScore *= boostFactor
			}
		}
	}

	// Re-sort by boosted score
	sortFusedResultsByScore(results)

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// sortFusedResultsByScore sorts results by FusionScore descending (higher is better).
func sortFusedResultsByScore(results []FusedResult) {
	slices.SortFunc(results, func(a, b FusedResult) int {
		if b.FusionScore > a.FusionScore {
			return 1
		}
		if b.FusionScore < a.FusionScore {
			return -1
		}
		return 0
	})
}

// GetContextWithContainerTag retrieves context with container tag boosting.
// Memories with matching container_tag receive a score boost.
func (s *Store) GetContextWithContainerTag(cfg ContextConfig, containerTag string) ([]ContextResult, error) {
	// Query all eligible observations with their entity's container_tag
	query := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       e.container_tag
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance >= ?
		ORDER BY o.importance DESC
	`

	type resultWithTag struct {
		EntityName   string         `db:"entity_name"`
		EntityType   string         `db:"entity_type"`
		Content      string         `db:"content"`
		FactType     string         `db:"fact_type"`
		Importance   float64        `db:"importance"`
		ContainerTag sql.NullString `db:"container_tag"`
	}

	var rawResults []resultWithTag
	err := s.db.Select(&rawResults, query, cfg.MinImportance)
	if err != nil {
		return nil, err
	}

	// Convert and apply boosts
	results := make([]ContextResult, len(rawResults))
	for i, r := range rawResults {
		results[i] = ContextResult{
			EntityName: r.EntityName,
			EntityType: r.EntityType,
			Content:    r.Content,
			FactType:   r.FactType,
			Importance: r.Importance,
			FinalScore: r.Importance,
		}

		// Apply container tag boost
		if containerTag != "" && r.ContainerTag.Valid && r.ContainerTag.String == containerTag {
			results[i].FinalScore *= cfg.ProjectBoost
		}

		// Apply static fact boost
		if r.FactType == "static" {
			results[i].FinalScore *= 1.2
		}
	}

	// Sort by final score (descending)
	slices.SortFunc(results, func(a, b ContextResult) int {
		if b.FinalScore > a.FinalScore {
			return 1
		}
		if b.FinalScore < a.FinalScore {
			return -1
		}
		return 0
	})

	// Apply token budget
	tokenCount := 0
	var selected []ContextResult
	for _, r := range results {
		entryTokens := (len(r.EntityName) + len(r.Content) + 20) / 4
		if tokenCount+entryTokens > cfg.TokenBudget {
			break
		}
		tokenCount += entryTokens
		selected = append(selected, r)
	}

	return selected, nil
}

// DefaultContextConfig returns the default context configuration.
// This is a method on Store to allow tests to call it.
func (s *Store) DefaultContextConfig() ContextConfig {
	return DefaultContextConfig()
}

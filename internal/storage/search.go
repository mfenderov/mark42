package storage

import (
	"strings"
)

// SearchResult represents an entity found by search.
type SearchResult struct {
	*Entity
	Score float64
}

// Graph represents the entire knowledge graph.
type Graph struct {
	Entities  []*Entity
	Relations []*Relation
}

// Search finds entities matching the query using FTS5.
func (s *Store) Search(query string) ([]*SearchResult, error) {
	return s.SearchWithLimit(query, 20)
}

// SearchWithLimit finds entities with a result limit.
func (s *Store) SearchWithLimit(query string, limit int) ([]*SearchResult, error) {
	// Escape FTS5 special characters and prepare query
	ftsQuery := prepareFTSQuery(query)

	// Search both observations and entity names
	// Union results and rank by BM25 score
	rows, err := s.db.Query(`
		WITH observation_matches AS (
			SELECT DISTINCT o.entity_id, bm25(observations_fts) as score
			FROM observations_fts f
			JOIN observations o ON o.id = f.rowid
			WHERE observations_fts MATCH ?
		),
		entity_matches AS (
			SELECT e.id as entity_id, bm25(entities_fts) as score
			FROM entities_fts f
			JOIN entities e ON e.id = f.rowid
			WHERE entities_fts MATCH ?
		),
		combined AS (
			SELECT entity_id, MIN(score) as score
			FROM (
				SELECT entity_id, score FROM observation_matches
				UNION ALL
				SELECT entity_id, score FROM entity_matches
			)
			GROUP BY entity_id
		)
		SELECT e.id, e.name, e.entity_type, e.created_at, c.score
		FROM combined c
		JOIN entities e ON e.id = c.entity_id
		ORDER BY c.score
		LIMIT ?
	`, ftsQuery, ftsQuery, limit)
	if err != nil {
		// If FTS query fails (invalid syntax), return empty results
		if strings.Contains(err.Error(), "fts5") {
			return []*SearchResult{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var r SearchResult
		r.Entity = &Entity{}
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.CreatedAt, &r.Score); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}

	// Load observations for each result
	for _, r := range results {
		obs, err := s.loadObservations(r.ID)
		if err != nil {
			return nil, err
		}
		r.Observations = obs
	}

	return results, nil
}

// ReadGraph returns the entire knowledge graph.
func (s *Store) ReadGraph() (*Graph, error) {
	entities, err := s.ListEntities("")
	if err != nil {
		return nil, err
	}

	// Load observations for each entity
	for _, e := range entities {
		obs, err := s.loadObservations(e.ID)
		if err != nil {
			return nil, err
		}
		e.Observations = obs
	}

	// Load all relations using sqlx
	var relList []Relation
	err = s.db.Select(&relList, `
		SELECT e_from.name as from_name, e_to.name as to_name,
		       r.relation_type, r.created_at
		FROM relations r
		JOIN entities e_from ON r.from_entity_id = e_from.id
		JOIN entities e_to ON r.to_entity_id = e_to.id
		ORDER BY r.created_at
	`)
	if err != nil {
		return nil, err
	}

	// Convert to pointer slice
	relations := make([]*Relation, len(relList))
	for i := range relList {
		relations[i] = &relList[i]
	}

	return &Graph{
		Entities:  entities,
		Relations: relations,
	}, nil
}

func (s *Store) loadObservations(entityID int64) ([]string, error) {
	var observations []string
	err := s.db.Select(&observations,
		"SELECT content FROM observations WHERE entity_id = ? ORDER BY created_at",
		entityID)
	return observations, err
}

// prepareFTSQuery escapes special characters and formats for FTS5.
func prepareFTSQuery(query string) string {
	// For simple queries, just wrap each word with quotes for exact matching
	// This handles most cases without complex escaping
	words := strings.Fields(query)
	if len(words) == 0 {
		return "\"\""
	}

	// Use OR to match any word
	var quoted []string
	for _, word := range words {
		// Escape quotes within the word
		word = strings.ReplaceAll(word, "\"", "\"\"")
		quoted = append(quoted, "\""+word+"\"")
	}

	return strings.Join(quoted, " OR ")
}

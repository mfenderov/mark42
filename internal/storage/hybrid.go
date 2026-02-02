package storage

import (
	"context"
	"strings"
)

// HybridSearch combines FTS5 keyword search with vector semantic search using RRF fusion.
// If queryEmbedding is nil, only FTS search is performed.
// If query is empty, only vector search is performed.
func (s *Store) HybridSearch(ctx context.Context, query string, queryEmbedding []float64, limit int) ([]FusedResult, error) {
	strategyResults := make(map[string][]RankedItem)

	// FTS search if query provided
	if strings.TrimSpace(query) != "" {
		ftsResults, err := s.ftsSearch(query, limit*2) // Get more results for better fusion
		if err != nil {
			return nil, err
		}
		if len(ftsResults) > 0 {
			strategyResults["fts"] = ftsResults
		}
	}

	// Vector search if embedding provided
	if len(queryEmbedding) > 0 {
		vectorResults, err := s.VectorSearch(queryEmbedding, limit*2)
		if err != nil {
			return nil, err
		}
		if len(vectorResults) > 0 {
			ranked := make([]RankedItem, len(vectorResults))
			for i, r := range vectorResults {
				ranked[i] = RankedItem{
					EntityName: r.EntityName,
					EntityType: r.EntityType,
					Content:    r.Content,
					Score:      r.Score,
					Source:     "vector",
				}
			}
			strategyResults["vector"] = ranked
		}
	}

	// No results from either strategy
	if len(strategyResults) == 0 {
		return []FusedResult{}, nil
	}

	// Fuse results using RRF
	results := FuseRRF(strategyResults, DefaultRRFConfig())

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ftsSearch performs FTS5 search and returns RankedItems.
func (s *Store) ftsSearch(query string, limit int) ([]RankedItem, error) {
	ftsQuery := prepareFTSQuery(query)

	rows, err := s.db.Query(`
		WITH observation_matches AS (
			SELECT DISTINCT o.entity_id, o.content, bm25(observations_fts) as score
			FROM observations_fts f
			JOIN observations o ON o.id = f.rowid
			WHERE observations_fts MATCH ?
		),
		entity_matches AS (
			SELECT e.id as entity_id, e.name as content, bm25(entities_fts) as score
			FROM entities_fts f
			JOIN entities e ON e.id = f.rowid
			WHERE entities_fts MATCH ?
		),
		combined AS (
			SELECT entity_id, content, MIN(score) as score
			FROM (
				SELECT entity_id, content, score FROM observation_matches
				UNION ALL
				SELECT entity_id, content, score FROM entity_matches
			)
			GROUP BY content
		)
		SELECT e.name, e.entity_type, c.content, c.score
		FROM combined c
		JOIN entities e ON e.id = c.entity_id
		ORDER BY c.score
		LIMIT ?
	`, ftsQuery, ftsQuery, limit)

	if err != nil {
		// If FTS query fails, return empty
		if strings.Contains(err.Error(), "fts5") {
			return []RankedItem{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var results []RankedItem
	for rows.Next() {
		var r RankedItem
		if err := rows.Scan(&r.EntityName, &r.EntityType, &r.Content, &r.Score); err != nil {
			return nil, err
		}
		// BM25 scores are negative (lower is better), convert to positive
		r.Score = -r.Score
		r.Source = "fts"
		results = append(results, r)
	}

	return results, nil
}

// HybridSearchWithEmbedder combines search with automatic embedding generation.
// Uses the provided embedder to generate query embeddings on the fly.
func (s *Store) HybridSearchWithEmbedder(ctx context.Context, query string, embedder *EmbeddingClient, limit int) ([]FusedResult, error) {
	var queryEmbedding []float64

	// Generate embedding for query if embedder is available
	if embedder != nil && strings.TrimSpace(query) != "" {
		emb, err := embedder.CreateEmbedding(ctx, query)
		if err != nil {
			// Log but continue with FTS-only search
			// Vector search is enhancement, not requirement
			return s.HybridSearch(ctx, query, nil, limit)
		}
		queryEmbedding = emb
	}

	return s.HybridSearch(ctx, query, queryEmbedding, limit)
}

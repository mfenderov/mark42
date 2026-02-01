package storage

import (
	"cmp"
	"slices"
)

// RankedItem represents a search result from a single strategy.
// Used as input to fusion algorithms.
type RankedItem struct {
	EntityName string
	EntityType string
	Content    string  // Observation content or entity name
	Score      float64 // Original score from strategy (BM25, cosine similarity, etc.)
	Source     string  // Strategy name: "fts", "vector", etc.
}

// FusedResult represents a result after fusion from multiple strategies.
type FusedResult struct {
	EntityName   string
	EntityType   string
	Content      string
	FusionScore  float64            // Combined score after fusion
	SourceScores map[string]float64 // Original scores from each source
	SourceRanks  map[string]int     // Rank in each source's results
}

// RRFConfig holds configuration for Reciprocal Rank Fusion.
type RRFConfig struct {
	K int // Smoothing parameter (default: 60)
}

// DefaultRRFConfig returns the default RRF configuration.
// k=60 is the standard value from the original RRF paper.
func DefaultRRFConfig() RRFConfig {
	return RRFConfig{K: 60}
}

// FuseRRF combines results from multiple search strategies using Reciprocal Rank Fusion.
//
// The RRF formula: score(d) = Σ(1 / (k + rank(d)))
// where k is typically 60, and rank starts at 1 for the top result.
//
// Reference: "Reciprocal Rank Fusion outperforms Condorcet and individual Rank Learning Methods"
// by Cormack, Clarke, and Buettcher (SIGIR 2009)
func FuseRRF(strategyResults map[string][]RankedItem, config RRFConfig) []FusedResult {
	if len(strategyResults) == 0 {
		return []FusedResult{}
	}

	k := cmp.Or(config.K, 60)

	// If only one strategy, convert and return
	if len(strategyResults) == 1 {
		for source, results := range strategyResults {
			fused := make([]FusedResult, len(results))
			for i, r := range results {
				fused[i] = FusedResult{
					EntityName:   r.EntityName,
					EntityType:   r.EntityType,
					Content:      r.Content,
					FusionScore:  r.Score,
					SourceScores: map[string]float64{source: r.Score},
					SourceRanks:  map[string]int{source: i + 1},
				}
			}
			return fused
		}
	}

	// Build unique document map using content as key
	type fusedDoc struct {
		EntityName   string
		EntityType   string
		Content      string
		SourceScores map[string]float64
		SourceRanks  map[string]int
		FusionScore  float64
	}

	docScores := make(map[string]*fusedDoc)

	for source, results := range strategyResults {
		for rank, result := range results {
			// Use content as unique identifier
			docID := result.Content

			if _, exists := docScores[docID]; !exists {
				docScores[docID] = &fusedDoc{
					EntityName:   result.EntityName,
					EntityType:   result.EntityType,
					Content:      result.Content,
					SourceScores: make(map[string]float64),
					SourceRanks:  make(map[string]int),
				}
			}

			// RRF formula: 1 / (k + rank)
			// rank starts at 1 for the first result
			rrfScore := 1.0 / float64(k+rank+1)

			docScores[docID].FusionScore += rrfScore
			docScores[docID].SourceScores[source] = result.Score
			docScores[docID].SourceRanks[source] = rank + 1
		}
	}

	// Convert to slice and sort by fusion score (descending)
	results := make([]FusedResult, 0, len(docScores))
	for _, doc := range docScores {
		results = append(results, FusedResult{
			EntityName:   doc.EntityName,
			EntityType:   doc.EntityType,
			Content:      doc.Content,
			FusionScore:  doc.FusionScore,
			SourceScores: doc.SourceScores,
			SourceRanks:  doc.SourceRanks,
		})
	}

	slices.SortFunc(results, func(a, b FusedResult) int {
		return cmp.Compare(b.FusionScore, a.FusionScore) // Descending
	})

	return results
}

// WeightedFusion combines results using weighted scores.
type WeightedConfig struct {
	Weights map[string]float64 // source name -> weight
}

// FuseWeighted combines results using weighted score summation.
func FuseWeighted(strategyResults map[string][]RankedItem, config WeightedConfig) []FusedResult {
	if len(strategyResults) == 0 {
		return []FusedResult{}
	}

	// Default equal weights if not specified
	if len(config.Weights) == 0 {
		config.Weights = make(map[string]float64)
		for source := range strategyResults {
			config.Weights[source] = 1.0
		}
	}

	type fusedDoc struct {
		EntityName   string
		EntityType   string
		Content      string
		SourceScores map[string]float64
		FusionScore  float64
	}

	docScores := make(map[string]*fusedDoc)

	for source, results := range strategyResults {
		weight := config.Weights[source]
		if weight == 0 {
			weight = 1.0
		}

		for _, result := range results {
			docID := result.Content

			if _, exists := docScores[docID]; !exists {
				docScores[docID] = &fusedDoc{
					EntityName:   result.EntityName,
					EntityType:   result.EntityType,
					Content:      result.Content,
					SourceScores: make(map[string]float64),
				}
			}

			docScores[docID].FusionScore += result.Score * weight
			docScores[docID].SourceScores[source] = result.Score
		}
	}

	results := make([]FusedResult, 0, len(docScores))
	for _, doc := range docScores {
		results = append(results, FusedResult{
			EntityName:   doc.EntityName,
			EntityType:   doc.EntityType,
			Content:      doc.Content,
			FusionScore:  doc.FusionScore,
			SourceScores: doc.SourceScores,
		})
	}

	slices.SortFunc(results, func(a, b FusedResult) int {
		return cmp.Compare(b.FusionScore, a.FusionScore)
	})

	return results
}

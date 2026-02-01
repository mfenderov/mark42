package storage

import (
	"testing"
)

func TestFuseRRF_EmptyInput(t *testing.T) {
	results := FuseRRF(nil, DefaultRRFConfig())
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestFuseRRF_SingleStrategy(t *testing.T) {
	input := map[string][]RankedItem{
		"fts": {
			{EntityName: "user", EntityType: "person", Content: "prefers typescript", Score: 0.9},
			{EntityName: "user", EntityType: "person", Content: "uses vim", Score: 0.7},
		},
	}

	results := FuseRRF(input, DefaultRRFConfig())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should have higher score
	if results[0].Content != "prefers typescript" {
		t.Errorf("expected 'prefers typescript' first, got %q", results[0].Content)
	}
}

func TestFuseRRF_MultipleStrategies(t *testing.T) {
	// Simulate FTS and vector search results
	input := map[string][]RankedItem{
		"fts": {
			{EntityName: "user", EntityType: "person", Content: "prefers typescript", Score: 0.9},
			{EntityName: "user", EntityType: "person", Content: "uses bun", Score: 0.7},
			{EntityName: "user", EntityType: "person", Content: "works on auth", Score: 0.5},
		},
		"vector": {
			{EntityName: "user", EntityType: "person", Content: "works on auth", Score: 0.95},
			{EntityName: "user", EntityType: "person", Content: "prefers typescript", Score: 0.8},
			{EntityName: "user", EntityType: "person", Content: "debugging oauth", Score: 0.6},
		},
	}

	results := FuseRRF(input, DefaultRRFConfig())

	// Should have 4 unique results
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Items appearing in both lists should rank higher due to RRF
	// "prefers typescript" appears in both: rank 1 in fts, rank 2 in vector
	// "works on auth" appears in both: rank 3 in fts, rank 1 in vector

	// Both should have source scores from both strategies
	foundBoth := 0
	for _, r := range results {
		if len(r.SourceScores) == 2 {
			foundBoth++
		}
	}
	if foundBoth != 2 {
		t.Errorf("expected 2 results with scores from both strategies, got %d", foundBoth)
	}

	// Verify source ranks are tracked
	for _, r := range results {
		if r.Content == "prefers typescript" {
			if r.SourceRanks["fts"] != 1 {
				t.Errorf("expected FTS rank 1 for 'prefers typescript', got %d", r.SourceRanks["fts"])
			}
			if r.SourceRanks["vector"] != 2 {
				t.Errorf("expected vector rank 2 for 'prefers typescript', got %d", r.SourceRanks["vector"])
			}
		}
	}
}

func TestFuseRRF_RankingMath(t *testing.T) {
	// Test that RRF math is correct: score = 1/(k+rank)
	input := map[string][]RankedItem{
		"a": {
			{Content: "doc1", Score: 1.0},
			{Content: "doc2", Score: 0.5},
		},
		"b": {
			{Content: "doc2", Score: 1.0},
			{Content: "doc1", Score: 0.5},
		},
	}

	config := RRFConfig{K: 60}
	results := FuseRRF(input, config)

	// Both docs appear at rank 1 in one strategy and rank 2 in another
	// RRF score = 1/(60+1) + 1/(60+2) = 1/61 + 1/62 ≈ 0.0325
	expectedScore := 1.0/61.0 + 1.0/62.0

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Both should have the same score (symmetric ranking)
	tolerance := 0.0001
	for _, r := range results {
		diff := r.FusionScore - expectedScore
		if diff < -tolerance || diff > tolerance {
			t.Errorf("expected score ~%f for %q, got %f", expectedScore, r.Content, r.FusionScore)
		}
	}
}

func TestFuseWeighted_EmptyInput(t *testing.T) {
	results := FuseWeighted(nil, WeightedConfig{})
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestFuseWeighted_WithWeights(t *testing.T) {
	input := map[string][]RankedItem{
		"fts":    {{Content: "doc1", Score: 0.5}},
		"vector": {{Content: "doc1", Score: 0.8}},
	}

	// Weight vector search higher
	config := WeightedConfig{
		Weights: map[string]float64{
			"fts":    0.3,
			"vector": 0.7,
		},
	}

	results := FuseWeighted(input, config)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Expected: 0.5*0.3 + 0.8*0.7 = 0.15 + 0.56 = 0.71
	expected := 0.5*0.3 + 0.8*0.7
	tolerance := 0.0001
	diff := results[0].FusionScore - expected
	if diff < -tolerance || diff > tolerance {
		t.Errorf("expected weighted score %f, got %f", expected, results[0].FusionScore)
	}
}

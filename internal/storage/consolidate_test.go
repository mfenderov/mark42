package storage_test

import (
	"testing"
)

func TestStore_ConsolidateObservations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Create entity with redundant observations
	store.CreateEntity("Go", "language", []string{
		"Compiled language",
		"Go is a compiled language with fast build times",
		"Has goroutines",
	})

	result, err := store.ConsolidateObservations("Go")
	if err != nil {
		t.Fatalf("ConsolidateObservations failed: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty result")
	}

	// Verify: the short duplicate "Compiled language" should be removed
	entity, _ := store.GetEntity("Go")
	for _, obs := range entity.Observations {
		if obs == "Compiled language" {
			t.Error("short duplicate should have been removed")
		}
	}

	// "Go is a compiled language..." and "Has goroutines" should remain
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations remaining, got %d: %v", len(entity.Observations), entity.Observations)
	}
}

func TestStore_ConsolidateObservations_NoDuplicates(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Go", "language", []string{
		"Compiled language",
		"Has goroutines",
		"Created by Google",
	})

	result, err := store.ConsolidateObservations("Go")
	if err != nil {
		t.Fatalf("ConsolidateObservations failed: %v", err)
	}

	// Nothing to consolidate
	entity, _ := store.GetEntity("Go")
	if len(entity.Observations) != 3 {
		t.Errorf("expected 3 observations (no duplicates), got %d", len(entity.Observations))
	}
	_ = result
}

func TestStore_ConsolidateObservations_SingleObservation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Go", "language", []string{"Compiled language"})

	result, err := store.ConsolidateObservations("Go")
	if err != nil {
		t.Fatalf("ConsolidateObservations failed: %v", err)
	}

	if result == "" {
		t.Fatal("expected non-empty result")
	}

	entity, _ := store.GetEntity("Go")
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d", len(entity.Observations))
	}
}

func TestStore_ConsolidateObservations_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.ConsolidateObservations("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

func TestStore_ConsolidateObservations_AllDuplicatesRemoved(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// "Go" is a substring of all three "Go is X" observations — it qualifies for removal
	store.CreateEntity("Lang", "language", []string{
		"Go",
		"Go is fast",
		"Go is safe",
		"Go is concurrent",
		"Has generics",
	})

	_, err := store.ConsolidateObservations("Lang")
	if err != nil {
		t.Fatalf("ConsolidateObservations failed: %v", err)
	}

	entity, _ := store.GetEntity("Lang")

	// "Go" is a substring of all three "Go is X" obs — must be removed
	for _, obs := range entity.Observations {
		if obs == "Go" {
			t.Error(`"Go" is contained in the longer observations and should be removed`)
		}
	}

	// The 3 "Go is X" observations + "Has generics" must remain
	if len(entity.Observations) != 4 {
		t.Errorf("expected 4 observations after consolidation, got %d: %v",
			len(entity.Observations), entity.Observations)
	}
}

// --- ConsolidateWithSimilarity tests ---

func TestConsolidateWithSimilarity_SemanticMode(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Animals", "concept", []string{
		"A is about dogs",
		"A is about cats",
	})

	// Get observation IDs so we can store embeddings for them
	entity, err := store.GetEntity("Animals")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(entity.Observations) != 2 {
		t.Fatalf("expected 2 observations, got %d: %v", len(entity.Observations), entity.Observations)
	}

	obs1 := store.GetObservationWithID("Animals", "A is about dogs")
	obs2 := store.GetObservationWithID("Animals", "A is about cats")
	if obs1 == nil || obs2 == nil {
		t.Fatal("could not get observation IDs")
	}

	// Store nearly identical embeddings — cosine similarity will be > 0.85
	highSimEmbedding1 := []float64{0.9, 0.1, 0.0, 0.0}
	highSimEmbedding2 := []float64{0.9, 0.11, 0.0, 0.0}
	if err := store.StoreEmbedding(obs1.ID, highSimEmbedding1, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}
	if err := store.StoreEmbedding(obs2.ID, highSimEmbedding2, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	summary, err := store.ConsolidateWithSimilarity("Animals", 0.85)
	if err != nil {
		t.Fatalf("ConsolidateWithSimilarity failed: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// One observation should be expired
	afterEntity, err := store.GetEntity("Animals")
	if err != nil {
		t.Fatalf("GetEntity after consolidation failed: %v", err)
	}

	if len(afterEntity.Observations) != 1 {
		t.Errorf("expected 1 observation after consolidation, got %d: %v",
			len(afterEntity.Observations), afterEntity.Observations)
	}

	// The shorter one ("A is about dogs" == "A is about cats", both 16 chars)
	// When equal length, either may be kept. Just verify one is gone.
	remaining := afterEntity.Observations[0]
	if remaining != "A is about dogs" && remaining != "A is about cats" {
		t.Errorf("unexpected remaining observation: %q", remaining)
	}
}

func TestConsolidateWithSimilarity_NoEmbeddings(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Dogs", "concept", []string{
		"dogs are great",
		"dogs",
	})

	summary, err := store.ConsolidateWithSimilarity("Dogs", 0.85)
	if err != nil {
		t.Fatalf("ConsolidateWithSimilarity failed: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	entity, err := store.GetEntity("Dogs")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	// "dogs" is a substring of "dogs are great" — shorter should be expired
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d: %v", len(entity.Observations), entity.Observations)
	}

	if entity.Observations[0] != "dogs are great" {
		t.Errorf("expected 'dogs are great' to remain, got: %q", entity.Observations[0])
	}
}

func TestConsolidateWithSimilarity_KeepsLonger(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Tech", "concept", []string{
		"Go is a fast compiled language",
		"Go is fast",
	})

	obs1 := store.GetObservationWithID("Tech", "Go is a fast compiled language")
	obs2 := store.GetObservationWithID("Tech", "Go is fast")
	if obs1 == nil || obs2 == nil {
		t.Fatal("could not get observation IDs")
	}

	// High-similarity embeddings
	emb1 := []float64{1.0, 0.0, 0.0}
	emb2 := []float64{0.99, 0.01, 0.0}
	if err := store.StoreEmbedding(obs1.ID, emb1, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}
	if err := store.StoreEmbedding(obs2.ID, emb2, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	_, err := store.ConsolidateWithSimilarity("Tech", 0.85)
	if err != nil {
		t.Fatalf("ConsolidateWithSimilarity failed: %v", err)
	}

	entity, err := store.GetEntity("Tech")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation after consolidation, got %d: %v",
			len(entity.Observations), entity.Observations)
	}

	// The longer one must be kept
	if entity.Observations[0] != "Go is a fast compiled language" {
		t.Errorf("expected longer observation to remain, got: %q", entity.Observations[0])
	}
}

func TestConsolidateWithSimilarity_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.ConsolidateWithSimilarity("nonexistent", 0.85)
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

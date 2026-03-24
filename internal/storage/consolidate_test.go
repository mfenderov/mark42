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

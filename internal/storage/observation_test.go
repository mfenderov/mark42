package storage_test

import (
	"testing"

	"github.com/mfenderov/claude-memory/internal/storage"
)

func TestAddObservation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.CreateEntity("TDD", "pattern", nil)
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	err = store.AddObservation("TDD", "Red-Green-Refactor cycle")
	if err != nil {
		t.Fatalf("AddObservation failed: %v", err)
	}

	entity, _ := store.GetEntity("TDD")
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d", len(entity.Observations))
	}
	if entity.Observations[0] != "Red-Green-Refactor cycle" {
		t.Errorf("unexpected observation content: %q", entity.Observations[0])
	}
}

func TestAddObservation_EntityNotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	err := store.AddObservation("nonexistent", "some observation")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAddObservation_Duplicate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"existing"})

	// Adding same observation should be idempotent (no error, but no duplicate)
	err := store.AddObservation("TDD", "existing")
	if err != nil {
		t.Fatalf("AddObservation duplicate failed: %v", err)
	}

	entity, _ := store.GetEntity("TDD")
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation (no duplicate), got %d", len(entity.Observations))
	}
}

func TestDeleteObservation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"obs1", "obs2"})

	err := store.DeleteObservation("TDD", "obs1")
	if err != nil {
		t.Fatalf("DeleteObservation failed: %v", err)
	}

	entity, _ := store.GetEntity("TDD")
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation after delete, got %d", len(entity.Observations))
	}
	if entity.Observations[0] != "obs2" {
		t.Errorf("wrong observation remaining: %q", entity.Observations[0])
	}
}

func TestDeleteObservation_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"obs1"})

	err := store.DeleteObservation("TDD", "nonexistent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

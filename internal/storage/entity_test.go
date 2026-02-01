package storage_test

import (
	"testing"

	"github.com/mfenderov/claude-memory/internal/storage"
)

func TestCreateEntity(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	entity, err := store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	if entity.Name != "TDD" {
		t.Errorf("expected name 'TDD', got %q", entity.Name)
	}
	if entity.Type != "pattern" {
		t.Errorf("expected type 'pattern', got %q", entity.Type)
	}
	if len(entity.Observations) != 1 {
		t.Errorf("expected 1 observation, got %d", len(entity.Observations))
	}
	if entity.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestCreateEntity_Duplicate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.CreateEntity("TDD", "pattern", nil)
	if err != nil {
		t.Fatalf("first CreateEntity failed: %v", err)
	}

	_, err = store.CreateEntity("TDD", "pattern", nil)
	if err == nil {
		t.Error("expected error for duplicate entity, got nil")
	}
}

func TestGetEntity(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development", "Red-Green-Refactor"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	entity, err := store.GetEntity("TDD")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if entity.Name != "TDD" {
		t.Errorf("expected name 'TDD', got %q", entity.Name)
	}
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}
}

func TestGetEntity_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.GetEntity("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent entity, got nil")
	}
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListEntities(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("konfig", "project", nil)
	store.CreateEntity("Simple Design", "pattern", nil)

	// List all
	entities, err := store.ListEntities("")
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(entities))
	}

	// List by type
	patterns, err := store.ListEntities("pattern")
	if err != nil {
		t.Fatalf("ListEntities with type failed: %v", err)
	}
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}
}

func TestDeleteEntity(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	err = store.DeleteEntity("TDD")
	if err != nil {
		t.Fatalf("DeleteEntity failed: %v", err)
	}

	_, err = store.GetEntity("TDD")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteEntity_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	err := store.DeleteEntity("nonexistent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteEntity_CascadesObservations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.CreateEntity("TDD", "pattern", []string{"obs1", "obs2"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	err = store.DeleteEntity("TDD")
	if err != nil {
		t.Fatalf("DeleteEntity failed: %v", err)
	}

	// Observations should be deleted via CASCADE
	count := store.CountObservations()
	if count != 0 {
		t.Errorf("expected 0 observations after cascade delete, got %d", count)
	}
}

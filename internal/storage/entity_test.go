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

// --- Versioning tests ---

func TestCreateOrUpdateEntity_NewEntity(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	entity, err := store.CreateOrUpdateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	if err != nil {
		t.Fatalf("CreateOrUpdateEntity failed: %v", err)
	}

	if entity.Name != "TDD" {
		t.Errorf("expected name 'TDD', got %q", entity.Name)
	}
	if entity.Version != 1 {
		t.Errorf("expected version 1, got %d", entity.Version)
	}
	if !entity.IsLatest {
		t.Error("expected IsLatest to be true")
	}
}

func TestCreateOrUpdateEntity_UpdateCreatesVersion(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create v1
	v1, err := store.CreateOrUpdateEntity("TDD", "pattern", []string{"original fact"})
	if err != nil {
		t.Fatalf("CreateOrUpdateEntity v1 failed: %v", err)
	}

	// Create v2 (same name)
	v2, err := store.CreateOrUpdateEntity("TDD", "pattern", []string{"updated fact"})
	if err != nil {
		t.Fatalf("CreateOrUpdateEntity v2 failed: %v", err)
	}

	// v2 should be a new version
	if v2.Version != 2 {
		t.Errorf("expected version 2, got %d", v2.Version)
	}
	if v2.SupersedesID != v1.ID {
		t.Errorf("expected supersedes_id %d, got %d", v1.ID, v2.SupersedesID)
	}
	if !v2.IsLatest {
		t.Error("expected v2 IsLatest to be true")
	}

	// GetEntity should return v2 (latest)
	entity, _ := store.GetEntity("TDD")
	if entity.ID != v2.ID {
		t.Errorf("GetEntity should return latest version, got ID %d instead of %d", entity.ID, v2.ID)
	}
}

func TestGetEntityHistory(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create 3 versions
	store.CreateOrUpdateEntity("TDD", "pattern", []string{"v1 fact"})
	store.CreateOrUpdateEntity("TDD", "pattern", []string{"v2 fact"})
	store.CreateOrUpdateEntity("TDD", "pattern", []string{"v3 fact"})

	history, err := store.GetEntityHistory("TDD")
	if err != nil {
		t.Fatalf("GetEntityHistory failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 versions in history, got %d", len(history))
	}

	// Should be ordered newest first
	if history[0].Version != 3 {
		t.Errorf("expected newest version (3) first, got %d", history[0].Version)
	}
	if history[2].Version != 1 {
		t.Errorf("expected oldest version (1) last, got %d", history[2].Version)
	}
}

func TestListEntities_OnlyLatest(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with multiple versions
	store.CreateOrUpdateEntity("TDD", "pattern", []string{"v1"})
	store.CreateOrUpdateEntity("TDD", "pattern", []string{"v2"})
	store.CreateOrUpdateEntity("Other", "pattern", []string{"obs"})

	// ListEntities should only return latest versions
	entities, err := store.ListEntities("")
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	// Should have 2 entities (TDD latest + Other), not 3
	if len(entities) != 2 {
		t.Errorf("expected 2 entities (latest only), got %d", len(entities))
	}
}

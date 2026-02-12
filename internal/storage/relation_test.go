package storage_test

import (
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestCreateRelation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)

	err := store.CreateRelation("TDD", "Simple Design", "enables")
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}

	relations, err := store.ListRelations("TDD")
	if err != nil {
		t.Fatalf("ListRelations failed: %v", err)
	}

	if len(relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(relations))
	}
	if relations[0].To != "Simple Design" {
		t.Errorf("expected relation to 'Simple Design', got %q", relations[0].To)
	}
	if relations[0].Type != "enables" {
		t.Errorf("expected relation type 'enables', got %q", relations[0].Type)
	}
}

func TestCreateRelation_EntityNotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)

	err := store.CreateRelation("TDD", "nonexistent", "enables")
	if err == nil {
		t.Error("expected error for nonexistent target entity")
	}

	err = store.CreateRelation("nonexistent", "TDD", "enables")
	if err == nil {
		t.Error("expected error for nonexistent source entity")
	}
}

func TestCreateRelation_Duplicate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)

	store.CreateRelation("TDD", "Simple Design", "enables")

	// Duplicate should be idempotent
	err := store.CreateRelation("TDD", "Simple Design", "enables")
	if err != nil {
		t.Fatalf("duplicate CreateRelation should be idempotent: %v", err)
	}

	relations, _ := store.ListRelations("TDD")
	if len(relations) != 1 {
		t.Errorf("expected 1 relation (no duplicate), got %d", len(relations))
	}
}

func TestListRelations_BothDirections(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)
	store.CreateEntity("Refactoring", "practice", nil)

	store.CreateRelation("TDD", "Simple Design", "enables")
	store.CreateRelation("Refactoring", "TDD", "part-of")

	// TDD should show both outgoing and incoming relations
	relations, _ := store.ListRelations("TDD")
	if len(relations) != 2 {
		t.Errorf("expected 2 relations for TDD, got %d", len(relations))
	}
}

func TestDeleteRelation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)
	store.CreateRelation("TDD", "Simple Design", "enables")

	err := store.DeleteRelation("TDD", "Simple Design", "enables")
	if err != nil {
		t.Fatalf("DeleteRelation failed: %v", err)
	}

	relations, _ := store.ListRelations("TDD")
	if len(relations) != 0 {
		t.Errorf("expected 0 relations after delete, got %d", len(relations))
	}
}

func TestDeleteRelation_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)

	err := store.DeleteRelation("TDD", "Simple Design", "nonexistent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteEntity_CascadesRelations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", nil)
	store.CreateEntity("Simple Design", "pattern", nil)
	store.CreateRelation("TDD", "Simple Design", "enables")

	store.DeleteEntity("TDD")

	// Relations should be deleted via CASCADE
	relations, _ := store.ListRelations("Simple Design")
	if len(relations) != 0 {
		t.Errorf("expected 0 relations after cascade delete, got %d", len(relations))
	}
}

package storage_test

import (
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
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

func TestAddObservationWithType(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Run migrations to get fact_type column
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, err := store.CreateEntity("user_prefs", "person", nil)
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	// Add static fact
	err = store.AddObservationWithType("user_prefs", "prefers TypeScript", storage.FactTypeStatic)
	if err != nil {
		t.Fatalf("AddObservationWithType failed: %v", err)
	}

	// Add dynamic fact
	err = store.AddObservationWithType("user_prefs", "working on auth", storage.FactTypeDynamic)
	if err != nil {
		t.Fatalf("AddObservationWithType failed: %v", err)
	}

	// Verify both exist
	entity, _ := store.GetEntity("user_prefs")
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}
}

func TestGetObservationsByFactType(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, _ = store.CreateEntity("user", "person", nil)

	// Add observations with different types
	store.AddObservationWithType("user", "prefers Go", storage.FactTypeStatic)
	store.AddObservationWithType("user", "likes TDD", storage.FactTypeStatic)
	store.AddObservationWithType("user", "debugging auth bug", storage.FactTypeDynamic)
	store.AddObservationWithType("user", "said hello earlier", storage.FactTypeSessionTurn)

	// Get static facts
	static, err := store.GetObservationsByFactType(storage.FactTypeStatic)
	if err != nil {
		t.Fatalf("GetObservationsByFactType failed: %v", err)
	}
	if len(static) != 2 {
		t.Errorf("expected 2 static facts, got %d", len(static))
	}

	// Get dynamic facts
	dynamic, err := store.GetObservationsByFactType(storage.FactTypeDynamic)
	if err != nil {
		t.Fatalf("GetObservationsByFactType failed: %v", err)
	}
	if len(dynamic) != 1 {
		t.Errorf("expected 1 dynamic fact, got %d", len(dynamic))
	}

	// Get session turn facts
	turns, err := store.GetObservationsByFactType(storage.FactTypeSessionTurn)
	if err != nil {
		t.Fatalf("GetObservationsByFactType failed: %v", err)
	}
	if len(turns) != 1 {
		t.Errorf("expected 1 session turn fact, got %d", len(turns))
	}
}

func TestGetContextByFactType(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, _ = store.CreateEntity("user", "person", nil)
	_, _ = store.CreateEntity("project", "codebase", nil)

	store.AddObservationWithType("user", "prefers TypeScript", storage.FactTypeStatic)
	store.AddObservationWithType("user", "uses Neovim", storage.FactTypeStatic)
	store.AddObservationWithType("project", "debugging auth", storage.FactTypeDynamic)

	context, err := store.GetContextByFactType()
	if err != nil {
		t.Fatalf("GetContextByFactType failed: %v", err)
	}

	if len(context.Static) != 2 {
		t.Errorf("expected 2 static facts, got %d", len(context.Static))
	}
	if len(context.Dynamic) != 1 {
		t.Errorf("expected 1 dynamic fact, got %d", len(context.Dynamic))
	}
}

func TestFormatContextForInjection(t *testing.T) {
	context := &storage.ContextByFactType{
		Static: []storage.ObservationWithMeta{
			{EntityName: "user", Content: "prefers TypeScript"},
			{EntityName: "user", Content: "uses Neovim"},
		},
		Dynamic: []storage.ObservationWithMeta{
			{EntityName: "project", Content: "debugging auth flow"},
		},
		SessionTurn: []storage.ObservationWithMeta{
			{EntityName: "conversation", Content: "discussed TDD approach"},
		},
	}

	formatted := context.FormatContextForInjection()

	// Check structure
	if !strings.Contains(formatted, "<mark42-context>") {
		t.Error("missing opening tag")
	}
	if !strings.Contains(formatted, "</mark42-context>") {
		t.Error("missing closing tag")
	}
	if !strings.Contains(formatted, "## User Profile (Persistent)") {
		t.Error("missing static section header")
	}
	if !strings.Contains(formatted, "## Recent Context") {
		t.Error("missing dynamic section header")
	}
	if !strings.Contains(formatted, "## Session History") {
		t.Error("missing session turn section header")
	}

	// Check content
	if !strings.Contains(formatted, "- prefers TypeScript") {
		t.Error("missing static content")
	}
	if !strings.Contains(formatted, "- debugging auth flow") {
		t.Error("missing dynamic content")
	}
}

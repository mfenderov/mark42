package storage_test

import (
	"errors"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestInvalidateObservation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	obsContent := "uses TDD religiously"
	_, err := store.CreateEntity("Alice", "person", []string{obsContent})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	if err := store.InvalidateObservation("Alice", obsContent); err != nil {
		t.Fatalf("InvalidateObservation failed: %v", err)
	}

	entity, err := store.GetEntity("Alice")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	for _, obs := range entity.Observations {
		if obs == obsContent {
			t.Errorf("expected invalidated observation to be absent from GetEntity, but found it")
		}
	}

	history, err := store.GetObservationHistory("Alice")
	if err != nil {
		t.Fatalf("GetObservationHistory failed: %v", err)
	}
	found := false
	for _, h := range history {
		if h.Content == obsContent {
			found = true
			if !h.ValidUntil.Valid {
				t.Errorf("expected ValidUntil to be set for invalidated observation")
			}
		}
	}
	if !found {
		t.Errorf("expected invalidated observation in history, but not found")
	}
}

func TestInvalidateObservation_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, err := store.CreateEntity("Bob", "person", []string{"likes Go"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	err = store.InvalidateObservation("Bob", "nonexistent observation")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing observation, got %v", err)
	}

	// Double-invalidate: second call on an already-invalidated observation must also return ErrNotFound.
	if err := store.InvalidateObservation("Bob", "likes Go"); err != nil {
		t.Fatalf("first InvalidateObservation failed: %v", err)
	}
	err = store.InvalidateObservation("Bob", "likes Go")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound on double-invalidate, got %v", err)
	}
}

func TestValidityFilter_HidesExpiredFromSearch(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, err := store.CreateEntity("Languages", "concept", []string{"Go is great", "Python is also great"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	if err := store.InvalidateObservation("Languages", "Python is also great"); err != nil {
		t.Fatalf("InvalidateObservation failed: %v", err)
	}

	results, err := store.Search("Python")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	for _, r := range results {
		for _, obs := range r.Observations {
			if obs == "Python is also great" {
				t.Errorf("expected invalidated observation to be absent from Search results, but found it")
			}
		}
	}
}

func TestValidityFilter_ExemptsSessions(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create a regular entity with one observation
	_, err := store.CreateEntity("Dave", "person", []string{"regular fact"})
	if err != nil {
		t.Fatalf("CreateEntity person failed: %v", err)
	}

	// Create a session entity with a session_summary observation
	_, err = store.CreateEntity("session-2024-01", "session", []string{})
	if err != nil {
		t.Fatalf("CreateEntity session failed: %v", err)
	}
	if err := store.AddObservationWithType("session-2024-01", "session summary content", storage.FactTypeSessionSummary); err != nil {
		t.Fatalf("AddObservationWithType failed: %v", err)
	}

	// Set importance > MinImportance so both observations pass the importance filter
	store.SetObservationImportance("Dave", "regular fact", 0.9)
	store.SetObservationImportance("session-2024-01", "session summary content", 0.9)

	// Invalidate the regular entity's observation
	if err := store.InvalidateObservation("Dave", "regular fact"); err != nil {
		t.Fatalf("InvalidateObservation failed: %v", err)
	}

	cfg := storage.DefaultContextConfig()
	cfg.TokenBudget = 10000
	cfg.MinImportance = 0.3

	results, err := store.GetContextForInjection(cfg, "testproject")
	if err != nil {
		t.Fatalf("GetContextForInjection failed: %v", err)
	}

	foundRegularFact := false
	foundSessionSummary := false
	for _, r := range results {
		if r.Content == "regular fact" {
			foundRegularFact = true
		}
		if r.Content == "session summary content" {
			foundSessionSummary = true
		}
	}

	if foundRegularFact {
		t.Error("expected invalidated regular fact to be absent from GetContextForInjection results")
	}
	if !foundSessionSummary {
		t.Error("expected session summary content to appear in GetContextForInjection results (sessions exempt from validity filter)")
	}
}

func TestGetObservationHistory(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	obs1 := "writes failing tests first"
	obs2 := "refactors mercilessly"
	_, err := store.CreateEntity("Carol", "person", []string{obs1, obs2})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	if err := store.InvalidateObservation("Carol", obs1); err != nil {
		t.Fatalf("InvalidateObservation failed: %v", err)
	}

	history, err := store.GetObservationHistory("Carol")
	if err != nil {
		t.Fatalf("GetObservationHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("expected 2 observations in history, got %d", len(history))
	}

	for _, h := range history {
		switch h.Content {
		case obs1:
			if !h.ValidUntil.Valid {
				t.Errorf("obs1 should be invalidated (ValidUntil.Valid == true)")
			}
		case obs2:
			if h.ValidUntil.Valid {
				t.Errorf("obs2 should still be valid (ValidUntil.Valid == false)")
			}
		default:
			t.Errorf("unexpected observation content: %q", h.Content)
		}
	}
}

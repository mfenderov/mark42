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
		t.Errorf("expected ErrNotFound, got %v", err)
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

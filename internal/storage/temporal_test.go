package storage_test

import (
	"context"
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

	if len(results) != 0 {
		t.Errorf("expected no results for 'Python' after invalidation, got %d result(s)", len(results))
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

func TestDetectAndExpireSuperseded_NilEmbedder(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	_, err := store.CreateEntity("Alice", "person", []string{"uses TDD"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	expired, err := store.DetectAndExpireSuperseded("Alice", "something new", nil, 0.85)
	if err != nil {
		t.Errorf("expected nil error with nil embedder, got %v", err)
	}
	if expired != nil {
		t.Errorf("expected nil expired list with nil embedder, got %v", expired)
	}
}

type highSimilarityEmbedder struct{}

func (e *highSimilarityEmbedder) CreateEmbedding(_ context.Context, _ string) ([]float64, error) {
	return []float64{0.9, 0.1, 0.0}, nil
}

type lowSimilarityEmbedder struct {
	calls int
}

func (e *lowSimilarityEmbedder) CreateEmbedding(_ context.Context, _ string) ([]float64, error) {
	e.calls++
	if e.calls%2 == 1 {
		return []float64{1.0, 0.0, 0.0}, nil
	}
	return []float64{0.0, 1.0, 0.0}, nil
}

func TestDetectAndExpireSuperseded_WithEmbedder(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	oldContent := "old content about TDD"
	_, err := store.CreateEntity("Dev", "person", []string{oldContent})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	// Get the observation ID and store an embedding for it
	obs := store.GetObservationWithID("Dev", oldContent)
	if obs == nil {
		t.Fatalf("observation not found")
	}
	// Store a high-similarity embedding for old content
	if err := store.StoreEmbedding(obs.ID, []float64{0.9, 0.1, 0.0}, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	newContent := "new content about TDD practice"
	embedder := &highSimilarityEmbedder{}
	expired, err := store.DetectAndExpireSuperseded("Dev", newContent, embedder, 0.85)
	if err != nil {
		t.Fatalf("DetectAndExpireSuperseded failed: %v", err)
	}

	if len(expired) != 1 || expired[0] != oldContent {
		t.Errorf("expected [%q] in expired list, got %v", oldContent, expired)
	}

	// Old content should be hidden from GetEntity
	entityAfter, err := store.GetEntity("Dev")
	if err != nil {
		t.Fatalf("GetEntity after expiry failed: %v", err)
	}
	for _, o := range entityAfter.Observations {
		if o == oldContent {
			t.Errorf("expected old content to be expired and hidden from GetEntity, but found it")
		}
	}
}

func TestDetectAndExpireSuperseded_BelowThreshold(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	oldContent := "orthogonal content"
	_, err := store.CreateEntity("Dev2", "person", []string{oldContent})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	obs := store.GetObservationWithID("Dev2", oldContent)
	if obs == nil {
		t.Fatalf("observation not found")
	}
	// Orthogonal embedding: [0, 1, 0] vs query [1, 0, 0] → similarity 0
	if err := store.StoreEmbedding(obs.ID, []float64{0.0, 1.0, 0.0}, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Embedder returns a very different vector from the stored one
	embedder := &lowSimilarityEmbedder{}
	expired, err := store.DetectAndExpireSuperseded("Dev2", "new orthogonal content", embedder, 0.85)
	if err != nil {
		t.Fatalf("DetectAndExpireSuperseded failed: %v", err)
	}

	if len(expired) != 0 {
		t.Errorf("expected empty expired list below threshold, got %v", expired)
	}

	// Old content should still be visible
	entity2, err := store.GetEntity("Dev2")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	found := false
	for _, o := range entity2.Observations {
		if o == oldContent {
			found = true
		}
	}
	if !found {
		t.Error("expected old content to remain visible below threshold")
	}
}

func TestDetectAndExpireSuperseded_DoesNotExpireNewContentItself(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	content := "TDD practice"
	_, err := store.CreateEntity("Dev3", "person", []string{content})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	obs := store.GetObservationWithID("Dev3", content)
	if obs == nil {
		t.Fatalf("observation not found")
	}
	// Same embedding for both
	if err := store.StoreEmbedding(obs.ID, []float64{0.9, 0.1, 0.0}, "test"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Call DetectAndExpire with the SAME content — must not expire it
	embedder := &highSimilarityEmbedder{}
	expired, err := store.DetectAndExpireSuperseded("Dev3", content, embedder, 0.85)
	if err != nil {
		t.Fatalf("DetectAndExpireSuperseded failed: %v", err)
	}

	if len(expired) != 0 {
		t.Errorf("expected empty expired list when newContent == existing content, got %v", expired)
	}
}

func TestDetectAndExpireSuperseded_MissingEntity(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	embedder := &highSimilarityEmbedder{}
	expired, err := store.DetectAndExpireSuperseded("NonExistentEntity", "some content", embedder, 0.85)
	if err != nil {
		t.Errorf("expected nil error for missing entity, got %v", err)
	}
	if expired != nil {
		t.Errorf("expected nil expired list for missing entity, got %v", expired)
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

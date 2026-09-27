package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

type fakeEmbedder struct {
	embeddings map[string][]float64
}

func (f *fakeEmbedder) CreateEmbedding(_ context.Context, text string) ([]float64, error) {
	if vec, ok := f.embeddings[text]; ok {
		return vec, nil
	}
	// Return deterministic mock 3-dim vector
	return []float64{0.1, 0.2, 0.3}, nil
}

func newTestStoreWithDB(t *testing.T) *storage.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	return store
}

func TestStore_Remember_SessionAliasAndValidation(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	// "session" (the MCP/CLI surface spelling) must land as session_turn.
	if _, err := store.Remember(ctx, storage.RememberParams{
		Topic:    "standup",
		Facts:    []string{"Shipped login page"},
		FactType: storage.FactType("session"),
	}); err != nil {
		t.Fatalf("Remember with session fact type failed: %v", err)
	}
	turns, err := store.GetObservationsByFactType(storage.FactTypeSessionTurn)
	if err != nil {
		t.Fatalf("GetObservationsByFactType failed: %v", err)
	}
	if len(turns) != 1 || turns[0].Content != "Shipped login page" {
		t.Errorf("expected session fact stored as session_turn, got %v", turns)
	}

	// Unknown fact types must be rejected, not stored literally.
	if _, err := store.Remember(ctx, storage.RememberParams{
		Topic:    "standup",
		Facts:    []string{"Bogus"},
		FactType: storage.FactType("banana"),
	}); err == nil {
		t.Error("expected error for unknown fact type, got nil")
	}
}

func TestStore_Remember_CreateAndUpsert(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	// Initial Remember
	res, err := store.Remember(ctx, storage.RememberParams{
		Topic:      "auth-system",
		Facts:      []string{"Uses JWT tokens", "Tokens expire in 15m"},
		EntityType: "architecture",
		FactType:   storage.FactTypeStatic,
		Project:    "mark42",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}
	if res.Topic != "auth-system" {
		t.Errorf("expected topic auth-system, got %s", res.Topic)
	}
	if res.ObservationsAdded != 2 {
		t.Errorf("expected 2 observations added, got %d", res.ObservationsAdded)
	}

	// Verify entity exists
	entity, err := store.GetEntity("auth-system")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if entity.Type != "architecture" {
		t.Errorf("expected entity type architecture, got %s", entity.Type)
	}
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}

	// Subsequent Remember (upsert/append)
	res2, err := store.Remember(ctx, storage.RememberParams{
		Topic: "auth-system",
		Facts: []string{"Refresh tokens are stored in Redis"},
	})
	if err != nil {
		t.Fatalf("Remember upsert failed: %v", err)
	}
	if res2.ObservationsAdded != 1 {
		t.Errorf("expected 1 observation added, got %d", res2.ObservationsAdded)
	}

	entityAfter, err := store.GetEntity("auth-system")
	if err != nil {
		t.Fatalf("GetEntity after upsert failed: %v", err)
	}
	if len(entityAfter.Observations) != 3 {
		t.Errorf("expected 3 observations after upsert, got %d", len(entityAfter.Observations))
	}
}

func TestStore_Remember_WithRelations(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	res, err := store.Remember(ctx, storage.RememberParams{
		Topic: "api-gateway",
		Facts: []string{"Routes incoming HTTP requests"},
		Relations: []storage.RelationParam{
			{To: "auth-system", Type: "depends_on"},
		},
	})
	if err != nil {
		t.Fatalf("Remember with relations failed: %v", err)
	}
	if res.RelationsCreated != 1 {
		t.Errorf("expected 1 relation created, got %d", res.RelationsCreated)
	}

	relations, err := store.ListRelations("api-gateway")
	if err != nil {
		t.Fatalf("ListRelations failed: %v", err)
	}
	if len(relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(relations))
	}
	if relations[0].To != "auth-system" || relations[0].Type != "depends_on" {
		t.Errorf("unexpected relation: %+v", relations[0])
	}
}

func TestStore_Remember_WithEmbedder(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	embedder := &fakeEmbedder{
		embeddings: map[string][]float64{
			"Fact with embedding": {0.5, 0.6, 0.7},
		},
	}
	store.WithEmbedder(embedder)

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "embedded-topic",
		Facts: []string{"Fact with embedding"},
	})
	if err != nil {
		t.Fatalf("Remember with embedder failed: %v", err)
	}

	obs := store.GetObservationWithID("embedded-topic", "Fact with embedding")
	if obs == nil {
		t.Fatal("expected observation to be found")
	}

	// Verify embedding was stored
	var count int
	err = store.DB().QueryRow("SELECT COUNT(*) FROM observation_embeddings").Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 embedding, got %d", count)
	}
}

func TestStore_Recall_Topic(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "cache-layer",
		Facts: []string{"Redis cluster with 3 nodes", "Eviction policy is LRU"},
		Relations: []storage.RelationParam{
			{To: "database", Type: "caches"},
		},
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	result, err := store.Recall(ctx, storage.RecallParams{
		Topic: "cache-layer",
	})
	if err != nil {
		t.Fatalf("Recall topic failed: %v", err)
	}
	if result.Type != "topic" {
		t.Errorf("expected type topic, got %s", result.Type)
	}
	if result.Topic == nil || result.Topic.Name != "cache-layer" {
		t.Fatalf("expected topic cache-layer, got %+v", result.Topic)
	}
	if len(result.Topic.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(result.Topic.Observations))
	}
	if len(result.Relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(result.Relations))
	}
}

func TestStore_Recall_Query(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "goconventions",
		Facts: []string{"Use table-driven tests for comprehensive coverage", "Avoid global variables"},
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	result, err := store.Recall(ctx, storage.RecallParams{
		Query: "table-driven tests",
	})
	if err != nil {
		t.Fatalf("Recall query failed: %v", err)
	}
	if result.Type != "search" {
		t.Errorf("expected type search, got %s", result.Type)
	}
	if len(result.SearchResults) == 0 {
		t.Errorf("expected search results, got 0")
	}
}

func TestStore_Recall_DefaultContext(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic:   "arch-rules",
		Facts:   []string{"Always validate inputs at the boundary"},
		Project: "mark42",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	result, err := store.Recall(ctx, storage.RecallParams{
		Project: "mark42",
	})
	if err != nil {
		t.Fatalf("Recall default context failed: %v", err)
	}
	if result.Type != "context" {
		t.Errorf("expected type context, got %s", result.Type)
	}
}

func TestStore_Forget_SoftInvalidateFact(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "database-config",
		Facts: []string{"Max connections is 50", "Port is 5432"},
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Soft invalidate one fact
	forgetRes, err := store.Forget(ctx, storage.ForgetParams{
		Topic:     "database-config",
		Fact:      "Max connections is 50",
		Permanent: false,
	})
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if forgetRes.Count != 1 {
		t.Errorf("expected 1 fact forgotten, got %d", forgetRes.Count)
	}
	if forgetRes.Permanent {
		t.Errorf("expected permanent to be false")
	}

	// Recall should no longer show the forgotten fact
	recallRes, err := store.Recall(ctx, storage.RecallParams{Topic: "database-config"})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(recallRes.Topic.Observations) != 1 {
		t.Errorf("expected 1 active observation, got %d", len(recallRes.Topic.Observations))
	}
	if recallRes.Topic.Observations[0] != "Port is 5432" {
		t.Errorf("expected remaining observation to be Port is 5432, got %s", recallRes.Topic.Observations[0])
	}
}

func TestStore_Forget_PermanentDeleteFact(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "secret-data",
		Facts: []string{"Temp token 12345"},
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	forgetRes, err := store.Forget(ctx, storage.ForgetParams{
		Topic:     "secret-data",
		Fact:      "Temp token 12345",
		Permanent: true,
	})
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if forgetRes.Count != 1 {
		t.Errorf("expected 1 count, got %d", forgetRes.Count)
	}

	obs := store.GetObservationWithID("secret-data", "Temp token 12345")
	if obs != nil {
		t.Errorf("expected observation to be permanently deleted, but still found")
	}
}

func TestStore_Forget_EntireTopic(t *testing.T) {
	store := newTestStoreWithDB(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Remember(ctx, storage.RememberParams{
		Topic: "temp-topic",
		Facts: []string{"Fact 1", "Fact 2"},
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Soft invalidate entire topic
	forgetRes, err := store.Forget(ctx, storage.ForgetParams{
		Topic:     "temp-topic",
		Permanent: false,
	})
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}
	if forgetRes.Count != 2 {
		t.Errorf("expected 2 facts forgotten, got %d", forgetRes.Count)
	}

	// Recall topic should have 0 active observations
	recallRes, err := store.Recall(ctx, storage.RecallParams{Topic: "temp-topic"})
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(recallRes.Topic.Observations) != 0 {
		t.Errorf("expected 0 active observations, got %d", len(recallRes.Topic.Observations))
	}

	// Permanent delete entire topic
	forgetPerm, err := store.Forget(ctx, storage.ForgetParams{
		Topic:     "temp-topic",
		Permanent: true,
	})
	if err != nil {
		t.Fatalf("Permanent forget failed: %v", err)
	}
	if forgetPerm.Count < 1 {
		t.Errorf("expected at least 1 entity deleted")
	}

	_, err = store.GetEntity("temp-topic")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound for deleted entity, got %v", err)
	}
}

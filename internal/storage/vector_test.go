package storage

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreEmbedding(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vector.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create entity with observation
	entity, err := store.CreateEntity("user", "person", []string{"prefers typescript"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// Get observation ID
	obsID, err := store.getObservationID(entity.ID, "prefers typescript")
	if err != nil {
		t.Fatalf("failed to get observation ID: %v", err)
	}

	// Store embedding
	embedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	err = store.StoreEmbedding(obsID, embedding, "nomic-embed-text")
	if err != nil {
		t.Fatalf("failed to store embedding: %v", err)
	}

	// Retrieve embedding
	retrieved, err := store.GetEmbedding(obsID)
	if err != nil {
		t.Fatalf("failed to get embedding: %v", err)
	}

	if len(retrieved) != len(embedding) {
		t.Errorf("expected %d dimensions, got %d", len(embedding), len(retrieved))
	}

	for i, v := range retrieved {
		if v != embedding[i] {
			t.Errorf("embedding[%d] = %f, expected %f", i, v, embedding[i])
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        []float64{1, 2, 3},
			b:        []float64{1, 2, 4},
			expected: 0.9914, // Approximate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.expected) > 0.001 {
				t.Errorf("CosineSimilarity() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestVectorSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vector_search.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create entities with observations
	testData := []struct {
		name        string
		entityType  string
		observation string
		embedding   []float64
	}{
		{"user_prefs", "person", "prefers typescript", []float64{0.9, 0.1, 0.0}},
		{"user_tools", "person", "uses neovim", []float64{0.1, 0.9, 0.0}},
		{"project", "code", "written in go", []float64{0.0, 0.0, 0.9}},
	}

	for _, td := range testData {
		entity, err := store.CreateEntity(td.name, td.entityType, []string{td.observation})
		if err != nil {
			t.Fatalf("failed to create entity: %v", err)
		}
		obsID, err := store.getObservationID(entity.ID, td.observation)
		if err != nil {
			t.Fatalf("failed to get observation ID: %v", err)
		}
		if err := store.StoreEmbedding(obsID, td.embedding, "test-model"); err != nil {
			t.Fatalf("failed to store embedding: %v", err)
		}
	}

	// Search for something similar to "prefers typescript" embedding
	queryEmbedding := []float64{0.85, 0.15, 0.0}
	results, err := store.VectorSearch(queryEmbedding, 10)
	if err != nil {
		t.Fatalf("VectorSearch failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First result should be "prefers typescript" (most similar)
	if results[0].Content != "prefers typescript" {
		t.Errorf("expected 'prefers typescript' first, got %q", results[0].Content)
	}
}

func TestVectorSearch_ExcludesSessionEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vector_exclude.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	entity, err := store.CreateEntity("SessEntity", "test", []string{"static content"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	staticID, err := store.getObservationID(entity.ID, "static content")
	if err != nil {
		t.Fatalf("failed to get static observation ID: %v", err)
	}
	if err := store.StoreEmbedding(staticID, []float64{0.9, 0.1, 0.0}, "test-model"); err != nil {
		t.Fatalf("failed to store static embedding: %v", err)
	}

	eventContent := `{"toolName":"Edit","filePath":"/a.go"}`
	if err := store.AddObservationWithType("SessEntity", eventContent, FactTypeSessionEvent); err != nil {
		t.Fatalf("AddObservationWithType failed: %v", err)
	}
	eventID, err := store.getObservationID(entity.ID, eventContent)
	if err != nil {
		t.Fatalf("failed to get event observation ID: %v", err)
	}
	if err := store.StoreEmbedding(eventID, []float64{0.9, 0.1, 0.0}, "test-model"); err != nil {
		t.Fatalf("failed to store event embedding: %v", err)
	}

	results, err := store.VectorSearch([]float64{0.9, 0.1, 0.0}, 10)
	if err != nil {
		t.Fatalf("VectorSearch failed: %v", err)
	}

	foundStatic := false
	for _, r := range results {
		if strings.Contains(r.Content, "toolName") {
			t.Error("session_event should be excluded from vector search")
		}
		if r.Content == "static content" {
			foundStatic = true
		}
	}
	if !foundStatic {
		t.Error("static observation should be present in vector search")
	}
}

func TestVectorSearch_ModelPartitioning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vector_partition.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Entity 1: embedded with "model-v1"
	e1, err := store.CreateEntity("EntityV1", "test", []string{"v1 content"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}
	id1, _ := store.getObservationID(e1.ID, "v1 content")
	if err := store.StoreEmbedding(id1, []float64{0.9, 0.1}, "model-v1"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Entity 2: embedded with "model-v2"
	e2, err := store.CreateEntity("EntityV2", "test", []string{"v2 content"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}
	id2, _ := store.getObservationID(e2.ID, "v2 content")
	if err := store.StoreEmbedding(id2, []float64{0.9, 0.1}, "model-v2"); err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Searching with model-v2 should ONLY return EntityV2, ignoring EntityV1
	results, err := store.VectorSearchWithModel([]float64{0.9, 0.1}, 10, "model-v2")
	if err != nil {
		t.Fatalf("VectorSearchWithModel failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result from model-v2, got %d", len(results))
	}
	if results[0].EntityName != "EntityV2" {
		t.Errorf("expected EntityV2, got %s", results[0].EntityName)
	}
}

func TestHasEmbedding(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_has_embedding.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create entity with observation
	entity, err := store.CreateEntity("test", "thing", []string{"observation"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	obsID, err := store.getObservationID(entity.ID, "observation")
	if err != nil {
		t.Fatalf("failed to get observation ID: %v", err)
	}

	// Should not have embedding initially
	has, err := store.HasEmbedding(obsID)
	if err != nil {
		t.Fatalf("HasEmbedding failed: %v", err)
	}
	if has {
		t.Error("expected no embedding initially")
	}

	// Store embedding
	if err := store.StoreEmbedding(obsID, []float64{0.1, 0.2}, "test-model"); err != nil {
		t.Fatalf("failed to store embedding: %v", err)
	}

	// Should have embedding now
	has, err = store.HasEmbedding(obsID)
	if err != nil {
		t.Fatalf("HasEmbedding failed: %v", err)
	}
	if !has {
		t.Error("expected embedding to exist")
	}
}

func TestBatchStoreEmbeddings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_batch_embed.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	entity, err := store.CreateEntity("batch", "test", []string{"obs one", "obs two"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	id1, err := store.getObservationID(entity.ID, "obs one")
	if err != nil {
		t.Fatalf("get obs1: %v", err)
	}
	id2, err := store.getObservationID(entity.ID, "obs two")
	if err != nil {
		t.Fatalf("get obs2: %v", err)
	}

	obs := []ObservationWithID{{ID: id1}, {ID: id2}}
	embeddings := [][]float64{{0.1, 0.2}, {0.3, 0.4}}

	if err := store.BatchStoreEmbeddings(obs, embeddings, "test-model"); err != nil {
		t.Fatalf("BatchStoreEmbeddings: %v", err)
	}

	_, withEmbeddings, err := store.EmbeddingStats()
	if err != nil {
		t.Fatalf("EmbeddingStats: %v", err)
	}
	if withEmbeddings != 2 {
		t.Errorf("withEmbeddings = %d, want 2", withEmbeddings)
	}

	if err := store.BatchStoreEmbeddings(obs, embeddings[:1], "test-model"); err == nil {
		t.Error("expected error on count mismatch, got nil")
	}
}

func TestGetObservationsWithoutEmbeddings(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test_noembed.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if _, err := store.CreateEntity("embed-check", "test", []string{"needs embedding", "already embedded"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	obs, err := store.GetObservationsWithoutEmbeddings()
	if err != nil {
		t.Fatalf("GetObservationsWithoutEmbeddings: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations without embeddings, got %d", len(obs))
	}

	target := store.GetObservationWithID("embed-check", "already embedded")
	if target == nil {
		t.Fatal("observation not found")
	}
	if err := store.StoreEmbedding(target.ID, []float64{0.1, 0.2}, "test"); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}

	obs, err = store.GetObservationsWithoutEmbeddings()
	if err != nil {
		t.Fatalf("GetObservationsWithoutEmbeddings: %v", err)
	}
	if len(obs) != 1 || obs[0].Content != "needs embedding" {
		t.Errorf("expected only 'needs embedding', got %v", obs)
	}
}

func BenchmarkCosineSimilarity_768Dim(b *testing.B) {
	vecA := make([]float64, 768)
	vecB := make([]float64, 768)
	for i := range vecA {
		vecA[i] = float64(i) / 768.0
		vecB[i] = float64(768-i) / 768.0
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CosineSimilarity(vecA, vecB)
	}
}

func BenchmarkVectorSearch_1000Vectors(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "bench_vector.db"))
	if err != nil {
		b.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Seed 1000 observations with 768-dim embeddings
	obsList := make([]ObservationWithID, 1000)
	embeddings := make([][]float64, 1000)
	for i := 0; i < 1000; i++ {
		content := fmt.Sprintf("observation content for benchmark vector item %d", i)
		entity, err := store.CreateEntity(fmt.Sprintf("entity_%d", i), "benchmark", []string{content})
		if err != nil {
			b.Fatalf("CreateEntity failed: %v", err)
		}
		obsID, err := store.getObservationID(entity.ID, content)
		if err != nil {
			b.Fatalf("getObservationID failed: %v", err)
		}
		obsList[i] = ObservationWithID{ID: obsID}

		vec := make([]float64, 768)
		vec[i%768] = 1.0
		embeddings[i] = vec
	}

	if err := store.BatchStoreEmbeddings(obsList, embeddings, "nomic-embed-text"); err != nil {
		b.Fatalf("BatchStoreEmbeddings failed: %v", err)
	}

	query := make([]float64, 768)
	query[0] = 1.0

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		results, err := store.VectorSearch(query, 10)
		if err != nil {
			b.Fatalf("VectorSearch failed: %v", err)
		}
		if len(results) == 0 {
			b.Fatal("expected results")
		}
	}
}

package storage

import (
	"math"
	"path/filepath"
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

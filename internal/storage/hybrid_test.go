package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestHybridSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_hybrid.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create test data
	testData := []struct {
		name        string
		entityType  string
		observation string
		embedding   []float64
	}{
		{"user_lang", "person", "prefers typescript programming", []float64{0.9, 0.1, 0.0}},
		{"user_editor", "person", "uses neovim editor daily", []float64{0.1, 0.9, 0.0}},
		{"codebase", "project", "written in go language", []float64{0.0, 0.0, 0.9}},
		{"user_workflow", "person", "likes test driven development", []float64{0.3, 0.3, 0.4}},
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

	// Test hybrid search with query embedding
	queryEmbedding := []float64{0.85, 0.15, 0.0} // Similar to "prefers typescript"
	results, err := store.HybridSearch(context.Background(), "typescript", queryEmbedding, 10)
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// "prefers typescript programming" should rank high (matches FTS + vector)
	foundTypescript := false
	for _, r := range results {
		if r.Content == "prefers typescript programming" {
			foundTypescript = true
			// Should have source scores from both strategies
			if len(r.SourceScores) == 0 {
				t.Error("expected source scores")
			}
			break
		}
	}
	if !foundTypescript {
		t.Error("expected to find 'prefers typescript programming' in results")
	}
}

func TestHybridSearch_FTSOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_hybrid_fts.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create entity without embedding
	_, err = store.CreateEntity("user", "person", []string{"prefers typescript"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// Search without query embedding (FTS only)
	results, err := store.HybridSearch(context.Background(), "typescript", nil, 10)
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Content != "prefers typescript" {
		t.Errorf("expected 'prefers typescript', got %q", results[0].Content)
	}
}

func TestHybridSearch_VectorOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_hybrid_vector.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create entity with embedding
	entity, err := store.CreateEntity("user", "person", []string{"prefers typescript"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	obsID, err := store.getObservationID(entity.ID, "prefers typescript")
	if err != nil {
		t.Fatalf("failed to get observation ID: %v", err)
	}

	if err := store.StoreEmbedding(obsID, []float64{0.9, 0.1, 0.0}, "test-model"); err != nil {
		t.Fatalf("failed to store embedding: %v", err)
	}

	// Search with only vector (no FTS query)
	queryEmbedding := []float64{0.85, 0.15, 0.0}
	results, err := store.HybridSearch(context.Background(), "", queryEmbedding, 10)
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

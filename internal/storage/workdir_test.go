package storage_test

import (
	"context"
	"testing"
)

func TestStore_SetContainerTag(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity
	store.CreateEntity("TestEntity", "project", []string{"obs1"})

	// Set container tag
	err := store.SetContainerTag("TestEntity", "mark42")
	if err != nil {
		t.Fatalf("SetContainerTag failed: %v", err)
	}

	// Verify tag was set
	tag, err := store.GetContainerTag("TestEntity")
	if err != nil {
		t.Fatalf("GetContainerTag failed: %v", err)
	}

	if tag != "mark42" {
		t.Errorf("expected tag 'mark42', got %q", tag)
	}
}

func TestStore_SetContainerTag_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	err := store.SetContainerTag("NonExistent", "tag")
	if err == nil {
		t.Error("expected error for non-existent entity")
	}
}

func TestStore_GetEntitiesByContainerTag(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entities with different tags
	store.CreateEntity("Project1", "project", []string{"obs1"})
	store.CreateEntity("Project2", "project", []string{"obs2"})
	store.CreateEntity("Global", "project", []string{"obs3"})

	store.SetContainerTag("Project1", "mark42")
	store.SetContainerTag("Project2", "mark42")
	// Global has no tag

	// Get entities by tag
	entities, err := store.GetEntitiesByContainerTag("mark42")
	if err != nil {
		t.Fatalf("GetEntitiesByContainerTag failed: %v", err)
	}

	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(entities))
	}
}

func TestStore_HybridSearchWithContainerBoost(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entities with observations
	store.CreateEntity("Go", "language", []string{"Go is a statically typed language"})
	store.CreateEntity("Python", "language", []string{"Python is a dynamically typed language"})

	// Set container tags
	store.SetContainerTag("Go", "mark42")
	// Python has no tag

	// Search with container boost
	results, err := store.HybridSearchWithBoost(
		context.Background(),
		"typed language",
		nil, // no embedding
		10,
		"mark42", // boost results in this container
		1.5,      // boost factor
	)
	if err != nil {
		t.Fatalf("HybridSearchWithBoost failed: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Go should rank higher due to container boost
	if results[0].EntityName != "Go" {
		t.Errorf("expected Go to be first (boosted), got %s", results[0].EntityName)
	}
}

func TestStore_GetContextWithContainerTag(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entities
	store.CreateEntity("LocalPattern", "pattern", []string{"Local project pattern"})
	store.CreateEntity("GlobalPattern", "pattern", []string{"Global pattern"})

	// Set container tag for local entity
	store.SetContainerTag("LocalPattern", "my-project")

	// Get context with container tag filtering/boosting
	cfg := store.DefaultContextConfig()
	results, err := store.GetContextWithContainerTag(cfg, "my-project")
	if err != nil {
		t.Fatalf("GetContextWithContainerTag failed: %v", err)
	}

	// Should include both, but local should have higher score
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}

	// Find LocalPattern and check its score is boosted
	var localScore, globalScore float64
	for _, r := range results {
		if r.EntityName == "LocalPattern" {
			localScore = r.FinalScore
		}
		if r.EntityName == "GlobalPattern" {
			globalScore = r.FinalScore
		}
	}

	if localScore <= globalScore {
		t.Errorf("expected LocalPattern (%.2f) to have higher score than GlobalPattern (%.2f)", localScore, globalScore)
	}
}

func TestStore_CreateEntityWithContainerTag(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with container tag in one call
	entity, err := store.CreateEntityWithContainer("LocalEntity", "type", []string{"obs"}, "my-project")
	if err != nil {
		t.Fatalf("CreateEntityWithContainer failed: %v", err)
	}

	if entity.Name != "LocalEntity" {
		t.Errorf("expected name LocalEntity, got %s", entity.Name)
	}

	// Verify tag was set
	tag, err := store.GetContainerTag("LocalEntity")
	if err != nil {
		t.Fatalf("GetContainerTag failed: %v", err)
	}

	if tag != "my-project" {
		t.Errorf("expected tag 'my-project', got %q", tag)
	}
}

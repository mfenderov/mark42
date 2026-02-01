package storage_test

import (
	"testing"
)

func TestSearch_ByObservationContent(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{
		"Test-Driven Development methodology",
		"Red-Green-Refactor cycle",
	})
	store.CreateEntity("BDD", "pattern", []string{
		"Behavior-Driven Development",
	})
	store.CreateEntity("konfig", "project", []string{
		"Go configuration library",
	})

	// Search should find entities with matching observations
	results, err := store.Search("development")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results for 'development', got %d", len(results))
	}

	// Verify TDD and BDD are in results
	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
	}
	if !names["TDD"] {
		t.Error("expected TDD in results")
	}
	if !names["BDD"] {
		t.Error("expected BDD in results")
	}
}

func TestSearch_ByEntityName(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"some observation"})
	store.CreateEntity("konfig", "project", []string{"other observation"})

	// Search by entity name
	results, err := store.Search("TDD")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result for 'TDD', got %d", len(results))
	}
	if results[0].Name != "TDD" {
		t.Errorf("expected TDD, got %q", results[0].Name)
	}
}

func TestSearch_Stemming(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Testing", "pattern", []string{
		"Unit testing is important",
		"Tests should be fast",
	})

	// Porter stemming should match "test" to "testing" and "tests"
	results, err := store.Search("test")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result for 'test' (via stemming), got %d", len(results))
	}
}

func TestSearch_NoResults(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})

	results, err := store.Search("nonexistent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_WithLimit(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Create many entities
	for i := 0; i < 10; i++ {
		store.CreateEntity(
			"pattern"+string(rune('A'+i)),
			"pattern",
			[]string{"common keyword here"},
		)
	}

	results, err := store.SearchWithLimit("keyword", 3)
	if err != nil {
		t.Fatalf("SearchWithLimit failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results with limit, got %d", len(results))
	}
}

func TestSearch_IncludesObservations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{
		"Test-Driven Development",
		"Red-Green-Refactor",
	})

	results, err := store.Search("TDD")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Results should include observations
	if len(results[0].Observations) != 2 {
		t.Errorf("expected 2 observations in result, got %d", len(results[0].Observations))
	}
}

func TestReadGraph(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	store.CreateEntity("Simple Design", "pattern", []string{"Kent Beck's rules"})
	store.CreateRelation("TDD", "Simple Design", "enables")

	graph, err := store.ReadGraph()
	if err != nil {
		t.Fatalf("ReadGraph failed: %v", err)
	}

	if len(graph.Entities) != 2 {
		t.Errorf("expected 2 entities in graph, got %d", len(graph.Entities))
	}
	if len(graph.Relations) != 1 {
		t.Errorf("expected 1 relation in graph, got %d", len(graph.Relations))
	}
}

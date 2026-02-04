package storage_test

import (
	"strings"
	"testing"

	"github.com/mfenderov/claude-memory/internal/storage"
)

func TestContextConfig_Defaults(t *testing.T) {
	cfg := storage.DefaultContextConfig()

	if cfg.TokenBudget <= 0 {
		t.Error("TokenBudget should be positive")
	}
	if cfg.MinImportance < 0 || cfg.MinImportance > 1 {
		t.Error("MinImportance should be between 0 and 1")
	}
	if len(cfg.FactTypePriority) == 0 {
		t.Error("FactTypePriority should not be empty")
	}
}

func TestStore_GetContextForInjection(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create test data
	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	store.CreateEntity("konfig", "project", []string{"Configuration library"})

	// Set importance scores
	store.SetObservationImportance("TDD", "Test-Driven Development", 0.8)
	store.SetObservationImportance("konfig", "Configuration library", 0.5)

	cfg := storage.DefaultContextConfig()
	cfg.MinImportance = 0.3

	results, err := store.GetContextForInjection(cfg, "")
	if err != nil {
		t.Fatalf("GetContextForInjection failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected some results")
	}
}

func TestStore_GetContextForInjection_TokenBudget(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create lots of test data
	for i := 0; i < 20; i++ {
		name := "Entity" + string(rune('A'+i))
		store.CreateEntity(name, "test", []string{"Some observation content for " + name})
		store.SetObservationImportance(name, "Some observation content for "+name, 0.8)
	}

	cfg := storage.DefaultContextConfig()
	cfg.TokenBudget = 200 // Very small budget
	cfg.MinImportance = 0.3

	results, err := store.GetContextForInjection(cfg, "")
	if err != nil {
		t.Fatalf("GetContextForInjection failed: %v", err)
	}

	// Should be limited by token budget
	if len(results) >= 20 {
		t.Errorf("expected fewer than 20 results due to token budget, got %d", len(results))
	}
}

func TestStore_GetContextForInjection_ProjectBoost(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create test data
	store.CreateEntity("claude-memory", "project", []string{"Memory system for Claude"})
	store.CreateEntity("other-project", "project", []string{"Some other project"})

	store.SetObservationImportance("claude-memory", "Memory system for Claude", 0.5)
	store.SetObservationImportance("other-project", "Some other project", 0.5)

	cfg := storage.DefaultContextConfig()
	cfg.MinImportance = 0.3
	cfg.ProjectBoost = 2.0

	results, err := store.GetContextForInjection(cfg, "claude-memory")
	if err != nil {
		t.Fatalf("GetContextForInjection failed: %v", err)
	}

	// Find the claude-memory result
	var claudeScore, otherScore float64
	for _, r := range results {
		if r.EntityName == "claude-memory" {
			claudeScore = r.FinalScore
		}
		if r.EntityName == "other-project" {
			otherScore = r.FinalScore
		}
	}

	// claude-memory should have higher final score due to project boost
	if claudeScore <= otherScore {
		t.Errorf("expected claude-memory score (%v) > other-project score (%v) due to project boost",
			claudeScore, otherScore)
	}
}

func TestFormatContextResults(t *testing.T) {
	results := []storage.ContextResult{
		{
			EntityName: "TDD",
			EntityType: "pattern",
			Content:    "Test-Driven Development",
			FactType:   "static",
			Importance: 0.9,
		},
		{
			EntityName: "konfig",
			EntityType: "project",
			Content:    "Configuration library",
			FactType:   "dynamic",
			Importance: 0.7,
		},
	}

	formatted := storage.FormatContextResults(results)

	// Should contain the entity names
	if !strings.Contains(formatted, "TDD") {
		t.Error("formatted output should contain 'TDD'")
	}
	if !strings.Contains(formatted, "konfig") {
		t.Error("formatted output should contain 'konfig'")
	}

	// Should have section headers
	if !strings.Contains(formatted, "[STATIC]") {
		t.Error("formatted output should contain '[STATIC]' section")
	}
	if !strings.Contains(formatted, "[DYNAMIC]") {
		t.Error("formatted output should contain '[DYNAMIC]' section")
	}
}

func TestFormatContextResults_Empty(t *testing.T) {
	formatted := storage.FormatContextResults(nil)
	if formatted != "" {
		t.Error("empty results should produce empty string")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text      string
		minTokens int
		maxTokens int
	}{
		{"", 0, 0},
		{"Hello world!", 2, 4},
		{"This is a longer text that should have more tokens", 10, 15},
	}

	for _, tt := range tests {
		tokens := storage.EstimateTokens(tt.text)
		if tokens < tt.minTokens || tokens > tt.maxTokens {
			t.Errorf("EstimateTokens(%q) = %d, want between %d and %d",
				tt.text, tokens, tt.minTokens, tt.maxTokens)
		}
	}
}

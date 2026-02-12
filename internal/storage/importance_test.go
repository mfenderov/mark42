package storage_test

import (
	"testing"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestImportanceConfig_Defaults(t *testing.T) {
	cfg := storage.DefaultImportanceConfig()

	if cfg.DecayConstant <= 0 {
		t.Error("DecayConstant should be positive")
	}
	if cfg.RecencyWeight <= 0 || cfg.RecencyWeight > 1 {
		t.Error("RecencyWeight should be between 0 and 1")
	}
	if cfg.FrequencyWeight <= 0 || cfg.FrequencyWeight > 1 {
		t.Error("FrequencyWeight should be between 0 and 1")
	}
	if cfg.CentralityWeight <= 0 || cfg.CentralityWeight > 1 {
		t.Error("CentralityWeight should be between 0 and 1")
	}
}

func TestRecencyDecay(t *testing.T) {
	cfg := storage.DefaultImportanceConfig()

	tests := []struct {
		name         string
		daysSince    float64
		wantMinDecay float64 // Minimum acceptable decay
		wantMaxDecay float64 // Maximum acceptable decay
	}{
		{
			name:         "just accessed",
			daysSince:    0,
			wantMinDecay: 0.99,
			wantMaxDecay: 1.0,
		},
		{
			name:         "accessed 7 days ago",
			daysSince:    7,
			wantMinDecay: 0.7,
			wantMaxDecay: 0.85,
		},
		{
			name:         "accessed 30 days ago",
			daysSince:    30,
			wantMinDecay: 0.3,
			wantMaxDecay: 0.4,
		},
		{
			name:         "accessed 90 days ago",
			daysSince:    90,
			wantMinDecay: 0.04,
			wantMaxDecay: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decay := storage.CalculateRecencyDecay(tt.daysSince, cfg.DecayConstant)
			if decay < tt.wantMinDecay || decay > tt.wantMaxDecay {
				t.Errorf("recency decay for %v days = %v, want between %v and %v",
					tt.daysSince, decay, tt.wantMinDecay, tt.wantMaxDecay)
			}
		})
	}
}

func TestAccessFrequencyScore(t *testing.T) {
	tests := []struct {
		name        string
		accessCount int
		wantMin     float64
		wantMax     float64
	}{
		{
			name:        "never accessed",
			accessCount: 0,
			wantMin:     0.9,
			wantMax:     1.1, // log(1)/10 = 0, so score = 1.0
		},
		{
			name:        "accessed once",
			accessCount: 1,
			wantMin:     1.0,
			wantMax:     1.1, // log(2)/10 ≈ 0.069, so score ≈ 1.07
		},
		{
			name:        "accessed 10 times",
			accessCount: 10,
			wantMin:     1.2,
			wantMax:     1.3, // log(11)/10 ≈ 0.24, so score ≈ 1.24
		},
		{
			name:        "accessed 100 times",
			accessCount: 100,
			wantMin:     1.4,
			wantMax:     1.5, // log(101)/10 ≈ 0.46, so score ≈ 1.46
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := storage.CalculateFrequencyScore(tt.accessCount)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("frequency score for %v accesses = %v, want between %v and %v",
					tt.accessCount, score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCentralityScore(t *testing.T) {
	tests := []struct {
		name          string
		relationCount int
		maxRelations  int
		wantMin       float64
		wantMax       float64
	}{
		{
			name:          "no relations",
			relationCount: 0,
			maxRelations:  10,
			wantMin:       0.5,
			wantMax:       0.6,
		},
		{
			name:          "half of max relations",
			relationCount: 5,
			maxRelations:  10,
			wantMin:       0.7,
			wantMax:       0.8,
		},
		{
			name:          "max relations",
			relationCount: 10,
			maxRelations:  10,
			wantMin:       0.95,
			wantMax:       1.05,
		},
		{
			name:          "no relations in empty graph",
			relationCount: 0,
			maxRelations:  0,
			wantMin:       0.5,
			wantMax:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := storage.CalculateCentralityScore(tt.relationCount, tt.maxRelations)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("centrality score for %v/%v relations = %v, want between %v and %v",
					tt.relationCount, tt.maxRelations, score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalculateImportance(t *testing.T) {
	cfg := storage.DefaultImportanceConfig()

	tests := []struct {
		name          string
		baseScore     float64
		daysSince     float64
		accessCount   int
		relationCount int
		maxRelations  int
		wantMin       float64
		wantMax       float64
	}{
		{
			name:          "fresh high-value memory",
			baseScore:     1.0,
			daysSince:     0,
			accessCount:   5,
			relationCount: 3,
			maxRelations:  5,
			wantMin:       0.8,
			wantMax:       1.2,
		},
		{
			name:          "stale low-value memory",
			baseScore:     0.5,
			daysSince:     60,
			accessCount:   0,
			relationCount: 0,
			maxRelations:  5,
			wantMin:       0.05,
			wantMax:       0.3,
		},
		{
			name:          "frequently accessed old memory",
			baseScore:     1.0,
			daysSince:     30,
			accessCount:   50,
			relationCount: 2,
			maxRelations:  5,
			wantMin:       0.5,
			wantMax:       0.85, // High frequency compensates for age
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := storage.CalculateImportance(
				tt.baseScore,
				tt.daysSince,
				tt.accessCount,
				tt.relationCount,
				tt.maxRelations,
				cfg,
			)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("importance score = %v, want between %v and %v", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestStore_UpdateLastAccessed(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Run migrations to get the importance fields
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with observation
	_, err := store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	if err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	// Update last accessed
	err = store.UpdateLastAccessed("TDD")
	if err != nil {
		t.Fatalf("UpdateLastAccessed failed: %v", err)
	}

	// Verify last_accessed was updated (should be recent)
	accessed, err := store.GetLastAccessed("TDD")
	if err != nil {
		t.Fatalf("GetLastAccessed failed: %v", err)
	}

	// Should be within last minute
	if time.Since(accessed) > time.Minute {
		t.Errorf("last_accessed should be recent, got %v ago", time.Since(accessed))
	}
}

func TestStore_RecalculateImportance(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entities with relations
	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	store.CreateEntity("konfig", "project", []string{"Config library"})
	store.CreateEntity("mark42", "project", []string{"Memory system"})
	store.CreateRelation("TDD", "konfig", "used_by")
	store.CreateRelation("TDD", "mark42", "used_by")

	// Recalculate importance
	updated, err := store.RecalculateImportance()
	if err != nil {
		t.Fatalf("RecalculateImportance failed: %v", err)
	}

	if updated == 0 {
		t.Error("expected some observations to be updated")
	}
}

func TestStore_GetObservationsByImportance(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entities
	store.CreateEntity("HighValue", "pattern", []string{"Important fact"})
	store.CreateEntity("LowValue", "pattern", []string{"Less important fact"})

	// Set different importance scores
	store.SetObservationImportance("HighValue", "Important fact", 0.9)
	store.SetObservationImportance("LowValue", "Less important fact", 0.1)

	// Get by importance threshold
	observations, err := store.GetObservationsByImportance(0.5)
	if err != nil {
		t.Fatalf("GetObservationsByImportance failed: %v", err)
	}

	// Should only return high value
	if len(observations) != 1 {
		t.Errorf("expected 1 observation above threshold, got %d", len(observations))
	}

	if len(observations) > 0 && observations[0].Content != "Important fact" {
		t.Errorf("expected 'Important fact', got %q", observations[0].Content)
	}
}

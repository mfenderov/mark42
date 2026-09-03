package storage

import (
	"fmt"
	"testing"
)

func TestGetMemoryAnalytics(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	// Seed: entity with 2 observations, one backdated 100 days, one accessed
	if _, err := store.CreateEntity("Alpha", "project", []string{"fresh obs", "ancient obs"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if _, err := store.CreateEntity("Beta", "person", []string{"beta obs"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := store.CreateRelation("Alpha", "Beta", "knows"); err != nil {
		t.Fatalf("CreateRelation: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE observations SET created_at = datetime('now', '-100 days') WHERE content = 'ancient obs'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE observations SET access_count = 5, last_accessed = datetime('now') WHERE content = 'fresh obs'`); err != nil {
		t.Fatalf("set access: %v", err)
	}
	if _, err := store.CreateSession("proj"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	a, err := store.GetMemoryAnalytics(10)
	if err != nil {
		t.Fatalf("GetMemoryAnalytics: %v", err)
	}

	if a.TotalEntities != 3 { // Alpha, Beta, session
		t.Errorf("TotalEntities = %d, want 3", a.TotalEntities)
	}
	if a.TotalObservations != 3 {
		t.Errorf("TotalObservations = %d, want 3", a.TotalObservations)
	}
	if a.TotalRelations != 1 {
		t.Errorf("TotalRelations = %d, want 1", a.TotalRelations)
	}
	if a.TotalSessions != 1 {
		t.Errorf("TotalSessions = %d, want 1", a.TotalSessions)
	}
	if len(a.DecayCurve) != 4 || a.DecayCurve[3].Count != 1 { // "90d+" bucket holds ancient obs
		t.Errorf("DecayCurve = %+v, want 4 buckets with 1 obs in 90d+", a.DecayCurve)
	}
	if len(a.TopAccessed) != 1 || a.TopAccessed[0].Content != "fresh obs" || a.TopAccessed[0].AccessCount != 5 {
		t.Errorf("TopAccessed = %+v", a.TopAccessed)
	}
	if a.NeverAccessed != 2 {
		t.Errorf("NeverAccessed = %d, want 2", a.NeverAccessed)
	}
	if a.FactTypeCounts["dynamic"] != 3 {
		t.Errorf("FactTypeCounts = %v", a.FactTypeCounts)
	}
}

func TestGetMemoryAnalytics_EmptyDB(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	a, err := store.GetMemoryAnalytics(10)
	if err != nil {
		t.Fatalf("empty DB should not error: %v", err)
	}
	if a.TotalEntities != 0 || len(a.TopAccessed) != 0 {
		t.Errorf("expected zero analytics, got %+v", a)
	}
}

func TestRecommendTuning_FromAccessPatterns(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	// 10 observations, re-accessed on average every ~5 days
	// (created 20 days ago, access_count 4 → 20/4 = 5 days interval)
	for i := 0; i < 10; i++ {
		if _, err := store.CreateEntity(fmt.Sprintf("E%d", i), "fact", []string{"obs"}); err != nil {
			t.Fatalf("CreateEntity: %v", err)
		}
	}
	if _, err := store.db.Exec(`
		UPDATE observations
		SET created_at = datetime('now', '-20 days'),
		    last_accessed = datetime('now'),
		    access_count = 4
	`); err != nil {
		t.Fatalf("seed access: %v", err)
	}

	rec, err := store.RecommendTuning()
	if err != nil {
		t.Fatalf("RecommendTuning: %v", err)
	}

	// median interval 5 days → suggested decay constant = clamp(15, 7, 90) = 15
	if rec.Suggested.DecayConstant != 15 {
		t.Errorf("SuggestedDecayConstant = %v, want 15", rec.Suggested.DecayConstant)
	}
	if len(rec.Rationale) == 0 {
		t.Error("expected rationale for the suggestion")
	}
	if !rec.Drift { // 30 → 15 is a 50% change
		t.Error("expected drift=true when suggestion diverges > 25%")
	}
}

func TestRecommendTuning_InsufficientData(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	rec, err := store.RecommendTuning()
	if err != nil {
		t.Fatalf("RecommendTuning on empty DB: %v", err)
	}
	if rec.Suggested.DecayConstant != rec.Current.DecayConstant {
		t.Errorf("expected current value kept with no access data, got %v", rec.Suggested.DecayConstant)
	}
	if rec.Drift {
		t.Error("expected drift=false on empty DB")
	}
}

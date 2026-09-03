package storage

import "testing"

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

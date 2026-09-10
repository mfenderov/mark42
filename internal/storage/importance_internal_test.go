package storage

import (
	"testing"
)

func TestRecalculateImportance_UsesPersistedConfig(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Persist an extreme decay constant: with DecayConstant=1, a 30-day-old
	// observation decays to ~0 (exp(-30)), vs default 30d constant (exp(-1)).
	custom := DefaultImportanceConfig()
	custom.DecayConstant = 1
	if err := store.SetImportanceConfig(custom); err != nil {
		t.Fatalf("SetImportanceConfig: %v", err)
	}

	if _, err := store.CreateEntity("Old", "fact", []string{"ancient observation"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	// Backdate the observation 30 days
	if _, err := store.db.Exec(`UPDATE observations SET created_at = datetime('now', '-30 days')`); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, err := store.RecalculateImportance(); err != nil {
		t.Fatalf("RecalculateImportance: %v", err)
	}

	// Assert: with DecayConstant=1 the recency factor is exp(-30)≈9e-14,
	// so final importance < 0.01; with default config it stays > 0.2.
	var imp float64
	err = store.db.Get(&imp, "SELECT importance FROM observations WHERE content = ?", "ancient observation")
	if err != nil {
		t.Fatalf("get importance: %v", err)
	}

	if imp > 0.5 {
		t.Errorf("expected heavy decay with DecayConstant=1, got importance %v", imp)
	}
}

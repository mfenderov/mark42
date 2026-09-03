package storage

import (
	"testing"
)

func TestSettings_RoundTrip(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	// Unset key: defaults, no error
	cfg := store.GetImportanceConfig()
	if cfg != DefaultImportanceConfig() {
		t.Errorf("expected defaults for unset config, got %+v", cfg)
	}

	// Persist and re-read
	custom := ImportanceConfig{DecayConstant: 14, RecencyWeight: 0.5, FrequencyWeight: 0.3, CentralityWeight: 0.2}
	if err := store.SetImportanceConfig(custom); err != nil {
		t.Fatalf("SetImportanceConfig: %v", err)
	}
	if got := store.GetImportanceConfig(); got != custom {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, custom)
	}

	// DecayConfig round-trip
	dc := DecayConfig{SoftDecayThreshold: 0.2, ArchiveAfterDays: 60, ForgetAfterDays: 120, MinImportanceToKeep: 0.05}
	if err := store.SetDecayConfig(dc); err != nil {
		t.Fatalf("SetDecayConfig: %v", err)
	}
	if got := store.GetDecayConfig(); got != dc {
		t.Errorf("decay round-trip mismatch: got %+v, want %+v", got, dc)
	}
}

func TestSettings_MalformedJSONFallsBackToDefaults(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	if err := store.SetSetting("importance_config", "{not json"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := store.GetImportanceConfig(); got != DefaultImportanceConfig() {
		t.Errorf("malformed JSON should yield defaults, got %+v", got)
	}
}

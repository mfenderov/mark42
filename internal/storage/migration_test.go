package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// ExpectedMigrationCount is the total number of goose migrations.
// Update this when adding new migrations.
const ExpectedMigrationCount int64 = 6

func TestMigrate_CreatesSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_migrate.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Run migrations
	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Check version was recorded
	version, err := store.GetSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	// Should be at latest migration
	if version != ExpectedMigrationCount {
		t.Errorf("expected version %d, got %d", ExpectedMigrationCount, version)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_migrate_idempotent.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Run migrations twice
	if err := store.Migrate(); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Second run should be no-op
	if err := store.Migrate(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	version, err := store.GetSchemaVersion()
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != ExpectedMigrationCount {
		t.Errorf("expected version %d after two runs, got %d", ExpectedMigrationCount, version)
	}
}

func TestMigrate_AddsFactTypeColumn(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_fact_type.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create an entity with observation
	_, err = store.CreateEntity("test", "thing", []string{"test observation"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// Verify fact_type column exists and defaults to 'dynamic'
	var factType string
	err = store.db.QueryRow(`
		SELECT fact_type FROM observations WHERE content = 'test observation'
	`).Scan(&factType)
	if err != nil {
		t.Fatalf("failed to query fact_type: %v", err)
	}

	if factType != "dynamic" {
		t.Errorf("expected fact_type 'dynamic', got %q", factType)
	}
}

func TestMigrate_AddsVersioningColumns(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_versioning.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Create an entity
	_, err = store.CreateEntity("versioned", "item", nil)
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}

	// Verify versioning columns exist
	var version int
	var isLatest bool
	err = store.db.QueryRow(`
		SELECT version, is_latest FROM entities WHERE name = 'versioned'
	`).Scan(&version, &isLatest)
	if err != nil {
		t.Fatalf("failed to query version columns: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if !isLatest {
		t.Error("expected is_latest to be true")
	}
}

func TestMigrate_PersistsAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_persist.db")

	// First open and migrate
	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}
	if err := store1.Migrate(); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	store1.Close()

	// Reopen - migrations should be no-op
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create second store: %v", err)
	}
	defer store2.Close()

	if err := store2.Migrate(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	version, _ := store2.GetSchemaVersion()
	if version != ExpectedMigrationCount {
		t.Errorf("expected version %d, got %d", ExpectedMigrationCount, version)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

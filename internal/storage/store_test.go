package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mfenderov/claude-memory/internal/storage"
)

func TestNewStore_CreatesDatabase(t *testing.T) {
	// Setup: create temp directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Act: create store
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Assert: database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNewStore_CreatesSchema(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Verify entities table exists
	tables := store.ListTables()

	expectedTables := []string{"entities", "observations", "relations", "observations_fts", "entities_fts"}
	for _, expected := range expectedTables {
		found := false
		for _, table := range tables {
			if table == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected table %q not found, got tables: %v", expected, tables)
		}
	}
}

func TestStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Close should not error
	if err := store.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// Helper to create a test store
func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	return store
}

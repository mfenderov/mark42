package storage

import (
	"path/filepath"
	"testing"
)

func TestNewStore_ConcurrencyPragmas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "pragma_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var busyTimeout int
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout < 5000 {
		t.Errorf("expected busy_timeout >= 5000, got %d", busyTimeout)
	}

	var syncMode int
	if err := store.db.QueryRow("PRAGMA synchronous").Scan(&syncMode); err != nil {
		t.Fatalf("failed to query synchronous: %v", err)
	}
	// 1 corresponds to NORMAL in SQLite
	if syncMode != 1 {
		t.Errorf("expected synchronous = 1 (NORMAL), got %d", syncMode)
	}
}

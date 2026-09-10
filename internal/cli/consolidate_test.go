package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidateCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	store.CreateEntity("Go", "language", []string{
		"Compiled language",
		"Go is a compiled language with fast build times",
		"Has goroutines",
	})
	store.Close()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	if err := consolidateCmd.RunE(consolidateCmd, nil); err != nil {
		t.Fatalf("consolidate command failed: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Consolidated 1 redundant observations") {
		t.Errorf("expected count output, got: %s", got)
	}

	store2, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}
	defer store2.Close()

	entity, err := store2.GetEntity("Go")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations after consolidate, got %d: %v", len(entity.Observations), entity.Observations)
	}
}

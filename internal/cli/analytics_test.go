package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyticsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldDBPath := dbPath
	dbPath = filepath.Join(tmpDir, "test.db")
	defer func() { dbPath = oldDBPath }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore: %v", err)
	}
	if _, err := store.CreateEntity("Alpha", "project", []string{"hello"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	store.Close()

	if err := analyticsCmd.RunE(analyticsCmd, nil); err != nil {
		t.Fatalf("analytics command: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"Memory Analytics", "Entities: 1", "Decay Curve", "Top Memories"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n%s", want, got)
		}
	}
}

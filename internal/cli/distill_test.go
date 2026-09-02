package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestDistillCommand(t *testing.T) {
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
	session, err := store.CreateSession("test-project")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	store.CaptureSessionEvent(session.Name, storage.SessionEvent{ToolName: "Edit", FilePath: "/a.go"})
	store.CaptureSessionEvent(session.Name, storage.SessionEvent{ToolName: "Bash", Command: "go test"})
	store.Close()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	if err := distillCmd.RunE(distillCmd, []string{session.Name}); err != nil {
		t.Fatalf("distill command failed: %v", err)
	}

	store2, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}
	defer store2.Close()

	s, err := store2.GetSession(session.Name)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if s.Summary == "" {
		t.Error("expected non-empty summary after distill")
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionCapture_WritesCurrentSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	t.Setenv("CLAUDE_PROJECT_DIR", projectDir)

	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")
	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.WriteString(`{"summary":"x","events":[]}`)
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	if err := sessionCaptureCmd.RunE(sessionCaptureCmd, []string{"testproject"}); err != nil {
		t.Fatalf("capture failed: %v", err)
	}

	data, err := os.ReadFile(currentSessionPath(projectDir))
	if err != nil {
		t.Fatalf("current-session file not created: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "session-testproject-") {
		t.Errorf("unexpected session name: %q", strings.TrimSpace(string(data)))
	}
}

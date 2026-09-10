package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/state"
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

	data, err := os.ReadFile(state.CurrentSessionPath(projectDir))
	if err != nil {
		t.Fatalf("current-session file not created: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "session-testproject-") {
		t.Errorf("unexpected session name: %q", strings.TrimSpace(string(data)))
	}
}

func TestSessionCapture_ValidatesEmptySummary(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test_validation.db")
	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.WriteString(`{"summary":"","events":[]}`)
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	err := sessionCaptureCmd.RunE(sessionCaptureCmd, []string{"testproject"})
	if err == nil {
		t.Fatal("expected error for empty summary, got nil")
	}
	if !strings.Contains(err.Error(), "summary is required") {
		t.Errorf("expected error mentioning 'summary is required', got: %v", err)
	}
}

func TestSessionCapture_ValidatesEmptyEventToolName(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test_validation_event.db")
	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.WriteString(`{"summary":"valid summary","events":[{"toolName":"","filePath":"/a.go"}]}`)
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	err := sessionCaptureCmd.RunE(sessionCaptureCmd, []string{"testproject"})
	if err == nil {
		t.Fatal("expected error for empty event toolName, got nil")
	}
	if !strings.Contains(err.Error(), "toolName") {
		t.Errorf("expected error mentioning 'toolName', got: %v", err)
	}
}

func TestSessionCapture_JSONFlag(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test_json_capture.db")
	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.WriteString(`{"summary":"valid json summary","events":[{"toolName":"Edit","filePath":"/main.go"}]}`)
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"session", "capture", "testproject", "--json"})
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("session capture --json failed: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("expected JSON output from --json flag, got: %s (err: %v)", buf.String(), err)
	}
	if res["status"] != "completed" {
		t.Errorf("expected status 'completed', got: %v", res["status"])
	}
	if res["project"] != "testproject" {
		t.Errorf("expected project 'testproject', got: %v", res["project"])
	}
	if res["sessionName"] == "" {
		t.Errorf("expected non-empty sessionName in JSON output")
	}
}

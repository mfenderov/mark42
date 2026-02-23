package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookStop(t *testing.T) {
	t.Run("full mode when files edited", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"),
			[]byte("src/main.go\nsrc/lib.go\n"), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"),
			[]byte(`{"toolName":"Edit","filePath":"/a.go"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, got)
		}

		if output["decision"] != "block" {
			t.Errorf("decision = %v, want block", output["decision"])
		}
		if output["suppressOutput"] != true {
			t.Errorf("suppressOutput = %v, want true", output["suppressOutput"])
		}

		reason, ok := output["reason"].(string)
		if !ok {
			t.Fatal("reason is not a string")
		}
		if !strings.Contains(reason, "memory-updater") {
			t.Errorf("reason should mention memory-updater, got: %s", reason)
		}
		if !strings.Contains(reason, "full") {
			t.Errorf("reason should contain 'full' mode, got: %s", reason)
		}
	})

	t.Run("knowledge-only mode with events but no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Read"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected blocking output for knowledge-only session")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, got)
		}

		if output["decision"] != "block" {
			t.Errorf("decision = %v, want block", output["decision"])
		}

		reason, ok := output["reason"].(string)
		if !ok {
			t.Fatal("reason is not a string")
		}
		if !strings.Contains(reason, "knowledge-only") {
			t.Errorf("reason should contain 'knowledge-only' mode, got: %s", reason)
		}
		if !strings.Contains(reason, "memory-updater") {
			t.Errorf("reason should mention memory-updater, got: %s", reason)
		}
	})

	t.Run("no output when no events and no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(""), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("truly empty session should produce no output, got: %s", buf.String())
		}
	})

	t.Run("flag guard prevents re-entry", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf1, buf2 captureBuffer
		runStopHook(dir, withOutput(&buf1))
		runStopHook(dir, withOutput(&buf2))

		if buf1.String() == "" {
			t.Error("first call should produce output")
		}
		if buf2.String() != "" {
			t.Errorf("second call should be silent (flag guard), got: %s", buf2.String())
		}
	})

	t.Run("clears both buffers after output", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Edit"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if len(strings.TrimSpace(string(dirty))) > 0 {
			t.Error("dirty-files should be cleared by hook")
		}

		events, _ := os.ReadFile(filepath.Join(m42, "session-events"))
		if len(strings.TrimSpace(string(events))) > 0 {
			t.Error("session-events should be cleared")
		}
	})

	t.Run("no output without project dir", func(t *testing.T) {
		var buf captureBuffer
		runStopHook("", withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("expected empty output, got: %s", buf.String())
		}
	})

	t.Run("caps events at 50", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		var sb strings.Builder
		for range 60 {
			sb.WriteString(`{"toolName":"Edit"}` + "\n")
		}
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(sb.String()), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := strings.TrimSpace(buf.String())
		if got == "" {
			t.Fatal("expected blocking output with dirty files")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(got), &output); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if output["decision"] != "block" {
			t.Errorf("decision = %v, want block", output["decision"])
		}

		reason, ok := output["reason"].(string)
		if !ok {
			t.Fatal("reason is not a string")
		}
		if !strings.Contains(reason, "full") {
			t.Errorf("reason should contain 'full' mode, got: %s", reason)
		}
	})
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookSessionEnd(t *testing.T) {
	t.Run("produces no output", func(t *testing.T) {
		dir := setupProjectDir(t)

		var buf captureBuffer
		runSessionEndHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("session-end should produce no output, got: %s", buf.String())
		}
	})

	t.Run("no panic on missing project dir", func(t *testing.T) {
		var buf captureBuffer
		runSessionEndHook("", withOutput(&buf))
	})
}

func TestHookPreCompact(t *testing.T) {
	t.Run("outputs hookSpecificOutput with file count", func(t *testing.T) {
		dir := setupProjectDir(t)
		os.WriteFile(filepath.Join(mark42Dir(dir), "dirty-files"),
			[]byte("a.go\nb.go\nc.go\n"), 0o644)

		var buf captureBuffer
		runPreCompactHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output not valid JSON: %v\ngot: %s", err, got)
		}

		specific, ok := output["hookSpecificOutput"].(map[string]any)
		if !ok {
			t.Fatal("missing hookSpecificOutput")
		}

		if specific["memoriesPreserved"] != float64(3) {
			t.Errorf("memoriesPreserved = %v, want 3", specific["memoriesPreserved"])
		}
	})

	t.Run("zero files produces zero count", func(t *testing.T) {
		dir := setupProjectDir(t)

		var buf captureBuffer
		runPreCompactHook(dir, withOutput(&buf))

		var output map[string]any
		json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &output)
		specific := output["hookSpecificOutput"].(map[string]any)

		if specific["memoriesPreserved"] != float64(0) {
			t.Errorf("memoriesPreserved = %v, want 0", specific["memoriesPreserved"])
		}
	})
}

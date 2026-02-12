package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestHookSessionStart(t *testing.T) {
	t.Run("clears stop flag", func(t *testing.T) {
		dir := t.TempDir()
		m42 := filepath.Join(dir, ".claude", "mark42")
		os.MkdirAll(m42, 0o755)
		flagPath := filepath.Join(m42, "stop-prompted")
		os.WriteFile(flagPath, []byte(""), 0o644)

		runSessionStartHook(dir, nil)

		if _, err := os.Stat(flagPath); !os.IsNotExist(err) {
			t.Error("stop flag should be cleared")
		}
	})

	t.Run("outputs session recall when sessions exist", func(t *testing.T) {
		dir := t.TempDir()
		projectDir := filepath.Join(dir, "testproject")
		os.MkdirAll(projectDir, 0o755)
		dbPath := filepath.Join(dir, "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		session, _ := store.CreateSession("testproject")
		store.CompleteSession(session.Name, "Did some testing work")

		var buf captureBuffer
		runSessionStartHook(projectDir, store, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Error("expected output from session recall")
		}
		if !contains(got, "Recent Sessions") {
			t.Errorf("output missing session recall header, got: %s", got)
		}
	})

	t.Run("outputs context when memories exist", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		store.CreateEntity("Go Conventions", "convention", []string{"Use gofmt"})

		var buf captureBuffer
		runSessionStartHook(dir, store, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Error("expected context output")
		}
		if !contains(got, "Relevant Memories") {
			t.Errorf("output missing context header, got: %s", got)
		}
	})

	t.Run("no output when store is nil", func(t *testing.T) {
		dir := t.TempDir()
		var buf captureBuffer
		runSessionStartHook(dir, nil, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("expected no output, got: %s", buf.String())
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

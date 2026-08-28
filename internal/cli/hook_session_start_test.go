package cli

import (
	"os"
	"path/filepath"
	"strings"
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
		if !strings.Contains(got, "Recent Sessions") {
			t.Errorf("output missing session recall header, got: %s", got)
		}
	})

	t.Run("outputs context when memories exist", func(t *testing.T) {
		tmpBase := t.TempDir()
		// Use a fixed project name so FTS query on projectName finds the entity
		projectDir := filepath.Join(tmpBase, "myproject")
		os.MkdirAll(projectDir, 0o755)
		dbPath := filepath.Join(tmpBase, "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		// Entity content contains the project name so FTS query ("myproject") finds it
		store.CreateEntity("myproject Conventions", "convention", []string{"myproject uses gofmt"})

		var buf captureBuffer
		runSessionStartHook(projectDir, store, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Error("expected context output")
		}
		if !strings.Contains(got, "Relevant Memories") {
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

	t.Run("creates current-session file when store is not nil", func(t *testing.T) {
		dir := t.TempDir()
		projectDir := filepath.Join(dir, "testproject")
		os.MkdirAll(mark42Dir(projectDir), 0o755)
		dbPath := filepath.Join(dir, "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		var buf captureBuffer
		runSessionStartHook(projectDir, store, withOutput(&buf))

		data, err := os.ReadFile(filepath.Join(mark42Dir(projectDir), "current-session"))
		if err != nil {
			t.Fatalf("current-session file not created: %v", err)
		}
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "session-testproject-") {
			t.Errorf("unexpected session name: %q", strings.TrimSpace(string(data)))
		}
	})

	t.Run("no current-session file when store is nil", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(mark42Dir(dir), 0o755)

		var buf captureBuffer
		runSessionStartHook(dir, nil, withOutput(&buf))

		if _, err := os.Stat(filepath.Join(mark42Dir(dir), "current-session")); !os.IsNotExist(err) {
			t.Error("current-session should not be created when store is nil")
		}
	})
}

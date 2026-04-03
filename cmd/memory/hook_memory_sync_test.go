package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestProjectSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard path", "/Users/mark/dev/private/deutsch", "-Users-mark-dev-private-deutsch"},
		{"root slash", "/", "-"},
		{"trailing slash", "/Users/mark/dev/", "-Users-mark-dev-"},
		{"single segment", "/project", "-project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectSlug(tt.input)
			if got != tt.want {
				t.Errorf("projectSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCCMemoryFile(t *testing.T) {
	t.Run("parses valid frontmatter and body", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\nname: Hook debugging with debug mode\ndescription: SessionStart hook errors are opaque in UI/JSONL\ntype: feedback\n---\n\nWhen SessionStart:startup hook error appears, use claude --debug.\n"
		path := filepath.Join(dir, "feedback_hook_debugging.md")
		os.WriteFile(path, []byte(content), 0o644)

		mem, err := parseCCMemoryFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mem.Name != "Hook debugging with debug mode" {
			t.Errorf("Name = %q, want %q", mem.Name, "Hook debugging with debug mode")
		}
		if mem.Description != "SessionStart hook errors are opaque in UI/JSONL" {
			t.Errorf("Description = %q, want %q", mem.Description, "SessionStart hook errors are opaque in UI/JSONL")
		}
		if mem.Type != "feedback" {
			t.Errorf("Type = %q, want %q", mem.Type, "feedback")
		}
		if mem.Body == "" {
			t.Error("Body should not be empty")
		}
		if mem.FileName != "feedback_hook_debugging.md" {
			t.Errorf("FileName = %q, want %q", mem.FileName, "feedback_hook_debugging.md")
		}
	})

	t.Run("returns error for file without frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		content := "No frontmatter here, just plain content.\n"
		path := filepath.Join(dir, "no_frontmatter.md")
		os.WriteFile(path, []byte(content), 0o644)

		_, err := parseCCMemoryFile(path)
		if err == nil {
			t.Error("expected error for file without frontmatter opening ---")
		}
	})

	t.Run("returns error for empty file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.md")
		os.WriteFile(path, []byte(""), 0o644)

		_, err := parseCCMemoryFile(path)
		if err == nil {
			t.Error("expected error for empty file (no frontmatter)")
		}
	})

	t.Run("returns error for unclosed frontmatter delimiter", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\nname: Unclosed\ndescription: missing closing delimiter\ntype: test\n"
		path := filepath.Join(dir, "unclosed.md")
		os.WriteFile(path, []byte(content), 0o644)

		_, err := parseCCMemoryFile(path)
		if err == nil {
			t.Error("expected error for unclosed frontmatter delimiter")
		}
	})

	t.Run("handles missing body after closing delimiter", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\nname: Empty body test\ndescription: no body\ntype: test\n---\n"
		path := filepath.Join(dir, "empty_body.md")
		os.WriteFile(path, []byte(content), 0o644)

		mem, err := parseCCMemoryFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mem.Body != "" {
			t.Errorf("Body should be empty string, got %q", mem.Body)
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := parseCCMemoryFile("/nonexistent/path/file.md")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestChecksums(t *testing.T) {
	t.Run("round-trips checksums to JSON file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "checksums.json")

		original := map[string]string{
			"feedback_hook.md":  "abc123",
			"project_phase6.md": "def456",
		}
		saveChecksums(path, original)

		loaded := loadChecksums(path)
		if len(loaded) != 2 {
			t.Fatalf("loaded %d checksums, want 2", len(loaded))
		}
		if loaded["feedback_hook.md"] != "abc123" {
			t.Errorf("feedback_hook.md = %q, want %q", loaded["feedback_hook.md"], "abc123")
		}
		if loaded["project_phase6.md"] != "def456" {
			t.Errorf("project_phase6.md = %q, want %q", loaded["project_phase6.md"], "def456")
		}
	})

	t.Run("returns empty map for nonexistent file", func(t *testing.T) {
		loaded := loadChecksums("/nonexistent/checksums.json")
		if len(loaded) != 0 {
			t.Errorf("expected empty map, got %d entries", len(loaded))
		}
	})

	t.Run("returns empty map for corrupt JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		os.WriteFile(path, []byte("not json"), 0o644)

		loaded := loadChecksums(path)
		if len(loaded) != 0 {
			t.Errorf("expected empty map for corrupt file, got %d entries", len(loaded))
		}
	})
}

func TestFileChecksum(t *testing.T) {
	t.Run("returns consistent SHA256 for same content", func(t *testing.T) {
		data := []byte("hello world")
		sum1 := checksumBytes(data)
		sum2 := checksumBytes(data)
		if sum1 != sum2 {
			t.Errorf("checksums differ for same data: %q vs %q", sum1, sum2)
		}
		if sum1 == "" {
			t.Error("checksum should not be empty")
		}
		if len(sum1) != 64 {
			t.Errorf("expected 64-char hex, got len=%d: %q", len(sum1), sum1)
		}
	})

	t.Run("returns different checksum for different content", func(t *testing.T) {
		if checksumBytes([]byte("hello")) == checksumBytes([]byte("world")) {
			t.Error("different content should produce different checksums")
		}
	})

	t.Run("returns lowercase hex", func(t *testing.T) {
		sum := checksumBytes([]byte("test"))
		if sum != strings.ToLower(sum) {
			t.Errorf("expected lowercase hex, got %q", sum)
		}
	})
}

func TestCCMemoryDir(t *testing.T) {
	t.Run("derives path from project dir", func(t *testing.T) {
		got := ccMemoryDir("/Users/mark/dev/private/deutsch")
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".claude", "projects", "-Users-mark-dev-private-deutsch", "memory")
		if got != want {
			t.Errorf("ccMemoryDir = %q, want %q", got, want)
		}
	})
}

func newTestStore(t *testing.T, dir string) *storage.Store {
	t.Helper()
	dbPath := filepath.Join(dir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSyncCCMemory(t *testing.T) {
	t.Run("syncs memory files into store", func(t *testing.T) {
		dir := t.TempDir()

		memDir := filepath.Join(dir, "cc-memory")
		os.MkdirAll(memDir, 0o755)
		content := "---\nname: Hook debugging\ndescription: Use debug mode for hook errors\ntype: feedback\n---\n\nWhen hook errors appear, use claude --debug.\n"
		os.WriteFile(filepath.Join(memDir, "feedback_hook.md"), []byte(content), 0o644)

		os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Index\n"), 0o644)

		store := newTestStore(t, dir)

		checksumPath := filepath.Join(dir, "checksums.json")
		syncCCMemory("test-project", memDir, store, checksumPath)

		entity, err := store.GetEntity("cc-memory/test-project/Hook debugging")
		if err != nil {
			t.Fatalf("entity not found: %v", err)
		}
		if entity.Type != "feedback" {
			t.Errorf("entity type = %q, want %q", entity.Type, "feedback")
		}

		found := false
		for _, obs := range entity.Observations {
			if strings.Contains(obs, "Use debug mode") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected description observation, got: %v", entity.Observations)
		}

		bodyFound := false
		for _, obs := range entity.Observations {
			if strings.Contains(obs, "use claude --debug") {
				bodyFound = true
			}
		}
		if !bodyFound {
			t.Errorf("expected body observation, got: %v", entity.Observations)
		}

		checksums := loadChecksums(checksumPath)
		if checksums["feedback_hook.md"] == "" {
			t.Error("checksum not saved for feedback_hook.md")
		}

		// Verify belongs_to relation was created
		relations, err := store.ListRelations("cc-memory/test-project/Hook debugging")
		if err != nil {
			t.Fatalf("failed to list relations: %v", err)
		}
		foundRelation := false
		for _, rel := range relations {
			if rel.From == "cc-memory/test-project/Hook debugging" && rel.To == "project:test-project" && rel.Type == "belongs_to" {
				foundRelation = true
			}
		}
		if !foundRelation {
			t.Error("expected belongs_to relation to project:test-project")
		}
	})

	t.Run("skips unchanged files on second sync", func(t *testing.T) {
		dir := t.TempDir()

		memDir := filepath.Join(dir, "cc-memory")
		os.MkdirAll(memDir, 0o755)
		content := "---\nname: Test mem\ndescription: A test\ntype: user\n---\n\nBody.\n"
		os.WriteFile(filepath.Join(memDir, "test.md"), []byte(content), 0o644)

		store := newTestStore(t, dir)
		checksumPath := filepath.Join(dir, "checksums.json")

		syncCCMemory("test-project", memDir, store, checksumPath)

		_, err := store.GetEntity("cc-memory/test-project/Test mem")
		if err != nil {
			t.Fatalf("entity not found after first sync: %v", err)
		}

		syncCCMemory("test-project", memDir, store, checksumPath)

		entity, err := store.GetEntity("cc-memory/test-project/Test mem")
		if err != nil {
			t.Fatalf("entity not found after second sync: %v", err)
		}
		if entity.Version != 1 {
			t.Errorf("version = %d, want 1 (no new version on unchanged file)", entity.Version)
		}
	})

	t.Run("creates new version when content changes", func(t *testing.T) {
		dir := t.TempDir()

		memDir := filepath.Join(dir, "cc-memory")
		os.MkdirAll(memDir, 0o755)
		path := filepath.Join(memDir, "evolving.md")
		os.WriteFile(path, []byte("---\nname: Evolving\ndescription: V1\ntype: project\n---\n\nOld body.\n"), 0o644)

		store := newTestStore(t, dir)
		checksumPath := filepath.Join(dir, "checksums.json")
		syncCCMemory("test-project", memDir, store, checksumPath)

		os.WriteFile(path, []byte("---\nname: Evolving\ndescription: V2 updated\ntype: project\n---\n\nNew body.\n"), 0o644)
		syncCCMemory("test-project", memDir, store, checksumPath)

		entity, err := store.GetEntity("cc-memory/test-project/Evolving")
		if err != nil {
			t.Fatalf("entity not found: %v", err)
		}
		if entity.Version != 2 {
			t.Errorf("version = %d, want 2", entity.Version)
		}
	})

	t.Run("skips MEMORY.md", func(t *testing.T) {
		dir := t.TempDir()

		memDir := filepath.Join(dir, "cc-memory")
		os.MkdirAll(memDir, 0o755)
		os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Index\n- entry\n"), 0o644)

		store := newTestStore(t, dir)
		checksumPath := filepath.Join(dir, "checksums.json")
		syncCCMemory("test-project", memDir, store, checksumPath)

		entities, _ := store.ListEntities("")
		for _, e := range entities {
			if strings.Contains(e.Name, "Index") || strings.Contains(e.Name, "MEMORY") {
				t.Errorf("MEMORY.md should be skipped, but found entity: %s", e.Name)
			}
		}
	})

	t.Run("handles missing memory directory gracefully", func(t *testing.T) {
		dir := t.TempDir()
		store := newTestStore(t, dir)
		syncCCMemory("test-project", "/nonexistent/memory", store, filepath.Join(dir, "checksums.json"))
	})
}

func TestStopHookTriggersCCMemorySync(t *testing.T) {
	t.Run("syncs CC memory files during stop hook", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Point dbPath at a temp DB so the test never touches ~/.claude/memory.db
		origDB := dbPath
		dbPath = filepath.Join(tmpHome, "test.db")
		t.Cleanup(func() { dbPath = origDB })

		dir := t.TempDir()
		m42 := mark42Dir(dir)
		os.MkdirAll(m42, 0o755)

		slug := projectSlug(dir)
		memDir := filepath.Join(tmpHome, ".claude", "projects", slug, "memory")
		os.MkdirAll(memDir, 0o755)

		content := "---\nname: Integration test memory\ndescription: Created during stop hook test\ntype: project\n---\n\nThis was synced via stop hook.\n"
		os.WriteFile(filepath.Join(memDir, "integration_test.md"), []byte(content), 0o644)

		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Edit"}`+"\n"), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		checksumPath := filepath.Join(m42, "memory-checksums.json")
		if _, err := os.Stat(checksumPath); os.IsNotExist(err) {
			t.Error("memory-checksums.json should exist after stop hook")
		}

		verifyStore, err := storage.NewStore(filepath.Join(tmpHome, "test.db"))
		if err != nil {
			t.Fatalf("failed to open test store for verification: %v", err)
		}
		defer verifyStore.Close()
		if err := verifyStore.Migrate(); err != nil {
			t.Fatalf("migrate failed: %v", err)
		}
		entityName := "cc-memory/" + slug + "/Integration test memory"
		entity, err := verifyStore.GetEntity(entityName)
		if err != nil {
			t.Fatalf("entity not found in DB after stop hook: %v", err)
		}
		if entity.Type != "project" {
			t.Errorf("entity type = %q, want %q", entity.Type, "project")
		}
	})
}

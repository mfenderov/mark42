package main

import (
	"os"
	"path/filepath"
	"testing"
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
		dir := t.TempDir()
		path := filepath.Join(dir, "test.md")
		os.WriteFile(path, []byte("hello world"), 0o644)

		sum1 := fileChecksum(path)
		sum2 := fileChecksum(path)
		if sum1 != sum2 {
			t.Errorf("checksums differ for same file: %q vs %q", sum1, sum2)
		}
		if sum1 == "" {
			t.Error("checksum should not be empty")
		}
		if len(sum1) != 64 {
			t.Errorf("expected 64-char hex, got len=%d: %q", len(sum1), sum1)
		}
		for _, c := range sum1 {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("checksum contains non-lowercase-hex character %q in %q", c, sum1)
				break
			}
		}
	})

	t.Run("returns different checksum for different content", func(t *testing.T) {
		dir := t.TempDir()
		p1 := filepath.Join(dir, "a.md")
		p2 := filepath.Join(dir, "b.md")
		os.WriteFile(p1, []byte("hello"), 0o644)
		os.WriteFile(p2, []byte("world"), 0o644)

		if fileChecksum(p1) == fileChecksum(p2) {
			t.Error("different content should produce different checksums")
		}
	})

	t.Run("returns empty string for missing file", func(t *testing.T) {
		if got := fileChecksum("/nonexistent"); got != "" {
			t.Errorf("expected empty string, got %q", got)
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

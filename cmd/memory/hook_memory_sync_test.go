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

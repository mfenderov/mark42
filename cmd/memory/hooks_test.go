package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetProjectDir(t *testing.T) {
	t.Run("returns CLAUDE_PROJECT_DIR when set", func(t *testing.T) {
		t.Setenv("CLAUDE_PROJECT_DIR", "/tmp/myproject")
		got := getProjectDir()
		if got != "/tmp/myproject" {
			t.Errorf("got %q, want /tmp/myproject", got)
		}
	})

	t.Run("returns empty when unset", func(t *testing.T) {
		t.Setenv("CLAUDE_PROJECT_DIR", "")
		got := getProjectDir()
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestMark42Dir(t *testing.T) {
	got := mark42Dir("/tmp/myproject")
	want := "/tmp/myproject/.claude/mark42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadLines(t *testing.T) {
	t.Run("reads file lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

		got := readLines(path)
		if len(got) != 3 {
			t.Fatalf("got %d lines, want 3", len(got))
		}
		if got[0] != "line1" || got[1] != "line2" || got[2] != "line3" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("returns empty for missing file", func(t *testing.T) {
		got := readLines("/nonexistent/file")
		if len(got) != 0 {
			t.Errorf("got %d lines, want 0", len(got))
		}
	})

	t.Run("skips blank lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		os.WriteFile(path, []byte("line1\n\n\nline2\n"), 0o644)

		got := readLines(path)
		if len(got) != 2 {
			t.Fatalf("got %d lines, want 2", len(got))
		}
	})
}

func TestTouchFlag(t *testing.T) {
	t.Run("creates flag file and returns true", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "subfolder", "flag")

		ok := touchFlag(path)
		if !ok {
			t.Error("expected true on first touch")
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("flag file not created")
		}
	})

	t.Run("returns false if flag already exists", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "flag")
		os.WriteFile(path, []byte(""), 0o644)

		ok := touchFlag(path)
		if ok {
			t.Error("expected false when flag already exists")
		}
	})
}

func TestClearFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flag")
	os.WriteFile(path, []byte(""), 0o644)

	clearFlag(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("flag file should be removed")
	}
}

func TestClearFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	os.WriteFile(path, []byte("content"), 0o644)

	clearFile(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("file should be empty, got %q", data)
	}
}

func TestReadJSONLines(t *testing.T) {
	type event struct {
		ToolName string `json:"toolName"`
		FilePath string `json:"filePath,omitempty"`
	}

	t.Run("reads valid JSONL", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "events.jsonl")
		content := `{"toolName":"Edit","filePath":"/a.go"}
{"toolName":"Write","filePath":"/b.go"}
`
		os.WriteFile(path, []byte(content), 0o644)

		got := readJSONLines[event](path)
		if len(got) != 2 {
			t.Fatalf("got %d events, want 2", len(got))
		}
		if got[0].ToolName != "Edit" || got[1].ToolName != "Write" {
			t.Errorf("unexpected events: %+v", got)
		}
	})

	t.Run("skips bad lines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "events.jsonl")
		content := `{"toolName":"Edit"}
bad json
{"toolName":"Write"}
`
		os.WriteFile(path, []byte(content), 0o644)

		got := readJSONLines[event](path)
		if len(got) != 2 {
			t.Fatalf("got %d events, want 2 (skip bad)", len(got))
		}
	})

	t.Run("returns empty for missing file", func(t *testing.T) {
		got := readJSONLines[event]("/nonexistent")
		if len(got) != 0 {
			t.Errorf("got %d, want 0", len(got))
		}
	})
}

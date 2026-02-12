package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldTrack(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		projectDir string
		want       bool
	}{
		{"normal file", "/proj/src/main.go", "/proj", true},
		{".claude dir", "/proj/.claude/config.json", "/proj", false},
		{"CLAUDE.md at root", "/proj/CLAUDE.md", "/proj", false},
		{"CLAUDE.md nested", "/proj/sub/CLAUDE.md", "/proj", false},
		{"outside project", "/other/file.go", "/proj", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrack(tt.filePath, tt.projectDir)
			if got != tt.want {
				t.Errorf("shouldTrack(%q, %q) = %v, want %v", tt.filePath, tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestShellTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple", `rm file.go`, []string{"rm", "file.go"}},
		{"double quotes", `rm "my file.go"`, []string{"rm", "my file.go"}},
		{"single quotes", `rm 'my file.go'`, []string{"rm", "my file.go"}},
		{"mixed", `mv "a b" dest`, []string{"mv", "a b", "dest"}},
		{"empty", ``, nil},
		{"flags", `rm -rf dir`, []string{"rm", "-rf", "dir"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellTokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("shellTokenize(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsShellSyntax(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"&&", true},
		{"||", true},
		{";", true},
		{"|", true},
		{">", true},
		{">>", true},
		{"2>/dev/null", true},
		{"2>&1", true},
		{"1>>log", true},
		{"<input", true},
		{"file.go", false},
		{"-rf", false},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			got := isShellSyntax(tt.token)
			if got != tt.want {
				t.Errorf("isShellSyntax(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestExtractFilesFromBash(t *testing.T) {
	proj := "/proj"
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{"rm single", "rm file.go", []string{"/proj/file.go"}},
		{"rm with flags", "rm -rf dir", []string{"/proj/dir"}},
		{"rm absolute", "rm /tmp/file.go", []string{"/tmp/file.go"}},
		{"rm with redirect", "rm file.go 2>/dev/null", []string{"/proj/file.go"}},
		{"git rm", "git rm old.go", []string{"/proj/old.go"}},
		{"mv", "mv old.go new.go", []string{"/proj/old.go"}},
		{"git mv", "git mv old.go new.go", []string{"/proj/old.go"}},
		{"unlink", "unlink temp.txt", []string{"/proj/temp.txt"}},
		{"skip ls", "ls -la", nil},
		{"skip cat", "cat file.go", nil},
		{"skip git status", "git status", nil},
		{"skip go build", "go build ./...", nil},
		{"skip make", "make test", nil},
		{"empty", "", nil},
		{"rm stops at pipe", "rm foo.go | echo done", []string{"/proj/foo.go"}},
		{"rm stops at &&", "rm foo.go && echo done", []string{"/proj/foo.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilesFromBash(tt.command, proj)
			if len(got) != len(tt.want) {
				t.Errorf("extractFilesFromBash(%q) = %v, want %v", tt.command, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPostToolUseHook(t *testing.T) {
	t.Run("tracks Edit file", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "src", "main.go")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 1 {
			t.Fatalf("got %d dirty files, want 1", len(dirty))
		}
		if !strings.Contains(dirty[0], "main.go") {
			t.Errorf("dirty file should contain main.go, got %q", dirty[0])
		}

		events := readLines(filepath.Join(mark42Dir(dir), "session-events"))
		if len(events) != 1 {
			t.Fatalf("got %d events, want 1", len(events))
		}
	})

	t.Run("tracks Write file", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Write",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "new.go")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 1 {
			t.Fatalf("got %d dirty files, want 1", len(dirty))
		}
	})

	t.Run("excludes .claude dir files", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, ".claude", "config.json")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 0 {
			t.Errorf("should not track .claude files, got %d", len(dirty))
		}
	})

	t.Run("excludes CLAUDE.md", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "CLAUDE.md")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 0 {
			t.Errorf("should not track CLAUDE.md, got %d", len(dirty))
		}
	})

	t.Run("deduplicates dirty files", func(t *testing.T) {
		dir := setupProjectDir(t)
		filePath := filepath.Join(dir, "src", "main.go")

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filePath},
		}
		runPostToolUseHook(dir, input)
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 1 {
			t.Errorf("should deduplicate, got %d dirty files", len(dirty))
		}
	})

	t.Run("writes session events as JSONL", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "a.go")},
		}
		runPostToolUseHook(dir, input)

		eventPath := filepath.Join(mark42Dir(dir), "session-events")
		data, _ := os.ReadFile(eventPath)
		var evt map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &evt); err != nil {
			t.Fatalf("event not valid JSON: %v", err)
		}
		if evt["toolName"] != "Edit" {
			t.Errorf("event toolName = %v, want Edit", evt["toolName"])
		}
	})

	t.Run("gitmode skips non-commit", func(t *testing.T) {
		dir := setupProjectDir(t)
		configDir := mark42Dir(dir)
		os.WriteFile(filepath.Join(configDir, "config.json"),
			[]byte(`{"triggerMode":"gitmode"}`), 0o644)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "a.go")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(configDir, "dirty-files"))
		if len(dirty) != 0 {
			t.Errorf("gitmode should skip non-commit, got %d", len(dirty))
		}
	})

	t.Run("Bash rm extracts file", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Bash",
			ToolInput: map[string]any{"command": "rm " + filepath.Join(dir, "old.go")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 1 {
			t.Fatalf("got %d dirty files, want 1", len(dirty))
		}
	})
}

func setupProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(mark42Dir(dir), 0o755)
	return dir
}

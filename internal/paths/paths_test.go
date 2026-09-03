package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde only", "~", home},
		{"tilde slash", "~/foo", filepath.Join(home, "foo")},
		{"dollar home", "${HOME}/foo", filepath.Join(home, "foo")},
		{"bare dollar home", "$HOME/foo", filepath.Join(home, "foo")},
		{"absolute unchanged", "/abs/path", "/abs/path"},
		{"relative unchanged", "rel/path", "rel/path"},
		{"empty unchanged", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolvePath(tt.in); got != tt.want {
				t.Errorf("ResolvePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDBPath(t *testing.T) {
	tests := []struct {
		name          string
		mark42Env     string
		claudeEnv     string
		neutralExists bool
		legacyExists  bool
	}{
		{"MARK42_DB wins", "/x/mark42.db", "/x/claude.db", false, false},
		{"CLAUDE_MEMORY_DB legacy alias", "", "/x/claude.db", false, false},
		{"MARK42_DB expands tilde", "~/mark42.db", "", false, false},
		{"neutral exists preferred", "", "", true, true},
		{"legacy exists when neutral absent", "", "", false, true},
		{"default neutral when neither exists", "", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("MARK42_DB", tt.mark42Env)
			t.Setenv("CLAUDE_MEMORY_DB", tt.claudeEnv)

			neutral := filepath.Join(home, ".mark42", "memory.db")
			legacy := filepath.Join(home, ".claude", "memory.db")

			if tt.neutralExists {
				os.MkdirAll(filepath.Dir(neutral), 0o755)
				os.WriteFile(neutral, []byte(""), 0o644)
			}
			if tt.legacyExists {
				os.MkdirAll(filepath.Dir(legacy), 0o755)
				os.WriteFile(legacy, []byte(""), 0o644)
			}

			var want string
			switch {
			case tt.mark42Env != "":
				want = ResolvePath(tt.mark42Env)
			case tt.claudeEnv != "":
				want = ResolvePath(tt.claudeEnv)
			case tt.neutralExists:
				want = neutral
			case tt.legacyExists:
				want = legacy
			default:
				want = neutral
			}

			if got := ResolveDBPath(home); got != want {
				t.Errorf("ResolveDBPath() = %q, want %q", got, want)
			}
		})
	}
}

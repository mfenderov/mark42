package cli

import (
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

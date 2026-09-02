package state

import (
	"path/filepath"
	"testing"
)

func TestStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := Dir("/tmp/myproject")
	want := filepath.Join(home, ".mark42", "state", ProjectSlug("/tmp/myproject"))
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

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
			got := ProjectSlug(tt.input)
			if got != tt.want {
				t.Errorf("ProjectSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

package main

import (
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
			if got := resolvePath(tt.in); got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

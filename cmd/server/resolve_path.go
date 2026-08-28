package main

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvePath expands a leading ~ and ${VAR}/$VAR references in p.
// .mcp.json passes CLAUDE_MEMORY_DB as a literal "${HOME}/..." string,
// so without this the server creates a literal "${HOME}" directory.
func resolvePath(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	return os.ExpandEnv(p)
}

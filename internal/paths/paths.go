package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath expands a leading ~ and ${VAR}/$VAR references in p.
// .mcp.json passes the DB path as a literal "${HOME}/..." string,
// so without this the server creates a literal "${HOME}" directory.
func ResolvePath(p string) string {
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

// ResolveDBPath resolves the database path using the neutral-first order:
// MARK42_DB, then CLAUDE_MEMORY_DB (legacy), then an existing neutral or
// legacy file, falling back to the neutral default.
func ResolveDBPath(home string) string {
	if p := os.Getenv("MARK42_DB"); p != "" {
		return ResolvePath(p)
	}
	if p := os.Getenv("CLAUDE_MEMORY_DB"); p != "" {
		return ResolvePath(p)
	}

	neutral := filepath.Join(home, ".mark42", "memory.db")
	if fileExists(neutral) {
		return neutral
	}

	legacy := filepath.Join(home, ".claude", "memory.db")
	if fileExists(legacy) {
		return legacy
	}

	return neutral
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

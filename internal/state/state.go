package state

import (
	"os"
	"path/filepath"
	"strings"
)

func ProjectSlug(projectDir string) string {
	return strings.ReplaceAll(projectDir, "/", "-")
}

func Dir(projectDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Legacy fallback when the home directory is unavailable.
		return filepath.Join(projectDir, ".claude", "mark42")
	}
	return filepath.Join(home, ".mark42", "state", ProjectSlug(projectDir))
}

func CurrentSessionPath(projectDir string) string {
	return filepath.Join(Dir(projectDir), "current-session")
}

func WriteCurrentSession(projectDir, sessionName string) {
	_ = os.MkdirAll(Dir(projectDir), 0o755)
	_ = os.WriteFile(CurrentSessionPath(projectDir), []byte(sessionName), 0o644)
}

// LegacyDir returns the project-local legacy scratch path (e.g. projectDir/.claude/mark42).
func LegacyDir(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "mark42")
}

// MigrateLegacyState promotes legacy project-local state files into the neutral global state dir.
// Implements the Strangler Fig pattern for seamless backward-compatible upgrade.
func MigrateLegacyState(projectDir string) error {
	legacy := LegacyDir(projectDir)
	entries, err := os.ReadDir(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	targetDir := Dir(projectDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(targetDir, entry.Name())

		// Only copy if destination does not already exist
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			data, err := os.ReadFile(src)
			if err == nil {
				_ = os.WriteFile(dst, data, 0o644)
			}
		}
	}
	return nil
}

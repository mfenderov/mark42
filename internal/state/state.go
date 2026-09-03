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

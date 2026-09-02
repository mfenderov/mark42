package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/state"
)

// mark42Dir returns the project-local mark42 scratch directory.
func mark42Dir(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "mark42")
}

// legacyCurrentSessionPath is the pre-neutral current-session location.
func legacyCurrentSessionPath(projectDir string) string {
	return filepath.Join(mark42Dir(projectDir), "current-session")
}

// readCurrentSession reads the active session name, preferring the neutral
// location and falling back to the legacy location.
func readCurrentSession(projectDir string) string {
	if data, err := os.ReadFile(state.CurrentSessionPath(projectDir)); err == nil {
		return strings.TrimSpace(string(data))
	}
	data, err := os.ReadFile(legacyCurrentSessionPath(projectDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func readJSONLines[T any](path string) []T {
	lines := readLines(path)
	var results []T
	for _, line := range lines {
		var v T
		if err := json.Unmarshal([]byte(line), &v); err == nil {
			results = append(results, v)
		}
	}
	return results
}

func clearFile(path string) {
	_ = os.WriteFile(path, []byte(""), 0o644)
}

// touchFlag creates a flag file. Returns true if created, false if it already exists.
func touchFlag(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return false
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(""), 0o644)
	return true
}

func clearFlag(path string) {
	_ = os.Remove(path)
}

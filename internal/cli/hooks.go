package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Claude Code hook handlers",
	Long:  "Subcommands for each Claude Code lifecycle hook event.",
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

func getProjectDir() string {
	return os.Getenv("CLAUDE_PROJECT_DIR")
}

func mark42Dir(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "mark42")
}

func stateDir(projectDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return mark42Dir(projectDir)
	}
	return filepath.Join(home, ".mark42", "state", projectSlug(projectDir))
}

func currentSessionPath(projectDir string) string {
	return filepath.Join(stateDir(projectDir), "current-session")
}

func legacyCurrentSessionPath(projectDir string) string {
	return filepath.Join(mark42Dir(projectDir), "current-session")
}

func readStdinJSON(v any) error {
	return json.NewDecoder(os.Stdin).Decode(v)
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

func readCurrentSession(projectDir string) string {
	if data, err := os.ReadFile(currentSessionPath(projectDir)); err == nil {
		return strings.TrimSpace(string(data))
	}
	data, err := os.ReadFile(legacyCurrentSessionPath(projectDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCurrentSession(projectDir, sessionName string) {
	_ = os.MkdirAll(stateDir(projectDir), 0o755)
	_ = os.WriteFile(currentSessionPath(projectDir), []byte(sessionName), 0o644)
}

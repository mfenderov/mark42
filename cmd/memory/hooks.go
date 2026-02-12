package main

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

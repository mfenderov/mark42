package state

import (
	"fmt"
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
		return fmt.Errorf("reading legacy state dir: %w", err)
	}

	targetDir := Dir(projectDir)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating target state dir: %w", err)
	}

	for _, entry := range entries {
		// Skip non-regular files (directories, symlinks, sockets, pipes) to prevent
		// arbitrary file reads or local file disclosure via symlinks in legacy dir.
		if !entry.Type().IsRegular() {
			continue
		}
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(targetDir, entry.Name())
		if err := migrateStateFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func migrateStateFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat legacy state file %s: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return nil // skip non-regular files or symlinks
	}

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading legacy state file %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("writing migrated state file %s: %w", dst, err)
		}
	} else if err != nil {
		return fmt.Errorf("stat destination state file %s: %w", dst, err)
	}
	return nil
}

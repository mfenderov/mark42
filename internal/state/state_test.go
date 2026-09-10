package state

import (
	"os"
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

func TestMigrateLegacyState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := t.TempDir()
	legacyPath := filepath.Join(projDir, ".claude", "mark42")
	if err := os.MkdirAll(legacyPath, 0o755); err != nil {
		t.Fatalf("failed to create legacy dir: %v", err)
	}

	legacySessionFile := filepath.Join(legacyPath, "current-session")
	if err := os.WriteFile(legacySessionFile, []byte("session-legacy-123"), 0o644); err != nil {
		t.Fatalf("failed to write legacy file: %v", err)
	}

	// Run migration
	if err := MigrateLegacyState(projDir); err != nil {
		t.Fatalf("MigrateLegacyState failed: %v", err)
	}

	// Verify neutral path now has current-session
	neutralFile := CurrentSessionPath(projDir)
	data, err := os.ReadFile(neutralFile)
	if err != nil {
		t.Fatalf("failed to read migrated neutral file: %v", err)
	}
	if string(data) != "session-legacy-123" {
		t.Errorf("expected migrated content %q, got %q", "session-legacy-123", string(data))
	}
}

func TestMigrateLegacyState_ErrorOnUnreadableFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := t.TempDir()
	legacyPath := filepath.Join(projDir, ".claude", "mark42")
	if err := os.MkdirAll(legacyPath, 0o755); err != nil {
		t.Fatalf("failed to create legacy dir: %v", err)
	}

	unreadableFile := filepath.Join(legacyPath, "unreadable.txt")
	if err := os.WriteFile(unreadableFile, []byte("data"), 0o000); err != nil {
		t.Fatalf("failed to write unreadable file: %v", err)
	}
	defer os.Chmod(unreadableFile, 0o644) // cleanup permission for tempdir removal

	// On unix systems where running as non-root, reading mode 0000 fails
	if os.Geteuid() != 0 {
		err := MigrateLegacyState(projDir)
		if err == nil {
			t.Errorf("expected error reading unreadable file, got nil")
		}
	}
}

func TestMigrateLegacyState_SkipsSymlinksAndNonRegularFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projDir := t.TempDir()
	legacyPath := filepath.Join(projDir, ".claude", "mark42")
	if err := os.MkdirAll(legacyPath, 0o755); err != nil {
		t.Fatalf("failed to create legacy dir: %v", err)
	}

	// 1. Regular state file
	regularFile := filepath.Join(legacyPath, "current-session")
	if err := os.WriteFile(regularFile, []byte("session-valid-123"), 0o644); err != nil {
		t.Fatalf("failed to write regular file: %v", err)
	}

	// 2. Sensitive external file
	sensitiveDir := t.TempDir()
	sensitiveFile := filepath.Join(sensitiveDir, "secret.key")
	if err := os.WriteFile(sensitiveFile, []byte("super-secret-content"), 0o600); err != nil {
		t.Fatalf("failed to write sensitive file: %v", err)
	}

	// 3. Symlink inside legacy dir pointing to the sensitive external file
	symlinkPath := filepath.Join(legacyPath, "symlink-to-secret")
	if err := os.Symlink(sensitiveFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// 4. Subdirectory (non-regular file)
	subDir := filepath.Join(legacyPath, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Run migration
	if err := MigrateLegacyState(projDir); err != nil {
		t.Fatalf("MigrateLegacyState failed: %v", err)
	}

	// Verify regular file WAS migrated
	migratedRegular := CurrentSessionPath(projDir)
	data, err := os.ReadFile(migratedRegular)
	if err != nil {
		t.Fatalf("expected regular file to be migrated: %v", err)
	}
	if string(data) != "session-valid-123" {
		t.Errorf("expected content %q, got %q", "session-valid-123", string(data))
	}

	// Verify symlink was NOT migrated
	targetDir := Dir(projDir)
	migratedSymlink := filepath.Join(targetDir, "symlink-to-secret")
	if _, err := os.Lstat(migratedSymlink); !os.IsNotExist(err) {
		t.Errorf("symlink was migrated into target state dir, expected it to be skipped")
	}

	// Verify subdir was NOT migrated as a file
	migratedSubdir := filepath.Join(targetDir, "subdir")
	if _, err := os.Lstat(migratedSubdir); !os.IsNotExist(err) {
		t.Errorf("subdir was migrated into target state dir, expected it to be skipped")
	}
}

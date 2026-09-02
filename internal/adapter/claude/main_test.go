package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

// TestMain pins StoreFactory to a temp DB so no test can resolve the default
// store path (~/.claude/memory.db or ~/.mark42/memory.db) and pollute it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mark42-claude-test-*")
	if err != nil {
		os.Exit(1)
	}
	orig := StoreFactory
	StoreFactory = func() (*storage.Store, error) {
		return storage.NewStore(filepath.Join(dir, "test.db"))
	}
	code := m.Run()
	StoreFactory = orig
	os.RemoveAll(dir)
	os.Exit(code)
}

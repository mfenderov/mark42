package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/cli"
	"github.com/mfenderov/mark42/internal/storage"
)

func setupTestCLI(t *testing.T) (*bytes.Buffer, *storage.Store) {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "cli_test.db")
	store, err := storage.NewStore(dbFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cli.SetStoreFactory(func() (*storage.Store, error) {
		return storage.NewStore(dbFile)
	})

	var buf bytes.Buffer
	cli.SetOutput(&buf)

	t.Cleanup(func() {
		cli.SetStoreFactory(nil)
		cli.SetOutput(nil)
		store.Close()
	})

	return &buf, store
}

func TestCLI_EntityLifecycle(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	// 1. Create entity
	buf.Reset()
	_, err := executeCommand(root, "entity", "create", "mark42", "project", "--obs", "local memory")
	if err != nil {
		t.Fatalf("entity create failed: %v", err)
	}

	// 2. Get entity
	buf.Reset()
	_, err = executeCommand(root, "entity", "get", "mark42")
	if err != nil {
		t.Fatalf("entity get failed: %v", err)
	}
	if !strings.Contains(buf.String(), "mark42") || !strings.Contains(buf.String(), "local memory") {
		t.Errorf("expected entity get output with observations, got: %s", buf.String())
	}

	// 3. List entities
	buf.Reset()
	_, err = executeCommand(root, "entity", "list")
	if err != nil {
		t.Fatalf("entity list failed: %v", err)
	}
	if !strings.Contains(buf.String(), "mark42") {
		t.Errorf("expected entity list to contain mark42, got: %s", buf.String())
	}

	// 4. Delete entity
	buf.Reset()
	_, err = executeCommand(root, "entity", "delete", "mark42")
	if err != nil {
		t.Fatalf("entity delete failed: %v", err)
	}

	// Verify entity is gone
	_, err = store.GetEntity("mark42")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after deletion, got: %v", err)
	}
}

func TestCLI_ObservationLifecycle(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	if _, err := store.CreateEntity("GoLang", "language", nil); err != nil {
		t.Fatalf("CreateEntity failed: %v", err)
	}

	// 1. Add observation
	buf.Reset()
	_, err := executeCommand(root, "obs", "add", "GoLang", "fast compilation")
	if err != nil {
		t.Fatalf("obs add failed: %v", err)
	}

	e, err := store.GetEntity("GoLang")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(e.Observations) != 1 || e.Observations[0] != "fast compilation" {
		t.Errorf("expected observation to be added, got: %v", e.Observations)
	}

	// 2. Delete observation
	buf.Reset()
	_, err = executeCommand(root, "obs", "delete", "GoLang", "fast compilation")
	if err != nil {
		t.Fatalf("obs delete failed: %v", err)
	}

	e, err = store.GetEntity("GoLang")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(e.Observations) != 0 {
		t.Errorf("expected observation to be deleted, got: %v", e.Observations)
	}
}

func TestCLI_RelationLifecycle(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	store.CreateEntity("NodeA", "test", nil)
	store.CreateEntity("NodeB", "test", nil)

	// 1. Create relation
	buf.Reset()
	_, err := executeCommand(root, "rel", "create", "NodeA", "NodeB", "connects")
	if err != nil {
		t.Fatalf("rel create failed: %v", err)
	}

	// 2. List relations
	buf.Reset()
	_, err = executeCommand(root, "rel", "list", "NodeA")
	if err != nil {
		t.Fatalf("rel list failed: %v", err)
	}
	if !strings.Contains(buf.String(), "NodeB") {
		t.Errorf("expected rel list to show NodeB, got: %s", buf.String())
	}

	// 3. Delete relation
	buf.Reset()
	_, err = executeCommand(root, "rel", "delete", "NodeA", "NodeB", "connects")
	if err != nil {
		t.Fatalf("rel delete failed: %v", err)
	}

	rels, err := store.ListRelations("NodeA")
	if err != nil {
		t.Fatalf("ListRelations failed: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("expected 0 relations after delete, got %d", len(rels))
	}
}

func TestCLI_SearchAndStats(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	store.CreateEntity("SearchTarget", "concept", []string{"quantum computing algorithms"})

	// 1. Search
	buf.Reset()
	_, err := executeCommand(root, "search", "quantum")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(buf.String(), "SearchTarget") {
		t.Errorf("expected search output to contain SearchTarget, got: %s", buf.String())
	}

	// 2. Stats
	buf.Reset()
	_, err = executeCommand(root, "stats")
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Entities:") {
		t.Errorf("expected stats output to contain Entities count, got: %s", buf.String())
	}

	// 3. Graph JSON
	buf.Reset()
	_, err = executeCommand(root, "graph")
	if err != nil {
		t.Fatalf("graph failed: %v", err)
	}
	if !strings.Contains(buf.String(), "SearchTarget") {
		t.Errorf("expected graph output to contain SearchTarget, got: %s", buf.String())
	}

	// 4. Graph DOT format
	buf.Reset()
	_, err = executeCommand(root, "graph", "--format", "dot")
	if err != nil {
		t.Fatalf("graph dot failed: %v", err)
	}
	if !strings.Contains(buf.String(), "digraph memory") {
		t.Errorf("expected graph dot output to contain 'digraph memory', got: %s", buf.String())
	}
}

func TestCLI_SessionCommands(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	s, err := store.CreateSession("test-app")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if err := store.CompleteSession(s.Name, "implemented user login"); err != nil {
		t.Fatalf("CompleteSession failed: %v", err)
	}

	// 1. Session list
	buf.Reset()
	_, err = executeCommand(root, "session", "list")
	if err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	if !strings.Contains(buf.String(), "test-app") {
		t.Errorf("expected session list to contain test-app, got: %s", buf.String())
	}

	// 2. Session get
	buf.Reset()
	_, err = executeCommand(root, "session", "get", s.Name)
	if err != nil {
		t.Fatalf("session get failed: %v", err)
	}
	if !strings.Contains(buf.String(), "implemented user login") {
		t.Errorf("expected session get to show summary, got: %s", buf.String())
	}

	// 3. Session recall
	buf.Reset()
	_, err = executeCommand(root, "session", "recall", "test-app")
	if err != nil {
		t.Fatalf("session recall failed: %v", err)
	}
	if !strings.Contains(buf.String(), "implemented user login") {
		t.Errorf("expected session recall to show summary, got: %s", buf.String())
	}
}

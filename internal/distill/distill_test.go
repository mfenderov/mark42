package distill

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return store
}

func captureSession(t *testing.T, store *storage.Store, project string, events []storage.SessionEvent) string {
	t.Helper()
	session, err := store.CreateSession(project)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for _, evt := range events {
		if err := store.CaptureSessionEvent(session.Name, evt); err != nil {
			t.Fatalf("CaptureSessionEvent failed: %v", err)
		}
	}
	return session.Name
}

func TestExtractFromEvents(t *testing.T) {
	events := []storage.SessionEvent{
		{ToolName: "Edit", FilePath: "/a.go"},
		{ToolName: "Edit", FilePath: "/a.go"},
		{ToolName: "Bash", Command: "go test ./..."},
		{ToolName: "Edit", FilePath: "/b.go"},
		{ToolName: "Bash", Command: "go test ./..."},
		{ToolName: "Write", FilePath: "/c.go"},
	}

	extract := ExtractFromEvents(events)

	if extract.EventCount != 6 {
		t.Errorf("expected EventCount 6, got %d", extract.EventCount)
	}

	// Files deduped and ordered by frequency: /a.go (2), then /b.go and /c.go (1 each)
	if len(extract.Files) != 3 {
		t.Fatalf("expected 3 unique files, got %d: %v", len(extract.Files), extract.Files)
	}
	if extract.Files[0] != "/a.go" {
		t.Errorf("expected most frequent file /a.go first, got %q", extract.Files[0])
	}

	// Commands deduped: go test ./... (2)
	if len(extract.Commands) != 1 {
		t.Fatalf("expected 1 unique command, got %d: %v", len(extract.Commands), extract.Commands)
	}
	if extract.Commands[0] != "go test ./..." {
		t.Errorf("unexpected command: %q", extract.Commands[0])
	}

	// Tools set, sorted
	if len(extract.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(extract.Tools), extract.Tools)
	}
	if extract.Tools[0] != "Bash" || extract.Tools[1] != "Edit" || extract.Tools[2] != "Write" {
		t.Errorf("expected tools sorted [Bash Edit Write], got %v", extract.Tools)
	}
}

func TestExtractFromEvents_TopTenCommands(t *testing.T) {
	var events []storage.SessionEvent
	for i := 0; i < 12; i++ {
		events = append(events, storage.SessionEvent{
			ToolName: "Bash",
			Command:  "cmd-" + string(rune('a'+i)),
		})
	}

	extract := ExtractFromEvents(events)

	if len(extract.Commands) != 10 {
		t.Errorf("expected commands capped at 10, got %d", len(extract.Commands))
	}
}

func TestExtractFromEvents_Empty(t *testing.T) {
	extract := ExtractFromEvents(nil)

	if extract.EventCount != 0 {
		t.Errorf("expected EventCount 0, got %d", extract.EventCount)
	}
	if len(extract.Files) != 0 || len(extract.Commands) != 0 || len(extract.Tools) != 0 {
		t.Errorf("expected empty extract, got %+v", extract)
	}
}

func TestStructuralSummarizer(t *testing.T) {
	extract := Extract{
		EventCount: 12,
		Files:      []string{"cart-service.kt", "order.go"},
		Commands:   []string{"git", "gradle"},
		Tools:      []string{"Bash", "Edit"},
	}

	got := (StructuralSummarizer{}).Summarize(extract)

	if !strings.HasPrefix(got, "Session: 12 tool events") {
		t.Errorf("summary should start with event count, got %q", got)
	}
	if !strings.Contains(got, "2 files touched (cart-service.kt, order.go)") {
		t.Errorf("summary should list files, got %q", got)
	}
	if !strings.Contains(got, "commands: git, gradle") {
		t.Errorf("summary should list commands, got %q", got)
	}
	if !strings.Contains(got, "tools: Bash, Edit") {
		t.Errorf("summary should list tools, got %q", got)
	}
}

func TestStructuralSummarizer_NoEvents(t *testing.T) {
	got := (StructuralSummarizer{}).Summarize(Extract{})

	if got != "Session: 0 tool events" {
		t.Errorf("expected bare summary, got %q", got)
	}
}

func TestRun(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	sessionName := captureSession(t, store, "test-project", []storage.SessionEvent{
		{ToolName: "Edit", FilePath: "/a.go"},
		{ToolName: "Bash", Command: "go test ./..."},
		{ToolName: "Write", FilePath: "/b.go"},
	})

	if err := Run(store, sessionName, StructuralSummarizer{}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	s, err := store.GetSession(sessionName)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if s.Summary != "Session: 3 tool events, 2 files touched (/a.go, /b.go), commands: go test ./..., tools: Bash, Edit, Write" {
		t.Errorf("unexpected summary: %q", s.Summary)
	}
}

func TestRun_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := Run(store, "nonexistent-session", StructuralSummarizer{}); err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRun_ConsumesEvents(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	sessionName := captureSession(t, store, "test-project", []storage.SessionEvent{
		{ToolName: "Edit", FilePath: "/a.go"},
		{ToolName: "Bash", Command: "go test ./..."},
	})

	if err := Run(store, sessionName, StructuralSummarizer{}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	events, err := store.GetSessionEvents(sessionName)
	if err != nil {
		t.Fatalf("GetSessionEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected raw events consumed after distill, got %d", len(events))
	}

	s, err := store.GetSession(sessionName)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if s.Summary == "" {
		t.Error("expected summary to persist after events consumed")
	}
}

func TestRun_SwappableSummarizer(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	sessionName := captureSession(t, store, "test-project", []storage.SessionEvent{
		{ToolName: "Edit", FilePath: "/a.go"},
	})

	custom := customSummarizer{}
	if err := Run(store, sessionName, custom); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	s, err := store.GetSession(sessionName)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if s.Summary != "custom-summary" {
		t.Errorf("expected custom summary, got %q", s.Summary)
	}
}

type customSummarizer struct{}

func (customSummarizer) Summarize(extract Extract) string {
	return "custom-summary"
}

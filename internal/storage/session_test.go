package storage

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func newTestStoreWithMigrations(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return store
}

func TestCreateSession(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	session, err := store.CreateSession("my-project")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.Project != "my-project" {
		t.Errorf("expected project 'my-project', got %q", session.Project)
	}
	if session.Status != "active" {
		t.Errorf("expected status 'active', got %q", session.Status)
	}
	if session.Name == "" {
		t.Error("expected non-empty session name")
	}

	// Verify entity was created
	entity, err := store.GetEntity(session.Name)
	if err != nil {
		t.Fatalf("session entity not found: %v", err)
	}
	if entity.Type != "session" {
		t.Errorf("expected entity type 'session', got %q", entity.Type)
	}

	// Verify metadata in container_tag
	var meta SessionMetadata
	tag, _ := store.GetContainerTag(session.Name)
	if err := json.Unmarshal([]byte(tag), &meta); err != nil {
		t.Fatalf("failed to parse container_tag JSON: %v", err)
	}
	if meta.Project != "my-project" {
		t.Errorf("expected metadata project 'my-project', got %q", meta.Project)
	}
	if meta.Status != "active" {
		t.Errorf("expected metadata status 'active', got %q", meta.Status)
	}
}

func TestCaptureSessionEvent(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	session, err := store.CreateSession("test-project")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	event := SessionEvent{
		ToolName:  "Edit",
		FilePath:  "/internal/storage/session.go",
		Timestamp: "2026-02-12T14:30:00Z",
	}
	if err := store.CaptureSessionEvent(session.Name, event); err != nil {
		t.Fatalf("CaptureSessionEvent failed: %v", err)
	}

	// Verify observation was created with session_event fact type
	entity, err := store.GetEntity(session.Name)
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(entity.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(entity.Observations))
	}

	obs, err := store.GetObservationsByFactType(FactTypeSessionEvent)
	if err != nil {
		t.Fatalf("GetObservationsByFactType failed: %v", err)
	}
	if len(obs) == 0 {
		t.Fatal("expected session_event observations")
	}
}

func TestCompleteSession(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	session, err := store.CreateSession("test-project")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add an event
	store.CaptureSessionEvent(session.Name, SessionEvent{
		ToolName: "Bash", Command: "go test ./...",
	})

	// Complete the session
	if err := store.CompleteSession(session.Name, "Implemented session capture feature with TDD"); err != nil {
		t.Fatalf("CompleteSession failed: %v", err)
	}

	// Verify session status updated
	s, err := store.GetSession(session.Name)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if s.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", s.Status)
	}
	if s.Summary != "Implemented session capture feature with TDD" {
		t.Errorf("unexpected summary: %q", s.Summary)
	}
	if s.EventCount != 1 {
		t.Errorf("expected 1 event, got %d", s.EventCount)
	}
}

func TestGetSession(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	session, _ := store.CreateSession("test-project")
	store.CaptureSessionEvent(session.Name, SessionEvent{ToolName: "Edit", FilePath: "/a.go"})
	store.CaptureSessionEvent(session.Name, SessionEvent{ToolName: "Bash", Command: "go test"})
	store.CompleteSession(session.Name, "Did some work")

	s, err := store.GetSession(session.Name)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if s.Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", s.Project)
	}
	if s.EventCount != 2 {
		t.Errorf("expected 2 events, got %d", s.EventCount)
	}
	if s.Summary != "Did some work" {
		t.Errorf("unexpected summary: %q", s.Summary)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	_, err := store.GetSession("nonexistent-session")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListSessions(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	s1, _ := store.CreateSession("project-a")
	store.CompleteSession(s1.Name, "Work on project A")

	s2, _ := store.CreateSession("project-b")
	store.CompleteSession(s2.Name, "Work on project B")

	store.CreateSession("project-a") // active session

	// List all sessions
	sessions, err := store.ListSessions("", "", 10)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}

	// Filter by project
	sessions, err = store.ListSessions("project-a", "", 10)
	if err != nil {
		t.Fatalf("ListSessions by project failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for project-a, got %d", len(sessions))
	}

	// Filter by status
	sessions, err = store.ListSessions("", "completed", 10)
	if err != nil {
		t.Fatalf("ListSessions by status failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 completed sessions, got %d", len(sessions))
	}

	// Filter by project and status
	sessions, err = store.ListSessions("project-a", "active", 10)
	if err != nil {
		t.Fatalf("ListSessions by project+status failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 active session for project-a, got %d", len(sessions))
	}
}

func TestGetRecentSessionSummaries(t *testing.T) {
	store := newTestStoreWithMigrations(t)
	defer store.Close()

	s1, _ := store.CreateSession("my-project")
	store.CompleteSession(s1.Name, "Implemented auth module")

	s2, _ := store.CreateSession("my-project")
	store.CompleteSession(s2.Name, "Fixed login bug")

	results, err := store.GetRecentSessionSummaries("my-project", 72, 1500)
	if err != nil {
		t.Fatalf("GetRecentSessionSummaries failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("expected at least 2 summaries, got %d", len(results))
	}

	// Verify summaries contain expected content
	found := false
	for _, r := range results {
		if r.Content == "Implemented auth module" || r.Content == "Fixed login bug" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find session summaries in results")
	}
}

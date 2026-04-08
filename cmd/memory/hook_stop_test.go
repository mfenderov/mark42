package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSessionDigest(t *testing.T) {
	t.Run("filters to user and assistant text blocks", func(t *testing.T) {
		fixture := filepath.Join("testdata", "transcript.jsonl")
		digest := buildSessionDigest(fixture)

		if !strings.Contains(digest, "### User") {
			t.Error("digest should contain user message headers")
		}
		if !strings.Contains(digest, "### Assistant") {
			t.Error("digest should contain assistant message headers")
		}
		if !strings.Contains(digest, "Add a new search feature") {
			t.Error("digest should contain user text content")
		}
		if !strings.Contains(digest, "FTS5") {
			t.Error("digest should contain assistant text content")
		}
	})

	t.Run("skips tool_use tool_result thinking progress system", func(t *testing.T) {
		fixture := filepath.Join("testdata", "transcript.jsonl")
		digest := buildSessionDigest(fixture)

		if strings.Contains(digest, "tool_use") {
			t.Error("digest should not contain tool_use references")
		}
		if strings.Contains(digest, "tool_result") {
			t.Error("digest should not contain tool_result")
		}
		if strings.Contains(digest, "Let me think about") {
			t.Error("digest should not contain thinking blocks")
		}
		if strings.Contains(digest, "file-history-snapshot") {
			t.Error("digest should not contain file-history-snapshot")
		}
		if strings.Contains(digest, "System reminder") {
			t.Error("digest should not contain system messages")
		}
	})

	t.Run("truncates long messages at 500 chars", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "transcript.jsonl")

		longMsg := strings.Repeat("x", 1000)
		lines := `{"type":"user","message":{"role":"user","content":"` + longMsg + `"}}` + "\n"
		os.WriteFile(path, []byte(lines), 0o644)

		digest := buildSessionDigest(path)

		// Each message chunk should be at most ~500 chars + truncation marker
		for _, line := range strings.Split(digest, "\n") {
			if len(line) > 520 {
				t.Errorf("line exceeds 500 char limit (got %d chars): %.100s...", len(line), line)
			}
		}
	})

	t.Run("caps total digest at 30KB", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "transcript.jsonl")

		var sb strings.Builder
		msg := strings.Repeat("a", 400)
		for range 200 {
			sb.WriteString(`{"type":"user","message":{"role":"user","content":"` + msg + `"}}` + "\n")
			sb.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + msg + `"}]}}` + "\n")
		}
		os.WriteFile(path, []byte(sb.String()), 0o644)

		digest := buildSessionDigest(path)
		if len(digest) > 30*1024 {
			t.Errorf("digest should be capped at 30KB, got %d bytes", len(digest))
		}
	})

	t.Run("handles missing transcript gracefully", func(t *testing.T) {
		digest := buildSessionDigest("/nonexistent/path.jsonl")
		if digest != "" {
			t.Errorf("expected empty digest for missing file, got %d bytes", len(digest))
		}
	})

	t.Run("handles empty transcript", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.jsonl")
		os.WriteFile(path, []byte(""), 0o644)

		digest := buildSessionDigest(path)
		if digest != "" {
			t.Errorf("expected empty digest for empty file, got: %s", digest)
		}
	})

	t.Run("returns partial content on scanner error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "transcript.jsonl")

		// Normal line followed by oversized line (>1MB)
		normalLine := `{"type":"user","message":{"role":"user","content":"visible content"}}` + "\n"
		oversizedLine := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", 2*1024*1024) + `"}}` + "\n"
		os.WriteFile(path, []byte(normalLine+oversizedLine), 0o644)

		digest := buildSessionDigest(path)
		if !strings.Contains(digest, "visible content") {
			t.Error("digest should contain content from lines before the oversized line")
		}
	})

	t.Run("handles oversized first line gracefully", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "transcript.jsonl")

		// Only an oversized line (>1MB)
		oversizedLine := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", 2*1024*1024) + `"}}` + "\n"
		os.WriteFile(path, []byte(oversizedLine), 0o644)

		digest := buildSessionDigest(path)
		if digest != "" {
			t.Errorf("expected empty digest for oversized-only transcript, got %d bytes", len(digest))
		}
	})

	t.Run("extracts text from mixed assistant blocks", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "transcript.jsonl")
		// Assistant message with text + tool_use blocks (only text should appear)
		line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"visible text"},{"type":"tool_use","name":"Read","input":{}}]}}` + "\n"
		os.WriteFile(path, []byte(line), 0o644)

		digest := buildSessionDigest(path)
		if !strings.Contains(digest, "visible text") {
			t.Error("digest should contain text from mixed blocks")
		}
		if strings.Contains(digest, "Read") {
			t.Error("digest should not contain tool_use name from mixed blocks")
		}
	})
}

func TestStopHookWritesDigest(t *testing.T) {
	t.Run("writes digest to mark42 dir", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		// Create a minimal transcript
		transcriptPath := filepath.Join(dir, "transcript.jsonl")
		line := `{"type":"user","message":{"role":"user","content":"Hello world"}}` + "\n"
		os.WriteFile(transcriptPath, []byte(line), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf), withStopInput(&stopInput{
			TranscriptPath: transcriptPath,
		}))

		digestPath := filepath.Join(m42, "session-digest.md")
		data, err := os.ReadFile(digestPath)
		if err != nil {
			t.Fatalf("digest file not created: %v", err)
		}
		if !strings.Contains(string(data), "Hello world") {
			t.Error("digest file should contain transcript content")
		}
	})

	t.Run("skips digest when no transcript path", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		digestPath := filepath.Join(m42, "session-digest.md")
		_, err := os.Stat(digestPath)
		if err == nil {
			t.Error("digest file should not be created without transcript path")
		}
	})
}

func TestBuildAutoSummaryWithContext(t *testing.T) {
	t.Run("enriches summary with last_assistant_message", func(t *testing.T) {
		files := []string{"main.go"}
		lastMsg := "Implemented the search feature with FTS5 ranking."

		summary := buildAutoSummary(files, lastMsg)
		if !strings.Contains(summary, "Session context:") {
			t.Errorf("summary should contain session context, got: %s", summary)
		}
		if !strings.Contains(summary, "Implemented the search") {
			t.Errorf("summary should contain last message content, got: %s", summary)
		}
	})

	t.Run("truncates long last_assistant_message at 200 chars", func(t *testing.T) {
		files := []string{"main.go"}
		longMsg := strings.Repeat("z", 300)

		summary := buildAutoSummary(files, longMsg)
		contextLine := ""
		for _, line := range strings.Split(summary, ". ") {
			if strings.HasPrefix(line, "Session context:") {
				contextLine = line
				break
			}
		}
		// "Session context: " (18) + 200 chars + "..." (3) = 221 max
		if len(contextLine) > 225 {
			t.Errorf("session context line too long (%d chars): %.100s...", len(contextLine), contextLine)
		}
		if strings.Contains(contextLine, strings.Repeat("z", 250)) {
			t.Error("long message should be truncated, but full content found")
		}
	})

	t.Run("skips context line when no last message", func(t *testing.T) {
		files := []string{"main.go"}

		summary := buildAutoSummary(files, "")
		if strings.Contains(summary, "Session context:") {
			t.Errorf("summary should not contain session context when empty, got: %s", summary)
		}
	})
}

func TestHookStop(t *testing.T) {
	t.Run("silent output when files edited", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"),
			[]byte("src/main.go\nsrc/lib.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output (silent approve), got: %s", buf.String())
		}

		// Verify side effect: buffers cleared
		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared")
		}
	})

	t.Run("silent output with no dirty files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output (silent approve), got: %s", buf.String())
		}
	})

	t.Run("no output when no events and no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("truly empty session should produce no output, got: %s", buf.String())
		}
	})

	t.Run("flag guard prevents re-entry", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf1 captureBuffer
		runStopHook(dir, withOutput(&buf1))

		// First call should have cleared dirty-files (proves it ran)
		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("first call should clear dirty-files")
		}

		// Write new dirty files for second call
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("b.go\n"), 0o644)

		var buf2 captureBuffer
		runStopHook(dir, withOutput(&buf2))

		// Second call should NOT have cleared dirty-files (flag guard blocked it)
		dirty2, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty2)) == "" {
			t.Error("second call should be blocked by flag guard (dirty-files should remain)")
		}
	})

	t.Run("clears dirty-files after output", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared by hook")
		}
	})

	t.Run("no output without project dir", func(t *testing.T) {
		var buf captureBuffer
		runStopHook("", withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("expected empty output, got: %s", buf.String())
		}
	})

	t.Run("produces no output when only dirty files present", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output, got: %s", buf.String())
		}
	})

	t.Run("completes existing session when current-session file present", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		store := newTestStore(t, t.TempDir())

		session, _ := store.CreateSession(filepath.Base(dir))
		os.WriteFile(filepath.Join(m42, "current-session"), []byte(session.Name), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("main.go\n"), 0o644)

		runStopHook(dir, withStore(store))

		got, err := store.GetSession(session.Name)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("session status = %q, want completed", got.Status)
		}
		if got.Summary == "" {
			t.Error("session should have a non-empty summary")
		}

		if _, err := os.Stat(filepath.Join(m42, "current-session")); !os.IsNotExist(err) {
			t.Error("current-session file should be deleted after stop")
		}
	})

	t.Run("creates fallback session when current-session file absent", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		store := newTestStore(t, t.TempDir())

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("main.go\n"), 0o644)

		runStopHook(dir, withStore(store))

		sessions, _ := store.ListSessions(filepath.Base(dir), "completed", 10)
		if len(sessions) != 1 {
			t.Errorf("expected 1 completed fallback session, got %d", len(sessions))
		}
	})

	t.Run("creates fallback session when current-session file has stale name", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		store := newTestStore(t, t.TempDir())

		// Write a session name that doesn't exist in the database (crashed session)
		os.WriteFile(filepath.Join(m42, "current-session"), []byte("session-stale-name-does-not-exist"), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("main.go\n"), 0o644)

		runStopHook(dir, withStore(store))

		sessions, _ := store.ListSessions(filepath.Base(dir), "completed", 10)
		if len(sessions) != 1 {
			t.Errorf("expected 1 fallback completed session, got %d", len(sessions))
		}
	})
}

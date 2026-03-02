package main

import (
	"encoding/json"
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

		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Edit"}`+"\n"), 0o644)

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

		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Edit"}`+"\n"), 0o644)

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
		type eventEntry struct {
			ToolName string `json:"toolName"`
		}
		events := []eventEntry{{ToolName: "Edit"}}
		files := []string{"main.go"}
		lastMsg := "Implemented the search feature with FTS5 ranking."

		summary := buildAutoSummary(events, files, lastMsg)
		if !strings.Contains(summary, "Session context:") {
			t.Errorf("summary should contain session context, got: %s", summary)
		}
		if !strings.Contains(summary, "Implemented the search") {
			t.Errorf("summary should contain last message content, got: %s", summary)
		}
	})

	t.Run("truncates long last_assistant_message at 200 chars", func(t *testing.T) {
		type eventEntry struct {
			ToolName string `json:"toolName"`
		}
		events := []eventEntry{{ToolName: "Edit"}}
		files := []string{"main.go"}
		longMsg := strings.Repeat("z", 300)

		summary := buildAutoSummary(events, files, longMsg)
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
		type eventEntry struct {
			ToolName string `json:"toolName"`
		}
		events := []eventEntry{{ToolName: "Edit"}}
		files := []string{"main.go"}

		summary := buildAutoSummary(events, files, "")
		if strings.Contains(summary, "Session context:") {
			t.Errorf("summary should not contain session context when empty, got: %s", summary)
		}
	})
}

func TestHookStop(t *testing.T) {
	t.Run("full mode systemMessage when files edited", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"),
			[]byte("src/main.go\nsrc/lib.go\n"), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"),
			[]byte(`{"toolName":"Edit","filePath":"/a.go"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, got)
		}

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("systemMessage is not a string")
		}
		if !strings.Contains(msg, "full") {
			t.Errorf("systemMessage should contain 'full' mode, got: %s", msg)
		}
		if !strings.Contains(msg, "1 events") {
			t.Errorf("systemMessage should contain event count, got: %s", msg)
		}

		// Async hooks should NOT have decision or suppressOutput
		if _, exists := output["decision"]; exists {
			t.Error("async hook should not output decision field")
		}
		if _, exists := output["suppressOutput"]; exists {
			t.Error("async hook should not output suppressOutput field")
		}
	})

	t.Run("knowledge-only mode systemMessage with events but no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Read"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output for knowledge-only session")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, got)
		}

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("systemMessage is not a string")
		}
		if !strings.Contains(msg, "knowledge-only") {
			t.Errorf("systemMessage should contain 'knowledge-only' mode, got: %s", msg)
		}
	})

	t.Run("no output when no events and no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(""), 0o644)

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

		var buf1, buf2 captureBuffer
		runStopHook(dir, withOutput(&buf1))
		runStopHook(dir, withOutput(&buf2))

		if buf1.String() == "" {
			t.Error("first call should produce output")
		}
		if buf2.String() != "" {
			t.Errorf("second call should be silent (flag guard), got: %s", buf2.String())
		}
	})

	t.Run("clears both buffers after output", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(`{"toolName":"Edit"}`+"\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared by hook")
		}

		events, _ := os.ReadFile(filepath.Join(m42, "session-events"))
		if strings.TrimSpace(string(events)) != "" {
			t.Error("session-events should be cleared")
		}
	})

	t.Run("no output without project dir", func(t *testing.T) {
		var buf captureBuffer
		runStopHook("", withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("expected empty output, got: %s", buf.String())
		}
	})

	t.Run("caps events at 50 with systemMessage", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		var sb strings.Builder
		for range 60 {
			sb.WriteString(`{"toolName":"Edit"}` + "\n")
		}
		os.WriteFile(filepath.Join(m42, "session-events"), []byte(sb.String()), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		got := strings.TrimSpace(buf.String())
		if got == "" {
			t.Fatal("expected output with dirty files")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(got), &output); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("systemMessage is not a string")
		}
		if !strings.Contains(msg, "50 events") {
			t.Errorf("systemMessage should show capped event count (50), got: %s", msg)
		}
	})
}

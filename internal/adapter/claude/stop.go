package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/state"
	"github.com/mfenderov/mark42/internal/storage"
)

// Stop runs the Stop hook: complete the session and sync CC memory files.
func Stop(projectDir string, opts ...Option) {
	if projectDir == "" {
		return
	}

	cfg := &Config{}
	for _, o := range opts {
		o(cfg)
	}

	m42 := mark42Dir(projectDir)

	// Flag guard — only fire once per session
	if !touchFlag(filepath.Join(m42, "stop-prompted")) {
		return
	}

	projectName := filepath.Base(projectDir)
	sessionName := currentSessionName(projectDir)

	// Read dirty files
	files := readLines(filepath.Join(m42, "dirty-files"))

	// Build and write session digest from transcript
	lastMsg := writeSessionDigest(m42, cfg.stopInput)

	summary := buildAutoSummary(files, lastMsg)
	completeSession(projectName, sessionName, summary, cfg.store)

	// Remove current-session file — session lifecycle is complete
	_ = os.Remove(state.CurrentSessionPath(projectDir))
	_ = os.Remove(legacyCurrentSessionPath(projectDir))

	// Clear dirty-files buffer
	clearFile(filepath.Join(m42, "dirty-files"))

	// Sync CC auto-memory files into mark42 (silent, non-blocking)
	syncCCMemoryIfAvailable(projectDir, m42)

	// Stop hook: silent exit 0 = approve stop (no JSON output).
	// Only output JSON with {"decision":"block"} when blocking.
}

// currentSessionName reads the current-session file written by SessionStart.
func currentSessionName(projectDir string) string {
	sessionNameBytes, _ := os.ReadFile(state.CurrentSessionPath(projectDir))
	if len(sessionNameBytes) == 0 {
		sessionNameBytes, _ = os.ReadFile(legacyCurrentSessionPath(projectDir))
	}
	return strings.TrimSpace(string(sessionNameBytes))
}

// writeSessionDigest builds and writes the session digest from the transcript.
// Returns the last assistant message from the stop input ("" when absent).
func writeSessionDigest(m42 string, input *StopInput) string {
	if input == nil {
		return ""
	}
	if input.TranscriptPath != "" {
		if digest := buildSessionDigest(input.TranscriptPath); digest != "" {
			_ = os.WriteFile(filepath.Join(m42, "session-digest.md"), []byte(digest), 0o644)
		}
	}
	return input.LastAssistantMessage
}

// syncCCMemoryIfAvailable syncs CC auto-memory files into mark42 when a memory
// directory exists. Silent and non-blocking on any failure.
func syncCCMemoryIfAvailable(projectDir, m42 string) {
	memDir := ccMemoryDir(projectDir)
	if memDir == "" {
		return
	}
	syncStore, err := StoreFactory()
	if err != nil {
		logger.Warn("failed to open store for cc memory sync", "err", err)
		return
	}
	defer syncStore.Close()
	syncCCMemory(state.ProjectSlug(projectDir), memDir, syncStore, filepath.Join(m42, "memory-checksums.json"))
}

func completeSession(projectName, sessionName, summary string, storeOverride *storage.Store) {
	store := storeOverride
	if store == nil {
		var err error
		store, err = StoreFactory()
		if err != nil {
			return
		}
		defer store.Close()
	}

	if sessionName != "" {
		if err := store.CompleteSession(sessionName, summary); err == nil {
			return
		}
		// any error (stale session name, DB failure) — fall through to create fallback
	}

	// Fallback: create and complete (SessionStart never ran, or stale session name)
	session, err := store.CreateSession(projectName)
	if err != nil {
		return
	}
	_ = store.CompleteSession(session.Name, summary)
}

const (
	maxMessageLen = 500
	maxDigestSize = 30 * 1024
	maxContextLen = 200
)

func buildSessionDigest(transcriptPath string) string {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		if sb.Len() >= maxDigestSize {
			break
		}
		appendMessageText(&sb, scanner.Bytes())
	}

	if err := scanner.Err(); err != nil && sb.Len() == 0 {
		fmt.Fprintf(os.Stderr, "[mark42] digest scanner error: %v\n", err)
	}

	result := strings.TrimSpace(sb.String())
	if len(result) > maxDigestSize {
		result = result[:maxDigestSize]
	}
	return result
}

// appendMessageText formats one transcript line into the digest.
// Unparseable lines and non-text messages are skipped.
func appendMessageText(sb *strings.Builder, line []byte) {
	var msg transcriptMessage
	if json.Unmarshal(line, &msg) != nil {
		return
	}

	var label, text string
	switch msg.Type {
	case "user":
		label, text = "User", extractUserText(msg.Message)
	case "assistant":
		label, text = "Assistant", extractAssistantText(msg.Message)
	}
	if text == "" {
		return
	}

	sb.WriteString("### " + label + "\n")
	sb.WriteString(truncate(text, maxMessageLen))
	sb.WriteString("\n\n")
}

type transcriptMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type messageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractUserText(raw json.RawMessage) string {
	var env messageEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return ""
	}

	// User content is either a string or an array of blocks
	var s string
	if json.Unmarshal(env.Content, &s) == nil {
		return s
	}

	// If it's an array, it's tool_results — skip
	return ""
}

func extractAssistantText(raw json.RawMessage) string {
	var env messageEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return ""
	}

	var blocks []contentBlock
	if json.Unmarshal(env.Content, &blocks) != nil {
		return ""
	}

	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func buildAutoSummary(files []string, lastMsg string) string {
	if len(files) == 0 && lastMsg == "" {
		return "Session with no tracked changes."
	}

	var parts []string
	if filesPart := summarizeFiles(files); filesPart != "" {
		parts = append(parts, filesPart)
	}
	if lastMsg != "" {
		parts = append(parts, "Session context: "+truncate(lastMsg, maxContextLen))
	}

	if len(parts) == 0 {
		return "Session with no tracked changes."
	}
	return strings.Join(parts, ". ") + "."
}

// summarizeFiles builds the "Modified N files: ..." summary part ("" when no files).
func summarizeFiles(files []string) string {
	if len(files) == 0 {
		return ""
	}

	names := make([]string, 0, len(files))
	for _, f := range files {
		base, _, _ := strings.Cut(f, " [")
		names = append(names, filepath.Base(base))
	}
	unique := dedupePreserveOrder(names)

	if len(unique) <= 5 {
		return fmt.Sprintf("Modified %d files: %s", len(unique), strings.Join(unique, ", "))
	}
	return fmt.Sprintf("Modified %d files: %s, +%d more", len(unique), strings.Join(unique[:5], ", "), len(unique)-5)
}

// dedupePreserveOrder removes duplicates while keeping first-seen order.
func dedupePreserveOrder(names []string) []string {
	seen := map[string]bool{}
	unique := names[:0]
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}
	return unique
}

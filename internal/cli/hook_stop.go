package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

type stopInput struct {
	TranscriptPath       string `json:"transcript_path"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
}

func withStopInput(input *stopInput) hookOption {
	return func(cfg *hookConfig) {
		cfg.stopInput = input
	}
}

var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop hook: trigger memory sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		var input stopInput
		_ = readStdinJSON(&input)

		runStopHook(projectDir, withStopInput(&input))
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookStopCmd)
}

func runStopHook(projectDir string, opts ...hookOption) {
	if projectDir == "" {
		return
	}

	cfg := &hookConfig{}
	for _, o := range opts {
		o(cfg)
	}

	m42 := mark42Dir(projectDir)

	// Flag guard — only fire once per session
	if !touchFlag(filepath.Join(m42, "stop-prompted")) {
		return
	}

	projectName := filepath.Base(projectDir)

	// Read current-session file written by SessionStart
	sessionNameBytes, _ := os.ReadFile(currentSessionPath(projectDir))
	if len(sessionNameBytes) == 0 {
		sessionNameBytes, _ = os.ReadFile(legacyCurrentSessionPath(projectDir))
	}
	sessionName := strings.TrimSpace(string(sessionNameBytes))

	// Read dirty files
	files := readLines(filepath.Join(m42, "dirty-files"))

	// Build and write session digest from transcript
	var lastMsg string
	if cfg.stopInput != nil {
		lastMsg = cfg.stopInput.LastAssistantMessage
		if cfg.stopInput.TranscriptPath != "" {
			digest := buildSessionDigest(cfg.stopInput.TranscriptPath)
			if digest != "" {
				_ = os.WriteFile(filepath.Join(m42, "session-digest.md"), []byte(digest), 0o644)
			}
		}
	}

	summary := buildAutoSummary(files, lastMsg)
	completeSession(projectName, sessionName, summary, cfg.store)

	// Remove current-session file — session lifecycle is complete
	_ = os.Remove(currentSessionPath(projectDir))
	_ = os.Remove(legacyCurrentSessionPath(projectDir))

	// Clear dirty-files buffer
	clearFile(filepath.Join(m42, "dirty-files"))

	// Sync CC auto-memory files into mark42 (silent, non-blocking)
	if memDir := ccMemoryDir(projectDir); memDir != "" {
		if syncStore, err := getStore(); err == nil {
			defer syncStore.Close()
			syncCCMemory(projectSlug(projectDir), memDir, syncStore, filepath.Join(m42, "memory-checksums.json"))
		} else {
			logger.Warn("failed to open store for cc memory sync", "err", err)
		}
	}

	// Stop hook: silent exit 0 = approve stop (no JSON output).
	// Only output JSON with {"decision":"block"} when blocking.
}

func completeSession(projectName, sessionName, summary string, storeOverride *storage.Store) {
	store := storeOverride
	if store == nil {
		var err error
		store, err = getStore()
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

		line := scanner.Bytes()
		var msg transcriptMessage
		if json.Unmarshal(line, &msg) != nil {
			continue
		}

		switch msg.Type {
		case "user":
			text := extractUserText(msg.Message)
			if text == "" {
				continue
			}
			text = truncate(text, maxMessageLen)
			sb.WriteString("### User\n")
			sb.WriteString(text)
			sb.WriteString("\n\n")

		case "assistant":
			text := extractAssistantText(msg.Message)
			if text == "" {
				continue
			}
			text = truncate(text, maxMessageLen)
			sb.WriteString("### Assistant\n")
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
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

	if len(files) > 0 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			base, _, _ := strings.Cut(f, " [")
			name := filepath.Base(base)
			names = append(names, name)
		}
		seen := map[string]bool{}
		unique := names[:0]
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				unique = append(unique, n)
			}
		}
		if len(unique) <= 5 {
			parts = append(parts, fmt.Sprintf("Modified %d files: %s", len(unique), strings.Join(unique, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("Modified %d files: %s, +%d more", len(unique), strings.Join(unique[:5], ", "), len(unique)-5))
		}
	}

	if lastMsg != "" {
		parts = append(parts, "Session context: "+truncate(lastMsg, maxContextLen))
	}

	if len(parts) == 0 {
		return "Session with no tracked changes."
	}
	return strings.Join(parts, ". ") + "."
}

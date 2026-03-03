package main

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

	// Read session events
	type eventEntry struct {
		ToolName  string `json:"toolName"`
		FilePath  string `json:"filePath,omitempty"`
		Command   string `json:"command,omitempty"`
		Timestamp string `json:"timestamp,omitempty"`
	}
	events := readJSONLines[eventEntry](filepath.Join(m42, "session-events"))
	if len(events) > 50 {
		events = events[:50]
	}

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

	// Capture session directly in SQLite (silent, no blocking)
	captureSessionDirectly(projectName, events, files, lastMsg)

	// Clear both buffers (deterministic cleanup — don't rely on agent)
	clearFile(filepath.Join(m42, "session-events"))
	clearFile(filepath.Join(m42, "dirty-files"))

	if len(events) == 0 && len(files) == 0 {
		return
	}

	mode := "knowledge-only"
	if len(files) > 0 {
		mode = "full"
	}

	output := map[string]any{
		"systemMessage": fmt.Sprintf("mark42: Session captured (%d events, %s mode)", len(events), mode),
	}
	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))
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

func captureSessionDirectly[E any](projectName string, events []E, files []string, lastMsg string) {
	store, err := getStore()
	if err != nil {
		return // fail silently
	}
	defer store.Close()

	session, err := store.CreateSession(projectName)
	if err != nil {
		return
	}

	// Store each event as observation
	for _, evt := range events {
		raw, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		var se storage.SessionEvent
		if err := json.Unmarshal(raw, &se); err != nil {
			continue
		}
		_ = store.CaptureSessionEvent(session.Name, se)
	}

	// Auto-generate summary from events and files
	summary := buildAutoSummary(events, files, lastMsg)
	_ = store.CompleteSession(session.Name, summary)
}

func buildAutoSummary[E any](events []E, files []string, lastMsg string) string {
	if len(events) == 0 && len(files) == 0 && lastMsg == "" {
		return "Session with no tracked changes."
	}

	var parts []string

	// Summarize files
	if len(files) > 0 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			name := filepath.Base(strings.SplitN(f, " [", 2)[0])
			names = append(names, name)
		}
		// Deduplicate
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

	// Count tool usage
	type eventEntry struct {
		ToolName string `json:"toolName"`
	}
	toolCounts := map[string]int{}
	for _, evt := range events {
		raw, _ := json.Marshal(evt)
		var e eventEntry
		if json.Unmarshal(raw, &e) == nil && e.ToolName != "" {
			toolCounts[e.ToolName]++
		}
	}
	if len(toolCounts) > 0 {
		var tools []string
		for tool, count := range toolCounts {
			tools = append(tools, fmt.Sprintf("%d %s", count, tool))
		}
		parts = append(parts, fmt.Sprintf("%d tool calls (%s)", len(events), strings.Join(tools, ", ")))
	}

	// Add session context from last assistant message
	if lastMsg != "" {
		parts = append(parts, "Session context: "+truncate(lastMsg, maxContextLen))
	}

	if len(parts) == 0 {
		return "Session with no tracked changes."
	}
	return strings.Join(parts, ". ") + "."
}

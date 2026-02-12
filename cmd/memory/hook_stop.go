package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/storage"
	"github.com/spf13/cobra"
)

var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop hook: trigger memory sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}
		runStopHook(projectDir)
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

	// Capture session directly in SQLite (no Claude involvement needed)
	captureSessionDirectly(projectName, events, files)

	// Clear buffers
	clearFile(filepath.Join(m42, "dirty-files"))
	clearFile(filepath.Join(m42, "session-events"))

	// Block with minimal reason — only ask Claude for learnings
	output := map[string]any{
		"decision":       "block",
		"reason":         "💾 Sync learnings for " + projectName + ". Use create_or_update_entities for new learnings, add_observations for updates. Use fact_type='static' for conventions/patterns, 'dynamic' for decisions/context. Reply only: 'Synced N learnings.'",
		"suppressOutput": true,
	}

	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))
}

func captureSessionDirectly[E any](projectName string, events []E, files []string) {
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
	summary := buildAutoSummary(events, files)
	_ = store.CompleteSession(session.Name, summary)
}

func buildAutoSummary[E any](events []E, files []string) string {
	if len(events) == 0 && len(files) == 0 {
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

	if len(parts) == 0 {
		return "Session with no tracked changes."
	}
	return strings.Join(parts, ". ") + "."
}

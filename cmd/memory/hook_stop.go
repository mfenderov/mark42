package main

import (
	"encoding/json"
	"path/filepath"
	"strings"

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

	// Read dirty files (cap at 5 for display)
	files := readLines(filepath.Join(m42, "dirty-files"))
	displayFiles := files
	if len(displayFiles) > 5 {
		displayFiles = displayFiles[:5]
	}

	// Read session events (cap at 50)
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

	filesList := "none"
	if len(displayFiles) > 0 {
		filesList = strings.Join(displayFiles, ", ")
	}

	eventsJSON, _ := json.Marshal(events)
	eventCount := len(events)

	output := map[string]any{
		"decision": "block",
		"reason": "Memory sync for " + projectName + ". " +
			"Modified: " + filesList + ". Events: " + itoa(eventCount) + ". " +
			"1. Call capture_session with projectName='" + projectName + "', " +
			"a brief summary of what was done, and events=" + string(eventsJSON) + ". " +
			"2. Use create_or_update_entities for new learnings, add_observations for updates. " +
			"Use fact_type='static' for conventions/patterns, 'dynamic' for decisions/context. " +
			"Reply only: 'Synced N learnings.'",
		"suppressOutput": true,
	}

	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))

	// Clear buffers
	clearFile(filepath.Join(m42, "dirty-files"))
	clearFile(filepath.Join(m42, "session-events"))
}

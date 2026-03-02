package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type subagentStopInput struct {
	AgentType            string `json:"agent_type"`
	LastAssistantMessage string `json:"last_assistant_message"`
	TranscriptPath       string `json:"agent_transcript_path,omitempty"`
}

var hookSubagentStopCmd = &cobra.Command{
	Use:   "subagent-stop",
	Short: "SubagentStop hook: capture agent results",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		var input subagentStopInput
		if err := readStdinJSON(&input); err != nil {
			return nil
		}

		runSubagentStopHook(projectDir, input)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookSubagentStopCmd)
}

func runSubagentStopHook(projectDir string, input subagentStopInput, opts ...hookOption) {
	if projectDir == "" {
		return
	}

	if input.AgentType != "knowledge-extractor" {
		return
	}

	cfg := &hookConfig{}
	for _, o := range opts {
		o(cfg)
	}

	summary := input.LastAssistantMessage
	if len(summary) > 500 {
		summary = summary[:500]
	}

	output := map[string]any{
		"systemMessage": fmt.Sprintf("mark42: %s completed — %s", input.AgentType, summary),
	}
	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))
}

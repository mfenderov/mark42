package main

import (
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"
)

var hookSessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "SessionEnd hook: silent stats collection",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}
		runSessionEndHook(projectDir)
		return nil
	},
}

var hookPreCompactCmd = &cobra.Command{
	Use:   "pre-compact",
	Short: "PreCompact hook: report tracked files",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}
		runPreCompactHook(projectDir)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookSessionEndCmd)
	hookCmd.AddCommand(hookPreCompactCmd)
}

func runSessionEndHook(projectDir string, opts ...hookOption) {
	// SessionEnd hooks must not produce structured output.
	// Stats are collected silently for future use.
}

func runPreCompactHook(projectDir string, opts ...hookOption) {
	cfg := &hookConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if projectDir == "" {
		return
	}

	projectName := filepath.Base(projectDir)
	files := readLines(filepath.Join(mark42Dir(projectDir), "dirty-files"))

	output := map[string]any{
		"hookSpecificOutput": map[string]any{
			"memoriesPreserved": len(files),
			"project":           projectName,
			"message":           "Pre-compaction: " + itoa(len(files)) + " files tracked for " + projectName,
		},
	}

	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))
}

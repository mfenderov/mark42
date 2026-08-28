package cli

import (
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"
)

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
	hookCmd.AddCommand(hookPreCompactCmd)
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
		"systemMessage": "mark42: pre-compact — " + itoa(len(files)) + " files tracked for " + projectName,
	}

	data, _ := json.Marshal(output)
	hookPrint(cfg, string(data))
}

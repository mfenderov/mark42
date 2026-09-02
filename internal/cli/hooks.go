package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/adapter/claude"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Claude Code hook handlers",
	Long:  "Subcommands for each Claude Code lifecycle hook event.",
}

func init() {
	rootCmd.AddCommand(hookCmd)
	claude.StoreFactory = getStore
}

func getProjectDir() string {
	return os.Getenv("CLAUDE_PROJECT_DIR")
}

var hookSessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "SessionStart hook: inject context",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		store, err := getStore()
		if err != nil {
			return nil
		}
		defer store.Close()
		_ = store.Migrate()

		claude.SessionStart(projectDir, store)
		return nil
	},
}

var hookPostToolUseCmd = &cobra.Command{
	Use:   "post-tool-use",
	Short: "PostToolUse hook: track file modifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		input, err := claude.ParsePostToolUseInput(os.Stdin)
		if err != nil {
			return nil
		}

		claude.PostToolUse(projectDir, input)
		return nil
	},
}

var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop hook: trigger memory sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		input, _ := claude.ParseStopInput(os.Stdin)

		claude.Stop(projectDir, claude.WithStopInput(&input))
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
		claude.PreCompact(projectDir)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookSessionStartCmd)
	hookCmd.AddCommand(hookPostToolUseCmd)
	hookCmd.AddCommand(hookStopCmd)
	hookCmd.AddCommand(hookPreCompactCmd)
}

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/distill"
	"github.com/mfenderov/mark42/internal/storage"
)

// --- Session commands ---

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage session capture and recall",
}

var sessionCaptureCmd = &cobra.Command{
	Use:   "capture <project>",
	Short: "Capture a session from JSON on stdin",
	Long: `Capture a session with summary and events from JSON on stdin.

Input format:
  {"summary": "What was done", "events": [{"toolName": "Edit", "filePath": "/a.go"}]}`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		var input struct {
			Summary string `json:"summary"`
			Events  []struct {
				ToolName  string `json:"toolName"`
				FilePath  string `json:"filePath,omitempty"`
				Command   string `json:"command,omitempty"`
				Timestamp string `json:"timestamp,omitempty"`
			} `json:"events"`
		}

		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			return fmt.Errorf("failed to read JSON from stdin: %w", err)
		}

		session, err := store.CreateSession(args[0])
		if err != nil {
			return err
		}

		if projectDir := getProjectDir(); projectDir != "" {
			writeCurrentSession(projectDir, session.Name)
		}

		for _, evt := range input.Events {
			_ = store.CaptureSessionEvent(session.Name, storage.SessionEvent{
				ToolName:  evt.ToolName,
				FilePath:  evt.FilePath,
				Command:   evt.Command,
				Timestamp: evt.Timestamp,
			})
		}

		if err := store.CompleteSession(session.Name, input.Summary); err != nil {
			return err
		}

		output(successStyle.Render("✓") + " Session captured: " + entityStyle.Render(session.Name))
		output("  " + dimStyle.Render("Events:") + "  " + itoa(len(input.Events)))
		output("  " + dimStyle.Render("Summary:") + " " + input.Summary)
		return nil
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List captured sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		project, _ := cmd.Flags().GetString("project")
		limit, _ := cmd.Flags().GetInt("limit")

		sessions, err := store.ListSessions(project, "", limit)
		if err != nil {
			return err
		}

		if len(sessions) == 0 {
			logger.Info("No sessions found")
			return nil
		}

		output(titleStyle.Render("Sessions"))
		output()
		for _, s := range sessions {
			status := dimStyle.Render("[" + s.Status + "]")
			output("  " + entityStyle.Render(s.Name) + " " + status)
			if s.Project != "" {
				output("    " + dimStyle.Render("Project:") + " " + s.Project)
			}
		}
		return nil
	},
}

var sessionGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get session details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		session, err := store.GetSession(args[0])
		if err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Session not found", "name", args[0])
				os.Exit(1)
			}
			return err
		}

		output(titleStyle.Render(session.Name))
		output()
		output("  " + dimStyle.Render("Project:") + "  " + session.Project)
		output("  " + dimStyle.Render("Status:") + "   " + session.Status)
		output("  " + dimStyle.Render("Events:") + "   " + itoa(session.EventCount))
		if session.Summary != "" {
			output("  " + dimStyle.Render("Summary:") + "  " + session.Summary)
		}
		return nil
	},
}

var sessionRecallCmd = &cobra.Command{
	Use:   "recall [project]",
	Short: "Recall recent session summaries",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		var project string
		if len(args) > 0 {
			project = args[0]
		}
		hours, _ := cmd.Flags().GetInt("hours")
		tokens, _ := cmd.Flags().GetInt("tokens")

		results, err := store.GetRecentSessionSummaries(project, hours, tokens)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No recent sessions found")
			return nil
		}

		formatted := storage.FormatSessionRecall(results)
		print(formatted)
		return nil
	},
}

var distillCmd = &cobra.Command{
	Use:   "distill <session-name>",
	Short: "Distill a session's raw events into a summary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		sessionName := args[0]

		if err := distill.Run(store, sessionName, distill.StructuralSummarizer{}); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				logger.Error("Session not found", "name", sessionName)
				os.Exit(1)
			}
			if errors.Is(err, distill.ErrNothingToDistill) {
				output(dimStyle.Render("Nothing to distill:") + " " + entityStyle.Render(sessionName))
				return nil
			}
			return err
		}

		consolidated, err := store.ConsolidateObservations(sessionName)
		if err != nil {
			return err
		}

		output(successStyle.Render("✓") + " Distilled: " + entityStyle.Render(sessionName))
		output("  " + dimStyle.Render("Consolidate:") + " " + consolidated)
		return nil
	},
}

func init() {
	sessionListCmd.Flags().String("project", "", "filter by project name")
	sessionListCmd.Flags().Int("limit", 20, "maximum number of sessions")

	sessionRecallCmd.Flags().Int("hours", 72, "time window in hours")
	sessionRecallCmd.Flags().Int("tokens", 1500, "token budget")

	sessionCmd.AddCommand(sessionCaptureCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionGetCmd)
	sessionCmd.AddCommand(sessionRecallCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(distillCmd)
}

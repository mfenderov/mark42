package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Context command ---

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Get memories optimized for context injection",
	Long: `Get memories optimized for context injection at session start.

Orders by fact type (static > dynamic > session_turn), then by importance.
Respects token budget to avoid context overflow.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations
		if err := store.Migrate(); err != nil {
			return err
		}

		tokenBudget, _ := cmd.Flags().GetInt("token-budget")
		minImportance, _ := cmd.Flags().GetFloat64("min-importance")
		projectName, _ := cmd.Flags().GetString("project")

		cfg := storage.DefaultContextConfig()
		if tokenBudget > 0 {
			cfg.TokenBudget = tokenBudget
		}
		if minImportance > 0 {
			cfg.MinImportance = minImportance
		}

		results, err := store.GetContextForInjection(cfg, projectName, "", nil)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No relevant memories found")
			return nil
		}

		formatted := storage.FormatContextResults(results)
		estimatedTokens := storage.EstimateTokens(formatted)

		output(titleStyle.Render("Context for Injection"))
		output(dimStyle.Render(fmt.Sprintf("[%d estimated tokens, %d memories]", estimatedTokens, len(results))))
		output()
		print(formatted)

		return nil
	},
}

func init() {
	contextCmd.Flags().Int("token-budget", 2000, "maximum tokens to include")
	contextCmd.Flags().Float64("min-importance", 0.3, "minimum importance score (0-1)")
	contextCmd.Flags().String("project", "", "project name for boosting relevant memories")

	rootCmd.AddCommand(contextCmd)
}

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// --- Importance commands ---

var importanceCmd = &cobra.Command{
	Use:   "importance",
	Short: "Manage memory importance scores",
}

var importanceRecalculateCmd = &cobra.Command{
	Use:   "recalculate",
	Short: "Recalculate importance scores for all memories",
	Long: `Recalculate importance scores based on:
- Recency (how recently accessed)
- Centrality (how connected via relations)
- Fact type (static facts get bonus)

This helps prioritize which memories to include in context injection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations to ensure schema is up to date
		if err := store.Migrate(); err != nil {
			return err
		}

		start := time.Now()
		updated, err := store.RecalculateImportance()
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Importance Recalculation"))
		output()
		output("  " + dimStyle.Render("Updated:") + " " + successStyle.Render(itoa(updated)) + " observations")
		output("  " + dimStyle.Render("Time:") + "    " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var importanceStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show importance score statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Get importance distribution
		type stats struct {
			Total     int     `db:"total"`
			AvgScore  float64 `db:"avg_score"`
			MinScore  float64 `db:"min_score"`
			MaxScore  float64 `db:"max_score"`
			HighCount int     `db:"high_count"`
			LowCount  int     `db:"low_count"`
		}
		var s stats
		err = store.DB().Get(&s, `
			SELECT
				COUNT(*) as total,
				COALESCE(AVG(importance), 0) as avg_score,
				COALESCE(MIN(importance), 0) as min_score,
				COALESCE(MAX(importance), 0) as max_score,
				SUM(CASE WHEN importance >= 0.7 THEN 1 ELSE 0 END) as high_count,
				SUM(CASE WHEN importance < 0.3 THEN 1 ELSE 0 END) as low_count
			FROM observations o
			JOIN entities e ON e.id = o.entity_id
			WHERE e.is_latest = 1
		`)
		if err != nil {
			return err
		}

		output(titleStyle.Render("Importance Statistics"))
		output()
		output("  " + dimStyle.Render("Total observations:") + " " + itoa(s.Total))
		output("  " + dimStyle.Render("Average score:") + "      " + fmt.Sprintf("%.3f", s.AvgScore))
		output("  " + dimStyle.Render("Min score:") + "          " + fmt.Sprintf("%.3f", s.MinScore))
		output("  " + dimStyle.Render("Max score:") + "          " + fmt.Sprintf("%.3f", s.MaxScore))
		output()
		output("  " + dimStyle.Render("High importance (≥0.7):") + " " + successStyle.Render(itoa(s.HighCount)))
		output("  " + dimStyle.Render("Low importance (<0.3):") + "  " + dimStyle.Render(itoa(s.LowCount)))

		return nil
	},
}

func init() {
	importanceCmd.AddCommand(importanceRecalculateCmd)
	importanceCmd.AddCommand(importanceStatsCmd)
	rootCmd.AddCommand(importanceCmd)
}

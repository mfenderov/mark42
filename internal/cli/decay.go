package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// --- Decay commands ---

var decayCmd = &cobra.Command{
	Use:   "decay",
	Short: "Manage memory decay and archival",
}

var decayStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show decay statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		stats, err := store.GetDecayStats()
		if err != nil {
			return err
		}

		output(titleStyle.Render("Decay Statistics"))
		output()
		output("  " + dimStyle.Render("Total observations:") + "     " + itoa(stats.TotalObservations))
		output("  " + dimStyle.Render("Low importance (<0.3):") + "  " + dimStyle.Render(itoa(stats.LowImportance)))
		output("  " + dimStyle.Render("Archived:") + "               " + itoa(stats.ArchivedCount))
		output("  " + dimStyle.Render("Expired (past date):") + "    " + dimStyle.Render(itoa(stats.ExpiredCount)))
		output("  " + dimStyle.Render("Average importance:") + "     " + fmt.Sprintf("%.3f", stats.AvgImportance))

		return nil
	},
}

var decaySoftCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply soft decay to importance scores",
	Long:  "Reduces importance scores based on recency of access.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		threshold, _ := cmd.Flags().GetFloat64("threshold")

		start := time.Now()
		affected, err := store.ApplySoftDecay(threshold)
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Soft Decay Applied"))
		output()
		output("  " + dimStyle.Render("Affected:") + " " + successStyle.Render(itoa(affected)) + " observations")
		output("  " + dimStyle.Render("Time:") + "     " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var decayArchiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive old, low-importance memories",
	Long:  "Moves memories to archive table based on age and importance.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		days, _ := cmd.Flags().GetInt("days")
		minImportance, _ := cmd.Flags().GetFloat64("min-importance")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		cfg := store.GetDecayConfig()
		cfg.ArchiveAfterDays = days
		cfg.MinImportanceToKeep = minImportance

		if dryRun {
			// Show what would be archived
			stats, err := store.GetDecayStats()
			if err != nil {
				return err
			}
			output(titleStyle.Render("Archive Preview (Dry Run)"))
			output()
			output("  " + dimStyle.Render("Would archive approximately:") + " " + itoa(stats.LowImportance) + " observations")
			output("  " + dimStyle.Render("(Run without --dry-run to execute)"))
			return nil
		}

		start := time.Now()
		archived, err := store.ArchiveOldMemories(cfg)
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Archive Complete"))
		output()
		output("  " + dimStyle.Render("Archived:") + " " + successStyle.Render(itoa(archived)) + " observations")
		output("  " + dimStyle.Render("Time:") + "     " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var decayForgetCmd = &cobra.Command{
	Use:   "forget",
	Short: "Delete expired memories",
	Long:  "Deletes memories that have passed their forget_after date.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		expired, _ := cmd.Flags().GetBool("expired")
		archiveDays, _ := cmd.Flags().GetInt("archive-days")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		var deleted int

		if expired {
			if dryRun {
				stats, _ := store.GetDecayStats()
				output(titleStyle.Render("Forget Preview (Dry Run)"))
				output()
				output("  " + dimStyle.Render("Expired to delete:") + " " + itoa(stats.ExpiredCount))
				return nil
			}

			count, err := store.ForgetExpiredMemories()
			if err != nil {
				return err
			}
			deleted += count
		}

		if archiveDays > 0 {
			count, err := store.ForgetOldArchivedMemories(archiveDays)
			if err != nil {
				return err
			}
			deleted += count
		}

		output(titleStyle.Render("Forget Complete"))
		output()
		output("  " + dimStyle.Render("Deleted:") + " " + successStyle.Render(itoa(deleted)) + " memories")

		return nil
	},
}

func init() {
	decaySoftCmd.Flags().Float64("threshold", 0.3, "minimum importance to apply decay")

	decayArchiveCmd.Flags().Int("days", 90, "archive memories older than this")
	decayArchiveCmd.Flags().Float64("min-importance", 0.1, "archive below this importance")
	decayArchiveCmd.Flags().Bool("dry-run", false, "preview without executing")

	decayForgetCmd.Flags().Bool("expired", false, "delete memories past forget_after date")
	decayForgetCmd.Flags().Int("archive-days", 0, "delete archived memories older than this")
	decayForgetCmd.Flags().Bool("dry-run", false, "preview without executing")

	decayCmd.AddCommand(decayStatsCmd)
	decayCmd.AddCommand(decaySoftCmd)
	decayCmd.AddCommand(decayArchiveCmd)
	decayCmd.AddCommand(decayForgetCmd)
	rootCmd.AddCommand(decayCmd)
}

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Analytics command ---

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Show memory analytics dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		asJSON, _ := cmd.Flags().GetBool("json")
		topN, _ := cmd.Flags().GetInt("top")

		analytics, err := store.GetMemoryAnalytics(topN)
		if err != nil {
			return err
		}

		if asJSON {
			return printAnalyticsJSON(analytics)
		}

		printAnalyticsOverview(analytics)
		output()
		printDecayCurve(analytics)
		output()
		printTopMemories(analytics)
		output()
		printDormantAndFactTypes(analytics)
		output()
		printActivity(analytics)

		rec, err := store.RecommendTuning()
		if err != nil {
			return err
		}
		printDriftBadge(rec)

		return nil
	},
}

// printAnalyticsJSON marshals the analytics struct directly to stdout.
func printAnalyticsJSON(a *storage.MemoryAnalytics) error {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	output(string(data))
	return nil
}

// printAnalyticsOverview prints the top-level entity/observation/relation/
// session counts and embedding/DB-size summary line.
func printAnalyticsOverview(a *storage.MemoryAnalytics) {
	output(titleStyle.Render("Memory Analytics"))
	output(fmt.Sprintf("  Entities: %d   Observations: %d   Relations: %d   Sessions: %d",
		a.TotalEntities, a.TotalObservations, a.TotalRelations, a.TotalSessions))
	dbSizeMB := float64(a.DBSizeBytes) / (1024 * 1024)
	output(fmt.Sprintf("  Embedding coverage: %.0f%%   DB size: %.1f MB", a.EmbeddingCoverage, dbSizeMB))
}

// printDecayCurve prints one line per age bucket in the decay curve.
func printDecayCurve(a *storage.MemoryAnalytics) {
	output(titleStyle.Render("Decay Curve"))
	for _, bucket := range a.DecayCurve {
		output(fmt.Sprintf("  %-7s %d obs  avg importance %.2f", bucket.Label, bucket.Count, bucket.AvgImportance))
	}
}

// printTopMemories prints the top-accessed observations, truncating each
// content preview to 60 chars.
func printTopMemories(a *storage.MemoryAnalytics) {
	output(titleStyle.Render("Top Memories (by access)"))
	for _, obs := range a.TopAccessed {
		output(fmt.Sprintf("  [%d hits] %s — %s", obs.AccessCount, obs.EntityName, truncate(obs.Content, 60)))
	}
}

// printDormantAndFactTypes prints the never-accessed observation count and
// a fact-type breakdown (static/dynamic split out, session_* summed).
func printDormantAndFactTypes(a *storage.MemoryAnalytics) {
	output(fmt.Sprintf("Dormant: %d observations never accessed", a.NeverAccessed))

	sessionCount := 0
	for factType, count := range a.FactTypeCounts {
		if strings.HasPrefix(factType, "session_") {
			sessionCount += count
		}
	}
	output(fmt.Sprintf("Fact types: static %d · dynamic %d · session_* %d",
		a.FactTypeCounts["static"], a.FactTypeCounts["dynamic"], sessionCount))
}

// printActivity prints session counts per ISO week for the last 4 weeks.
func printActivity(a *storage.MemoryAnalytics) {
	output(titleStyle.Render("Activity (last 4 weeks)"))
	for _, week := range a.SessionsPerWeek {
		output(fmt.Sprintf("  %s  %d sessions", week.Week, week.Count))
	}
}

// printDriftBadge prints a warning badge when the tuning recommendation
// has drifted from the current config.
func printDriftBadge(rec *storage.TuningRecommendation) {
	if rec.Drift {
		output()
		output(dimStyle.Render("[!] Config drift detected — run 'mark42 analytics tune'"))
	}
}

// --- Analytics tune command ---

var analyticsTuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Show usage-driven tuning recommendations for importance config",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		rec, err := store.RecommendTuning()
		if err != nil {
			return err
		}

		printTuningRecommendation(rec)

		apply, _ := cmd.Flags().GetBool("apply")
		if apply {
			if err := store.SetImportanceConfig(rec.Suggested); err != nil {
				return err
			}
			output("Applied. New config saved.")
			return nil
		}

		output("No changes applied. Re-run with --apply to persist.")
		return nil
	},
}

// printTuningRecommendation prints the current vs. suggested importance
// config for each parameter that changed, paired with its rationale.
func printTuningRecommendation(rec *storage.TuningRecommendation) {
	output(titleStyle.Render("Tuning Recommendation"))
	if rec.Current.DecayConstant != rec.Suggested.DecayConstant {
		output(fmt.Sprintf("  DecayConstant    %.0f → %.0f   (%s)",
			rec.Current.DecayConstant, rec.Suggested.DecayConstant, findRationale(rec.Rationale, "decay constant")))
	}
	if rec.Current.FrequencyWeight != rec.Suggested.FrequencyWeight {
		output(fmt.Sprintf("  FrequencyWeight  %.1f → %.1f (%s)",
			rec.Current.FrequencyWeight, rec.Suggested.FrequencyWeight, findRationale(rec.Rationale, "frequency")))
	}
	if rec.Current.CentralityWeight != rec.Suggested.CentralityWeight {
		output(fmt.Sprintf("  CentralityWeight %.1f → %.1f (%s)",
			rec.Current.CentralityWeight, rec.Suggested.CentralityWeight, findRationale(rec.Rationale, "centrality")))
	}
	if rec.Current.RecencyWeight != rec.Suggested.RecencyWeight {
		output(fmt.Sprintf("  RecencyWeight    %.1f → %.1f",
			rec.Current.RecencyWeight, rec.Suggested.RecencyWeight))
	}
}

// findRationale returns the first entry in rationale that mentions
// keyword, or "" when none matches.
func findRationale(rationale []string, keyword string) string {
	for _, r := range rationale {
		if strings.Contains(r, keyword) {
			return r
		}
	}
	return ""
}

// truncate shortens s to at most n runes, appending "..." when truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func init() {
	analyticsCmd.Flags().Bool("json", false, "output as JSON")
	analyticsCmd.Flags().Int("top", 10, "number of top-accessed memories to show")
	analyticsTuneCmd.Flags().Bool("apply", false, "persist the suggested config")

	analyticsCmd.AddCommand(analyticsTuneCmd)
	rootCmd.AddCommand(analyticsCmd)
}

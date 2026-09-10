package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// --- Stats command ---

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show database statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		graph, err := store.ReadGraph()
		if err != nil {
			return err
		}

		obsCount := 0
		for _, e := range graph.Entities {
			obsCount += len(e.Observations)
		}

		output(titleStyle.Render("Database Statistics"))
		output()
		output("  " + dimStyle.Render("Path:") + "         " + dbPath)
		output("  " + dimStyle.Render("Entities:") + "     " + successStyle.Render(itoa(len(graph.Entities))))
		output("  " + dimStyle.Render("Observations:") + " " + successStyle.Render(itoa(obsCount)))
		output("  " + dimStyle.Render("Relations:") + "    " + successStyle.Render(itoa(len(graph.Relations))))

		if total, withEmb, err := store.EmbeddingStats(); err == nil {
			pct := 0.0
			if total > 0 {
				pct = float64(withEmb) / float64(total) * 100
			}
			indicator := ""
			if total > 0 && withEmb < total {
				indicator = " !"
			}
			output("  " + dimStyle.Render("Embeddings:") + "   " + successStyle.Render(fmt.Sprintf("%d/%d (%.1f%%)", withEmb, total, pct)) + indicator)
		}

		return nil
	},
}

// --- Version command ---

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		output(titleStyle.Render("mark42") + " " + dimStyle.Render(Version))
	},
}

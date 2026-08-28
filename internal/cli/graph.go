package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

// --- Graph command ---

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Output the entire knowledge graph",
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

		format, _ := cmd.Flags().GetString("format")

		switch format {
		case "dot":
			output("digraph memory {")
			output("  rankdir=LR;")
			for _, e := range graph.Entities {
				output("  \"" + e.Name + "\" [label=\"" + e.Name + "\\n(" + e.Type + ")\"];")
			}
			for _, r := range graph.Relations {
				output("  \"" + r.From + "\" -> \"" + r.To + "\" [label=\"" + r.Type + "\"];")
			}
			output("}")
		default:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(graph)
		}
		return nil
	},
}

func init() {
	graphCmd.Flags().String("format", "json", "output format: json, dot")
}

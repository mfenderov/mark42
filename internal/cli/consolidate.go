package cli

import (
	"github.com/spf13/cobra"
)

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Consolidate duplicate observations across all entities",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		entities, err := store.ListEntities("")
		if err != nil {
			return err
		}

		before := store.CountObservations()
		for _, e := range entities {
			if _, err := store.ConsolidateObservations(e.Name); err != nil {
				return err
			}
		}
		removed := before - store.CountObservations()

		output(successStyle.Render("✓") + " Consolidated " + itoa(removed) + " redundant observations across " + itoa(len(entities)) + " entities")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(consolidateCmd)
}

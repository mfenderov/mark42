package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Observation commands ---

var obsCmd = &cobra.Command{
	Use:   "obs",
	Short: "Manage observations",
}

var obsAddCmd = &cobra.Command{
	Use:   "add <entity> <content>",
	Short: "Add an observation to an entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.AddObservation(args[0], args[1]); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", args[0])
				os.Exit(1)
			}
			return err
		}

		logger.Info("Added observation", "entity", entityStyle.Render(args[0]))
		return nil
	},
}

var obsDeleteCmd = &cobra.Command{
	Use:   "delete <entity> <content>",
	Short: "Delete an observation from an entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.DeleteObservation(args[0], args[1]); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Observation not found")
				os.Exit(1)
			}
			return err
		}

		logger.Info("Deleted observation", "entity", args[0])
		return nil
	},
}

func init() {
	obsCmd.AddCommand(obsAddCmd)
	obsCmd.AddCommand(obsDeleteCmd)
}

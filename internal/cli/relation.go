package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Relation commands ---

var relCmd = &cobra.Command{
	Use:   "rel",
	Short: "Manage relations",
}

var relCreateCmd = &cobra.Command{
	Use:   "create <from> <to> <type>",
	Short: "Create a relation between entities",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.CreateRelation(args[0], args[1], args[2]); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("One or both entities not found")
				os.Exit(1)
			}
			return err
		}

		logger.Info("Created relation",
			"from", entityStyle.Render(args[0]),
			"type", relationStyle.Render(args[2]),
			"to", entityStyle.Render(args[1]))
		return nil
	},
}

var relListCmd = &cobra.Command{
	Use:   "list <entity>",
	Short: "List relations for an entity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		relations, err := store.ListRelations(args[0])
		if err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", args[0])
				os.Exit(1)
			}
			return err
		}

		if len(relations) == 0 {
			logger.Info("No relations found")
			return nil
		}

		for _, r := range relations {
			output(entityStyle.Render(r.From) + " " +
				relationStyle.Render("─["+r.Type+"]→") + " " +
				entityStyle.Render(r.To))
		}
		return nil
	},
}

var relDeleteCmd = &cobra.Command{
	Use:   "delete <from> <to> <type>",
	Short: "Delete a relation",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.DeleteRelation(args[0], args[1], args[2]); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Relation not found")
				os.Exit(1)
			}
			return err
		}

		logger.Info("Deleted relation",
			"from", args[0],
			"type", args[2],
			"to", args[1])
		return nil
	},
}

func init() {
	relCmd.AddCommand(relCreateCmd)
	relCmd.AddCommand(relListCmd)
	relCmd.AddCommand(relDeleteCmd)
}

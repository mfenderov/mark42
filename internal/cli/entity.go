package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Entity commands ---

var entityCmd = &cobra.Command{
	Use:   "entity",
	Short: "Manage entities",
}

var entityCreateCmd = &cobra.Command{
	Use:   "create <name> <type>",
	Short: "Create a new entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		obs, _ := cmd.Flags().GetStringSlice("obs")

		entity, err := store.CreateEntity(args[0], args[1], obs)
		if err != nil {
			return err
		}

		logger.Info("Created entity",
			"name", entityStyle.Render(entity.Name),
			"type", typeStyle.Render(entity.Type))
		return nil
	},
}

var entityGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get an entity by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		entity, err := store.GetEntity(args[0])
		if err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", args[0])
				os.Exit(1)
			}
			return err
		}

		printEntity(entity)
		return nil
	},
}

var entityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all entities",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		entityType, _ := cmd.Flags().GetString("type")
		entities, err := store.ListEntities(entityType)
		if err != nil {
			return err
		}

		if len(entities) == 0 {
			logger.Info("No entities found")
			return nil
		}

		for _, e := range entities {
			output(entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
		}
		return nil
	},
}

var entityDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an entity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.DeleteEntity(args[0]); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", args[0])
				os.Exit(1)
			}
			return err
		}

		logger.Info("Deleted entity", "name", args[0])
		return nil
	},
}

func init() {
	entityCreateCmd.Flags().StringSlice("obs", nil, "observations to add")
	entityListCmd.Flags().String("type", "", "filter by entity type")

	entityCmd.AddCommand(entityCreateCmd)
	entityCmd.AddCommand(entityGetCmd)
	entityCmd.AddCommand(entityListCmd)
	entityCmd.AddCommand(entityDeleteCmd)
}

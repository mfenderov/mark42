package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Working directory (container tag) commands ---

var workdirCmd = &cobra.Command{
	Use:   "workdir",
	Short: "Manage working directory (project) scoping",
	Long: `Manage working directory awareness for multi-project memory scoping.

Entities can be tagged with a container (project) identifier.
During search and context injection, entities matching the current
project receive a score boost (1.5x by default).`,
}

var workdirSetCmd = &cobra.Command{
	Use:   "set <entity> <container-tag>",
	Short: "Set the container tag for an entity",
	Long: `Set the container tag (project identifier) for an entity.

Example:
  mark42 workdir set "Go Conventions" "mark42"

This associates the entity with the specified project.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		entityName := args[0]
		containerTag := args[1]

		if err := store.SetContainerTag(entityName, containerTag); err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", entityName)
				os.Exit(1)
			}
			return err
		}

		logger.Info("Set container tag",
			"entity", entityStyle.Render(entityName),
			"tag", typeStyle.Render(containerTag))
		return nil
	},
}

var workdirGetCmd = &cobra.Command{
	Use:   "get <entity>",
	Short: "Get the container tag for an entity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		entityName := args[0]

		tag, err := store.GetContainerTag(entityName)
		if err != nil {
			if err == storage.ErrNotFound {
				logger.Error("Entity not found", "name", entityName)
				os.Exit(1)
			}
			return err
		}

		if tag == "" {
			logger.Info("No container tag set", "entity", entityName)
		} else {
			output(entityStyle.Render(entityName) + " " + dimStyle.Render("→") + " " + typeStyle.Render(tag))
		}
		return nil
	},
}

var workdirListCmd = &cobra.Command{
	Use:   "list <container-tag>",
	Short: "List all entities with a specific container tag",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		containerTag := args[0]

		entities, err := store.GetEntitiesByContainerTag(containerTag)
		if err != nil {
			return err
		}

		if len(entities) == 0 {
			logger.Info("No entities found with tag", "tag", containerTag)
			return nil
		}

		output(titleStyle.Render("Entities in " + containerTag))
		output()
		for _, e := range entities {
			output("  " + entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
		}
		return nil
	},
}

var workdirSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search with container tag boosting",
	Long: `Search with working directory awareness.

Entities matching the specified container tag receive a 1.5x score boost.
This helps surface project-specific memories first.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		limit, _ := cmd.Flags().GetInt("limit")
		containerTag, _ := cmd.Flags().GetString("tag")
		boost, _ := cmd.Flags().GetFloat64("boost")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		results, err := store.HybridSearchWithBoost(ctx, args[0], nil, limit, containerTag, boost)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No results found", "query", args[0])
			return nil
		}

		output(titleStyle.Render("Search Results") + " " + dimStyle.Render("(boosted: "+containerTag+")"))
		output()
		for _, r := range results {
			score := fmt.Sprintf("%.4f", r.FusionScore)
			output(entityStyle.Render(r.EntityName) + " " +
				typeStyle.Render("("+r.EntityType+")") + " " +
				dimStyle.Render("["+score+"]"))
			output("  " + obsStyle.Render(r.Content))
			output()
		}
		return nil
	},
}

func init() {
	workdirSearchCmd.Flags().Int("limit", 10, "maximum number of results")
	workdirSearchCmd.Flags().String("tag", "", "container tag to boost (required)")
	workdirSearchCmd.Flags().Float64("boost", 1.5, "score multiplier for matching entities")

	workdirCmd.AddCommand(workdirSetCmd)
	workdirCmd.AddCommand(workdirGetCmd)
	workdirCmd.AddCommand(workdirListCmd)
	workdirCmd.AddCommand(workdirSearchCmd)
	rootCmd.AddCommand(workdirCmd)
}

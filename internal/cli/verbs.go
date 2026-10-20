package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

var rememberCmd = &cobra.Command{
	Use:   "remember <topic> <facts...>",
	Short: "Store or update knowledge in memory under a topic",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if store.Embedder() == nil {
			if emb := tryCLIEmbedder(); emb != nil {
				store.WithEmbedder(emb)
			}
		}

		topic := args[0]
		facts := args[1:]

		entityType, _ := cmd.Flags().GetString("type")
		factTypeStr, _ := cmd.Flags().GetString("fact-type")
		project, _ := cmd.Flags().GetString("project")
		rels, _ := cmd.Flags().GetStringSlice("rel")

		var relations []storage.RelationParam
		for _, relStr := range rels {
			parts := strings.SplitN(relStr, ":", 2)
			if len(parts) == 2 {
				relations = append(relations, storage.RelationParam{
					To:   strings.TrimSpace(parts[0]),
					Type: strings.TrimSpace(parts[1]),
				})
			}
		}

		factType := storage.FactTypeStatic
		if factTypeStr != "" {
			factType = storage.FactType(factTypeStr)
		}

		res, err := store.Remember(context.Background(), storage.RememberParams{
			Topic:      topic,
			Facts:      facts,
			EntityType: entityType,
			FactType:   factType,
			Relations:  relations,
			Project:    project,
		})
		if err != nil {
			return err
		}

		output(successStyle.Render("✓ Remembered") + " " + itoa(res.ObservationsAdded) + " fact(s) under " + entityStyle.Render(res.Topic))
		if res.RelationsCreated > 0 {
			output("  " + dimStyle.Render(fmt.Sprintf("Created %d relation(s)", res.RelationsCreated)))
		}
		return nil
	},
}

var recallCmd = &cobra.Command{
	Use:   "recall [query]",
	Short: "Retrieve memories by query, topic, or project context",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if store.Embedder() == nil {
			if emb := tryCLIEmbedder(); emb != nil {
				store.WithEmbedder(emb)
			}
		}

		topic, _ := cmd.Flags().GetString("topic")
		project, _ := cmd.Flags().GetString("project")
		limit, _ := cmd.Flags().GetInt("limit")

		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		res, err := store.Recall(context.Background(), storage.RecallParams{
			Query:   query,
			Topic:   topic,
			Project: project,
			Limit:   limit,
		})
		if err != nil {
			return err
		}

		switch res.Type {
		case "topic":
			if res.Topic == nil {
				output("Topic not found: " + entityStyle.Render(topic))
				return nil
			}
			output(titleStyle.Render("Topic: ") + entityStyle.Render(res.Topic.Name) + " " + typeStyle.Render("("+res.Topic.Type+")"))
			if len(res.Topic.Observations) == 0 {
				output("  " + dimStyle.Render("(no active facts)"))
			} else {
				for _, obs := range res.Topic.Observations {
					output("  " + dimStyle.Render("•") + " " + obsStyle.Render(obs))
				}
			}
			if len(res.Relations) > 0 {
				output()
				output(dimStyle.Render("Relations:"))
				for _, rel := range res.Relations {
					output("  " + dimStyle.Render("→") + " " + relationStyle.Render(rel.Type) + " " + entityStyle.Render(rel.To))
				}
			}

		case "search":
			if len(res.SearchResults) == 0 {
				output("No matching memories found for " + entityStyle.Render(query))
				return nil
			}
			output(titleStyle.Render("Search Results for ") + entityStyle.Render(query))
			output()
			for _, item := range res.SearchResults {
				output(entityStyle.Render(item.EntityName) + " " + typeStyle.Render("("+item.EntityType+")"))
				output("  " + obsStyle.Render(item.Content))
				output()
			}

		case "context":
			output(titleStyle.Render("Project Context"))
			if len(res.Context) == 0 && len(res.Sessions) == 0 {
				output("  " + dimStyle.Render("(no context found)"))
				return nil
			}
			if len(res.Context) > 0 {
				output()
				output(dimStyle.Render("Core Rules & Conventions:"))
				for _, c := range res.Context {
					output("  " + entityStyle.Render("["+c.EntityName+"]") + " " + obsStyle.Render(c.Content))
				}
			}
			if len(res.Sessions) > 0 {
				output()
				output(dimStyle.Render("Recent Session Summaries:"))
				for _, s := range res.Sessions {
					output("  " + entityStyle.Render("["+s.EntityName+"]") + " " + obsStyle.Render(s.Content))
				}
			}
		}

		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget <topic> [fact]",
	Short: "Invalidate or delete knowledge from memory",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		topic := args[0]
		fact := ""
		if len(args) > 1 {
			fact = args[1]
		}

		permanent, _ := cmd.Flags().GetBool("permanent")

		res, err := store.Forget(context.Background(), storage.ForgetParams{
			Topic:     topic,
			Fact:      fact,
			Permanent: permanent,
		})
		if err != nil {
			return err
		}

		action := "Invalidated"
		if res.Permanent {
			action = "Permanently deleted"
		}

		if fact != "" {
			output(successStyle.Render("✓ "+action) + " fact from " + entityStyle.Render(res.Topic))
		} else {
			output(successStyle.Render("✓ "+action) + " topic " + entityStyle.Render(res.Topic))
		}
		return nil
	},
}

func tryCLIEmbedder() storage.Embedder {
	url := os.Getenv("CLAUDE_MEMORY_EMBEDDER_URL")
	if url == "disabled" {
		return nil
	}
	if url == "" {
		url = storage.DefaultOllamaBaseURL()
	}
	client := storage.NewEmbeddingClient(url)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := client.CreateEmbedding(ctx, "test"); err != nil {
		return nil
	}
	return client
}

func init() {
	rememberCmd.Flags().String("type", "concept", "entity category or type (person, preference, decision, concept)")
	rememberCmd.Flags().String("fact-type", "static", "fact type: static (durable/evergreen), dynamic (active state), session (summary)")
	rememberCmd.Flags().String("project", "", "namespace or container tag")
	rememberCmd.Flags().StringSlice("rel", nil, "relation to other topic in target:type format")

	recallCmd.Flags().String("topic", "", "specific topic, person, or entity to inspect")
	recallCmd.Flags().String("project", "", "namespace or container to scope context")
	recallCmd.Flags().Int("limit", 10, "maximum search results")

	forgetCmd.Flags().Bool("permanent", false, "permanently delete from database instead of soft-invalidating")

	rootCmd.AddCommand(rememberCmd)
	rootCmd.AddCommand(recallCmd)
	rootCmd.AddCommand(forgetCmd)
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Search command ---

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search entities by keyword",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")

		results, err := store.SearchWithLimit(args[0], limit)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No results found", "query", args[0])
			return nil
		}

		switch format {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		case "context":
			// Format optimized for Claude context injection
			for _, r := range results {
				output("## " + entityStyle.Render(r.Name) + " " + typeStyle.Render("("+r.Type+")"))
				for _, obs := range r.Observations {
					output("- " + obs)
				}
				output()
			}
		default:
			for _, r := range results {
				printEntity(r.Entity)
				output()
			}
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().Int("limit", 10, "maximum number of results")
	searchCmd.Flags().String("format", "default", "output format: default, json, context")
}

// --- Hybrid Search command ---

var hybridSearchCmd = &cobra.Command{
	Use:   "hybrid-search <query>",
	Short: "Search using FTS5 + vector semantic search",
	Long: `Search entities using hybrid FTS5 + vector semantic search.

Combines keyword matching (FTS5 BM25) with semantic similarity (embeddings)
using Reciprocal Rank Fusion (RRF) for best results.

Requires Ollama to be running with an embedding model for vector search.
Falls back to FTS-only search if Ollama is unavailable.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations to ensure embedding table exists
		if err := store.Migrate(); err != nil {
			return err
		}

		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")
		model, _ := cmd.Flags().GetString("model")
		url, _ := cmd.Flags().GetString("url")

		// Create embedding client
		client := storage.NewEmbeddingClient(url)
		client.SetModel(model)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		results, err := store.HybridSearchWithEmbedder(ctx, args[0], client, limit)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No results found", "query", args[0])
			return nil
		}

		switch format {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		case "context":
			// Group results by entity for context output
			entityMap := make(map[string]struct {
				Type         string
				Observations []string
				MaxScore     float64
			})
			for _, r := range results {
				if e, ok := entityMap[r.EntityName]; ok {
					e.Observations = append(e.Observations, r.Content)
					if r.FusionScore > e.MaxScore {
						e.MaxScore = r.FusionScore
					}
					entityMap[r.EntityName] = e
				} else {
					entityMap[r.EntityName] = struct {
						Type         string
						Observations []string
						MaxScore     float64
					}{
						Type:         r.EntityType,
						Observations: []string{r.Content},
						MaxScore:     r.FusionScore,
					}
				}
			}
			for name, e := range entityMap {
				output("## " + entityStyle.Render(name) + " " + typeStyle.Render("("+e.Type+")"))
				for _, obs := range e.Observations {
					output("- " + obs)
				}
				output()
			}
		default:
			// Default: show results with scores
			output(titleStyle.Render("Hybrid Search Results"))
			output()
			for _, r := range results {
				score := fmt.Sprintf("%.4f", r.FusionScore)
				// Build sources list from SourceScores map
				var sources []string
				for source := range r.SourceScores {
					sources = append(sources, source)
				}
				sourcesStr := strings.Join(sources, ", ")
				output(entityStyle.Render(r.EntityName) + " " +
					typeStyle.Render("("+r.EntityType+")") + " " +
					dimStyle.Render("["+score+"] ["+sourcesStr+"]"))
				output("  " + obsStyle.Render(r.Content))
				output()
			}
		}
		return nil
	},
}

func init() {
	defaultOllamaURL := storage.DefaultOllamaBaseURL()

	hybridSearchCmd.Flags().Int("limit", 10, "maximum number of results")
	hybridSearchCmd.Flags().String("format", "default", "output format: default, json, context")
	hybridSearchCmd.Flags().String("model", "nomic-embed-text", "embedding model for vector search")
	hybridSearchCmd.Flags().String("url", defaultOllamaURL, "Ollama API URL")

	rootCmd.AddCommand(hybridSearchCmd)
}

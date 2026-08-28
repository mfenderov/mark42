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

// --- Embed commands ---

var (
	ollamaURL  string
	embedModel string
	embedBatch int
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Manage embeddings for semantic search",
}

var embedTestCmd = &cobra.Command{
	Use:   "test [text]",
	Short: "Test Ollama embedding generation",
	Long: `Test that Ollama is running and can generate embeddings.

If no text is provided, uses "Hello, world!" as test input.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		text := "Hello, world!"
		if len(args) > 0 {
			text = strings.Join(args, " ")
		}

		client := storage.NewEmbeddingClient(ollamaURL)
		client.SetModel(embedModel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		start := time.Now()
		embedding, err := client.CreateEmbedding(ctx, text)
		elapsed := time.Since(start)

		if err != nil {
			logger.Error("Embedding failed - is Ollama running?",
				"url", ollamaURL,
				"error", err)
			output()
			output(dimStyle.Render("To start Ollama:"))
			output("  ollama serve")
			output()
			output(dimStyle.Render("To pull the embedding model:"))
			output("  ollama pull " + embedModel)
			os.Exit(1)
		}

		output(titleStyle.Render("Embedding Test"))
		output()
		output("  " + dimStyle.Render("URL:") + "        " + ollamaURL)
		output("  " + dimStyle.Render("Model:") + "      " + embedModel)
		output("  " + dimStyle.Render("Input:") + "      " + text)
		output("  " + dimStyle.Render("Dimensions:") + " " + successStyle.Render(itoa(len(embedding))))
		output("  " + dimStyle.Render("Time:") + "       " + successStyle.Render(elapsed.String()))
		output()
		output(successStyle.Render("✓ Ollama is working!"))

		return nil
	},
}

var embedGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate embeddings for all observations",
	Long:  "Generates embeddings for observations that don't have them yet.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations first
		if err := store.Migrate(); err != nil {
			return err
		}

		// Get observations without embeddings
		observations, err := store.GetObservationsWithoutEmbeddings()
		if err != nil {
			return err
		}

		if len(observations) == 0 {
			output(successStyle.Render("✓ All observations have embeddings"))
			return nil
		}

		output(titleStyle.Render("Generating Embeddings"))
		output()
		output("  " + dimStyle.Render("Observations:") + " " + itoa(len(observations)))
		output("  " + dimStyle.Render("Model:") + "        " + embedModel)
		output("  " + dimStyle.Render("Batch size:") + "   " + itoa(embedBatch))
		output()

		client := storage.NewEmbeddingClient(ollamaURL)
		client.SetModel(embedModel)

		ctx := context.Background()
		start := time.Now()
		processed := 0

		// Process in batches
		for i := 0; i < len(observations); i += embedBatch {
			end := i + embedBatch
			if end > len(observations) {
				end = len(observations)
			}

			batch := observations[i:end]
			texts := make([]string, len(batch))
			for j, obs := range batch {
				texts[j] = obs.Content
			}

			embeddings, err := client.CreateBatchEmbedding(ctx, texts)
			if err != nil {
				logger.Error("Batch embedding failed",
					"batch", i/embedBatch+1,
					"error", err)
				continue
			}

			if err := store.BatchStoreEmbeddings(batch, embeddings, embedModel); err != nil {
				logger.Error("Failed to store embeddings", "error", err)
				continue
			}

			processed += len(batch)
			progress := float64(processed) / float64(len(observations)) * 100
			fmt.Printf("\r  Progress: %s%.1f%% (%d/%d)%s",
				successStyle.Render(""), progress, processed, len(observations),
				strings.Repeat(" ", 10))
		}

		elapsed := time.Since(start)
		output()
		output()
		output("  " + dimStyle.Render("Processed:") + " " + successStyle.Render(itoa(processed)))
		output("  " + dimStyle.Render("Time:") + "      " + successStyle.Render(elapsed.String()))
		output()
		output(successStyle.Render("✓ Embeddings generated"))

		return nil
	},
}

var embedStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show embedding statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		total, withEmbeddings, err := store.EmbeddingStats()
		if err != nil {
			return err
		}

		coverage := 0.0
		if total > 0 {
			coverage = float64(withEmbeddings) / float64(total) * 100
		}

		output(titleStyle.Render("Embedding Statistics"))
		output()
		output("  " + dimStyle.Render("Total observations:") + "     " + itoa(total))
		output("  " + dimStyle.Render("With embeddings:") + "        " + successStyle.Render(itoa(withEmbeddings)))
		output("  " + dimStyle.Render("Without embeddings:") + "     " + itoa(total-withEmbeddings))
		output("  " + dimStyle.Render("Coverage:") + "               " + successStyle.Render(fmt.Sprintf("%.1f%%", coverage)))

		return nil
	},
}

func init() {
	defaultOllamaURL := storage.DefaultOllamaBaseURL()

	embedCmd.PersistentFlags().StringVar(&ollamaURL, "url", defaultOllamaURL, "Ollama API URL")
	embedCmd.PersistentFlags().StringVar(&embedModel, "model", "nomic-embed-text", "embedding model name")
	embedGenerateCmd.Flags().IntVar(&embedBatch, "batch", 10, "batch size for embedding generation")

	embedCmd.AddCommand(embedTestCmd)
	embedCmd.AddCommand(embedGenerateCmd)
	embedCmd.AddCommand(embedStatsCmd)
	rootCmd.AddCommand(embedCmd)
}

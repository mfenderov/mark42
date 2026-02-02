package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/mfenderov/claude-memory/internal/storage"
	"github.com/spf13/cobra"
)

var (
	dbPath  string
	Version = "dev"
	logger  = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	entityStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	typeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	obsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	relationStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("219"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "claude-memory",
	Short: "Local memory system for Claude Code",
	Long: titleStyle.Render("claude-memory") + " - A privacy-first, SQLite-based memory system\n\n" +
		"Store entities, observations, and relations in a local database\n" +
		"with full-text search capabilities.",
}

func init() {
	defaultDB := filepath.Join(os.Getenv("HOME"), ".claude", "memory.db")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "path to database file")

	rootCmd.AddCommand(entityCmd)
	rootCmd.AddCommand(obsCmd)
	rootCmd.AddCommand(relCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(versionCmd)
}

func getStore() (*storage.Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return storage.NewStore(dbPath)
}

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
			println(entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
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
			println(entityStyle.Render(r.From) + " " +
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
				println("## " + entityStyle.Render(r.Name) + " " + typeStyle.Render("("+r.Type+")"))
				for _, obs := range r.Observations {
					println("- " + obs)
				}
				println()
			}
		default:
			for _, r := range results {
				printEntity(r.Entity)
				println()
			}
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().Int("limit", 10, "maximum number of results")
	searchCmd.Flags().String("format", "default", "output format: default, json, context")
}

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
			println("digraph memory {")
			println("  rankdir=LR;")
			for _, e := range graph.Entities {
				println("  \"" + e.Name + "\" [label=\"" + e.Name + "\\n(" + e.Type + ")\"];")
			}
			for _, r := range graph.Relations {
				println("  \"" + r.From + "\" -> \"" + r.To + "\" [label=\"" + r.Type + "\"];")
			}
			println("}")
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

// --- Init command ---

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		logger.Info("Database initialized", "path", dimStyle.Render(dbPath))
		return nil
	},
}

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

		println(titleStyle.Render("Database Statistics"))
		println()
		println("  " + dimStyle.Render("Path:") + "         " + dbPath)
		println("  " + dimStyle.Render("Entities:") + "     " + successStyle.Render(itoa(len(graph.Entities))))
		println("  " + dimStyle.Render("Observations:") + " " + successStyle.Render(itoa(obsCount)))
		println("  " + dimStyle.Render("Relations:") + "    " + successStyle.Render(itoa(len(graph.Relations))))

		return nil
	},
}

// --- Version command ---

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		println(titleStyle.Render("claude-memory") + " " + dimStyle.Render(Version))
	},
}

// --- Migrate command ---

type jsonMemory struct {
	Entities  []jsonEntity   `json:"entities"`
	Relations []jsonRelation `json:"relations"`
}

type jsonEntity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

type jsonRelation struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
}

// NDJSON format (Docker MCP style)
type ndjsonRecord struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
	From         string   `json:"from"`
	To           string   `json:"to"`
	RelationType string   `json:"relationType"`
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Import from JSON Memory MCP format",
	Long: `Import from JSON Memory MCP format.

Supports two formats:
  - Single JSON object with "entities" and "relations" arrays
  - NDJSON (newline-delimited JSON) with {"type":"entity",...} or {"type":"relation",...}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fromPath, _ := cmd.Flags().GetString("from")
		if fromPath == "" {
			logger.Error("--from flag is required")
			os.Exit(1)
		}

		data, err := os.ReadFile(fromPath)
		if err != nil {
			return err
		}

		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		var entities []jsonEntity
		var relations []jsonRelation

		// Try single JSON format first
		var memory jsonMemory
		if err := json.Unmarshal(data, &memory); err == nil && (len(memory.Entities) > 0 || len(memory.Relations) > 0) {
			entities = memory.Entities
			relations = memory.Relations
		} else {
			// Try NDJSON format (Docker MCP style)
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				var record ndjsonRecord
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					logger.Warn("Skipping invalid line", "error", err)
					continue
				}

				switch record.Type {
				case "entity":
					entities = append(entities, jsonEntity{
						Name:         record.Name,
						EntityType:   record.EntityType,
						Observations: record.Observations,
					})
				case "relation":
					relations = append(relations, jsonRelation{
						From:         record.From,
						To:           record.To,
						RelationType: record.RelationType,
					})
				default:
					logger.Warn("Unknown record type", "type", record.Type)
				}
			}
		}

		entityCount := 0
		obsCount := 0
		for _, e := range entities {
			_, err := store.CreateEntity(e.Name, e.EntityType, e.Observations)
			if err != nil {
				for _, obs := range e.Observations {
					if err := store.AddObservation(e.Name, obs); err == nil {
						obsCount++
					}
				}
			} else {
				entityCount++
				obsCount += len(e.Observations)
			}
		}

		relCount := 0
		for _, r := range relations {
			if err := store.CreateRelation(r.From, r.To, r.RelationType); err == nil {
				relCount++
			}
		}

		println(titleStyle.Render("Migration Complete"))
		println()
		println("  " + dimStyle.Render("Entities:") + "     " + successStyle.Render(itoa(entityCount)))
		println("  " + dimStyle.Render("Observations:") + " " + successStyle.Render(itoa(obsCount)))
		println("  " + dimStyle.Render("Relations:") + "    " + successStyle.Render(itoa(relCount)))

		return nil
	},
}

func init() {
	migrateCmd.Flags().String("from", "", "path to JSON Memory MCP file")
	rootCmd.AddCommand(migrateCmd)
}

// --- Upgrade command (schema migrations) ---

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Run database schema migrations",
	Long:  "Applies pending schema migrations to upgrade the database to the latest version.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		beforeVersion, err := store.GetSchemaVersion()
		if err != nil {
			return err
		}

		if err := store.Migrate(); err != nil {
			return err
		}

		afterVersion, err := store.GetSchemaVersion()
		if err != nil {
			return err
		}

		println(titleStyle.Render("Schema Upgrade"))
		println()
		if beforeVersion == afterVersion {
			println("  " + dimStyle.Render("Status:") + "  " + successStyle.Render("Already up to date"))
		} else {
			println("  " + dimStyle.Render("Before:") + "  Version " + fmt.Sprintf("%d", beforeVersion))
			println("  " + dimStyle.Render("After:") + "   Version " + successStyle.Render(fmt.Sprintf("%d", afterVersion)))
		}
		println("  " + dimStyle.Render("Path:") + "    " + dbPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

// --- Embed commands ---

var (
	ollamaURL   string
	embedModel  string
	embedBatch  int
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
			println()
			println(dimStyle.Render("To start Ollama:"))
			println("  ollama serve")
			println()
			println(dimStyle.Render("To pull the embedding model:"))
			println("  ollama pull " + embedModel)
			os.Exit(1)
		}

		println(titleStyle.Render("Embedding Test"))
		println()
		println("  " + dimStyle.Render("URL:") + "        " + ollamaURL)
		println("  " + dimStyle.Render("Model:") + "      " + embedModel)
		println("  " + dimStyle.Render("Input:") + "      " + text)
		println("  " + dimStyle.Render("Dimensions:") + " " + successStyle.Render(itoa(len(embedding))))
		println("  " + dimStyle.Render("Time:") + "       " + successStyle.Render(elapsed.String()))
		println()
		println(successStyle.Render("✓ Ollama is working!"))

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
			println(successStyle.Render("✓ All observations have embeddings"))
			return nil
		}

		println(titleStyle.Render("Generating Embeddings"))
		println()
		println("  " + dimStyle.Render("Observations:") + " " + itoa(len(observations)))
		println("  " + dimStyle.Render("Model:") + "        " + embedModel)
		println("  " + dimStyle.Render("Batch size:") + "   " + itoa(embedBatch))
		println()

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
		println()
		println()
		println("  " + dimStyle.Render("Processed:") + " " + successStyle.Render(itoa(processed)))
		println("  " + dimStyle.Render("Time:") + "      " + successStyle.Render(elapsed.String()))
		println()
		println(successStyle.Render("✓ Embeddings generated"))

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

		println(titleStyle.Render("Embedding Statistics"))
		println()
		println("  " + dimStyle.Render("Total observations:") + "     " + itoa(total))
		println("  " + dimStyle.Render("With embeddings:") + "        " + successStyle.Render(itoa(withEmbeddings)))
		println("  " + dimStyle.Render("Without embeddings:") + "     " + itoa(total-withEmbeddings))
		println("  " + dimStyle.Render("Coverage:") + "               " + successStyle.Render(fmt.Sprintf("%.1f%%", coverage)))

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

// --- Helpers ---

func printEntity(e *storage.Entity) {
	println(entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
	if len(e.Observations) > 0 {
		for _, obs := range e.Observations {
			println("  " + dimStyle.Render("•") + " " + obsStyle.Render(obs))
		}
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}


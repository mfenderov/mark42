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
	"github.com/spf13/cobra"

	"github.com/mfenderov/claude-memory/internal/storage"
)

var (
	dbPath  string
	Version = "dev"

	// logger writes operational messages (errors, info) to stderr
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})

	// out is the destination for command output (search results, stats, etc.)
	out = os.Stdout
)

// output writes command results to stdout (not stderr).
// This follows Unix conventions: data to stdout, logs to stderr.
func output(a ...any) {
	fmt.Fprintln(out, a...)
}

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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

		output(titleStyle.Render("Database Statistics"))
		output()
		output("  " + dimStyle.Render("Path:") + "         " + dbPath)
		output("  " + dimStyle.Render("Entities:") + "     " + successStyle.Render(itoa(len(graph.Entities))))
		output("  " + dimStyle.Render("Observations:") + " " + successStyle.Render(itoa(obsCount)))
		output("  " + dimStyle.Render("Relations:") + "    " + successStyle.Render(itoa(len(graph.Relations))))

		return nil
	},
}

// --- Version command ---

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		output(titleStyle.Render("claude-memory") + " " + dimStyle.Render(Version))
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

		output(titleStyle.Render("Migration Complete"))
		output()
		output("  " + dimStyle.Render("Entities:") + "     " + successStyle.Render(itoa(entityCount)))
		output("  " + dimStyle.Render("Observations:") + " " + successStyle.Render(itoa(obsCount)))
		output("  " + dimStyle.Render("Relations:") + "    " + successStyle.Render(itoa(relCount)))

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

		output(titleStyle.Render("Schema Upgrade"))
		output()
		if beforeVersion == afterVersion {
			output("  " + dimStyle.Render("Status:") + "  " + successStyle.Render("Already up to date"))
		} else {
			output("  " + dimStyle.Render("Before:") + "  Version " + fmt.Sprintf("%d", beforeVersion))
			output("  " + dimStyle.Render("After:") + "   Version " + successStyle.Render(fmt.Sprintf("%d", afterVersion)))
		}
		output("  " + dimStyle.Render("Path:") + "    " + dbPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

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

// --- Importance commands ---

var importanceCmd = &cobra.Command{
	Use:   "importance",
	Short: "Manage memory importance scores",
}

var importanceRecalculateCmd = &cobra.Command{
	Use:   "recalculate",
	Short: "Recalculate importance scores for all memories",
	Long: `Recalculate importance scores based on:
- Recency (how recently accessed)
- Centrality (how connected via relations)
- Fact type (static facts get bonus)

This helps prioritize which memories to include in context injection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations to ensure schema is up to date
		if err := store.Migrate(); err != nil {
			return err
		}

		start := time.Now()
		updated, err := store.RecalculateImportance()
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Importance Recalculation"))
		output()
		output("  " + dimStyle.Render("Updated:") + " " + successStyle.Render(itoa(updated)) + " observations")
		output("  " + dimStyle.Render("Time:") + "    " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var importanceStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show importance score statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Get importance distribution
		type stats struct {
			Total     int     `db:"total"`
			AvgScore  float64 `db:"avg_score"`
			MinScore  float64 `db:"min_score"`
			MaxScore  float64 `db:"max_score"`
			HighCount int     `db:"high_count"`
			LowCount  int     `db:"low_count"`
		}
		var s stats
		err = store.DB().Get(&s, `
			SELECT
				COUNT(*) as total,
				COALESCE(AVG(importance), 0) as avg_score,
				COALESCE(MIN(importance), 0) as min_score,
				COALESCE(MAX(importance), 0) as max_score,
				SUM(CASE WHEN importance >= 0.7 THEN 1 ELSE 0 END) as high_count,
				SUM(CASE WHEN importance < 0.3 THEN 1 ELSE 0 END) as low_count
			FROM observations o
			JOIN entities e ON e.id = o.entity_id
			WHERE e.is_latest = 1
		`)
		if err != nil {
			return err
		}

		output(titleStyle.Render("Importance Statistics"))
		output()
		output("  " + dimStyle.Render("Total observations:") + " " + itoa(s.Total))
		output("  " + dimStyle.Render("Average score:") + "      " + fmt.Sprintf("%.3f", s.AvgScore))
		output("  " + dimStyle.Render("Min score:") + "          " + fmt.Sprintf("%.3f", s.MinScore))
		output("  " + dimStyle.Render("Max score:") + "          " + fmt.Sprintf("%.3f", s.MaxScore))
		output()
		output("  " + dimStyle.Render("High importance (≥0.7):") + " " + successStyle.Render(itoa(s.HighCount)))
		output("  " + dimStyle.Render("Low importance (<0.3):") + "  " + dimStyle.Render(itoa(s.LowCount)))

		return nil
	},
}

func init() {
	importanceCmd.AddCommand(importanceRecalculateCmd)
	importanceCmd.AddCommand(importanceStatsCmd)
	rootCmd.AddCommand(importanceCmd)
}

// --- Context command ---

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Get memories optimized for context injection",
	Long: `Get memories optimized for context injection at session start.

Orders by fact type (static > dynamic > session_turn), then by importance.
Respects token budget to avoid context overflow.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Run migrations
		if err := store.Migrate(); err != nil {
			return err
		}

		tokenBudget, _ := cmd.Flags().GetInt("token-budget")
		minImportance, _ := cmd.Flags().GetFloat64("min-importance")
		projectName, _ := cmd.Flags().GetString("project")

		cfg := storage.DefaultContextConfig()
		if tokenBudget > 0 {
			cfg.TokenBudget = tokenBudget
		}
		if minImportance > 0 {
			cfg.MinImportance = minImportance
		}

		results, err := store.GetContextForInjection(cfg, projectName)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			logger.Info("No relevant memories found")
			return nil
		}

		formatted := storage.FormatContextResults(results)
		estimatedTokens := storage.EstimateTokens(formatted)

		output(titleStyle.Render("Context for Injection"))
		output(dimStyle.Render(fmt.Sprintf("[%d estimated tokens, %d memories]", estimatedTokens, len(results))))
		output()
		print(formatted)

		return nil
	},
}

func init() {
	contextCmd.Flags().Int("token-budget", 2000, "maximum tokens to include")
	contextCmd.Flags().Float64("min-importance", 0.3, "minimum importance score (0-1)")
	contextCmd.Flags().String("project", "", "project name for boosting relevant memories")

	rootCmd.AddCommand(contextCmd)
}

// --- Decay commands ---

var decayCmd = &cobra.Command{
	Use:   "decay",
	Short: "Manage memory decay and archival",
}

var decayStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show decay statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		stats, err := store.GetDecayStats()
		if err != nil {
			return err
		}

		output(titleStyle.Render("Decay Statistics"))
		output()
		output("  " + dimStyle.Render("Total observations:") + "     " + itoa(stats.TotalObservations))
		output("  " + dimStyle.Render("Low importance (<0.3):") + "  " + dimStyle.Render(itoa(stats.LowImportance)))
		output("  " + dimStyle.Render("Archived:") + "               " + itoa(stats.ArchivedCount))
		output("  " + dimStyle.Render("Expired (past date):") + "    " + dimStyle.Render(itoa(stats.ExpiredCount)))
		output("  " + dimStyle.Render("Average importance:") + "     " + fmt.Sprintf("%.3f", stats.AvgImportance))

		return nil
	},
}

var decaySoftCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply soft decay to importance scores",
	Long:  "Reduces importance scores based on recency of access.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		threshold, _ := cmd.Flags().GetFloat64("threshold")

		start := time.Now()
		affected, err := store.ApplySoftDecay(threshold)
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Soft Decay Applied"))
		output()
		output("  " + dimStyle.Render("Affected:") + " " + successStyle.Render(itoa(affected)) + " observations")
		output("  " + dimStyle.Render("Time:") + "     " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var decayArchiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive old, low-importance memories",
	Long:  "Moves memories to archive table based on age and importance.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		days, _ := cmd.Flags().GetInt("days")
		minImportance, _ := cmd.Flags().GetFloat64("min-importance")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		cfg := storage.DefaultDecayConfig()
		cfg.ArchiveAfterDays = days
		cfg.MinImportanceToKeep = minImportance

		if dryRun {
			// Show what would be archived
			stats, err := store.GetDecayStats()
			if err != nil {
				return err
			}
			output(titleStyle.Render("Archive Preview (Dry Run)"))
			output()
			output("  " + dimStyle.Render("Would archive approximately:") + " " + itoa(stats.LowImportance) + " observations")
			output("  " + dimStyle.Render("(Run without --dry-run to execute)"))
			return nil
		}

		start := time.Now()
		archived, err := store.ArchiveOldMemories(cfg)
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		output(titleStyle.Render("Archive Complete"))
		output()
		output("  " + dimStyle.Render("Archived:") + " " + successStyle.Render(itoa(archived)) + " observations")
		output("  " + dimStyle.Render("Time:") + "     " + successStyle.Render(elapsed.String()))

		return nil
	},
}

var decayForgetCmd = &cobra.Command{
	Use:   "forget",
	Short: "Delete expired memories",
	Long:  "Deletes memories that have passed their forget_after date.",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := getStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(); err != nil {
			return err
		}

		expired, _ := cmd.Flags().GetBool("expired")
		archiveDays, _ := cmd.Flags().GetInt("archive-days")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		var deleted int

		if expired {
			if dryRun {
				stats, _ := store.GetDecayStats()
				output(titleStyle.Render("Forget Preview (Dry Run)"))
				output()
				output("  " + dimStyle.Render("Expired to delete:") + " " + itoa(stats.ExpiredCount))
				return nil
			}

			count, err := store.ForgetExpiredMemories()
			if err != nil {
				return err
			}
			deleted += count
		}

		if archiveDays > 0 {
			count, err := store.ForgetOldArchivedMemories(archiveDays)
			if err != nil {
				return err
			}
			deleted += count
		}

		output(titleStyle.Render("Forget Complete"))
		output()
		output("  " + dimStyle.Render("Deleted:") + " " + successStyle.Render(itoa(deleted)) + " memories")

		return nil
	},
}

func init() {
	decaySoftCmd.Flags().Float64("threshold", 0.3, "minimum importance to apply decay")

	decayArchiveCmd.Flags().Int("days", 90, "archive memories older than this")
	decayArchiveCmd.Flags().Float64("min-importance", 0.1, "archive below this importance")
	decayArchiveCmd.Flags().Bool("dry-run", false, "preview without executing")

	decayForgetCmd.Flags().Bool("expired", false, "delete memories past forget_after date")
	decayForgetCmd.Flags().Int("archive-days", 0, "delete archived memories older than this")
	decayForgetCmd.Flags().Bool("dry-run", false, "preview without executing")

	decayCmd.AddCommand(decayStatsCmd)
	decayCmd.AddCommand(decaySoftCmd)
	decayCmd.AddCommand(decayArchiveCmd)
	decayCmd.AddCommand(decayForgetCmd)
	rootCmd.AddCommand(decayCmd)
}

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
  claude-memory workdir set "Go Conventions" "claude-memory"

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

// --- Helpers ---

func printEntity(e *storage.Entity) {
	output(entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
	if len(e.Observations) > 0 {
		for _, obs := range e.Observations {
			output("  " + dimStyle.Render("•") + " " + obsStyle.Render(obs))
		}
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

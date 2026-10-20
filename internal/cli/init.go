package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

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

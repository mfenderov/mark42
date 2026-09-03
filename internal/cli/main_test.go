package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
	"github.com/spf13/cobra"
)

// captureOutput captures stdout/stderr during command execution.
func captureOutput(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), err
}

func TestVersionCommand(t *testing.T) {
	// Create a fresh root command for testing
	testCmd := &cobra.Command{Use: "mark42"}
	testCmd.AddCommand(&cobra.Command{
		Use: "version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("mark42 dev")
		},
	})

	output, err := captureOutput(t, testCmd, []string{"version"})
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	if !strings.Contains(output, "mark42") {
		t.Errorf("expected 'mark42' in output, got: %s", output)
	}
}

func TestInitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	// Temporarily set the global dbPath
	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}
	defer store.Close()

	// Verify database file was created
	if _, err := os.Stat(testDBPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestEntityCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	// Test entity create
	t.Run("CreateEntity", func(t *testing.T) {
		store, err := getStore()
		if err != nil {
			t.Fatalf("getStore failed: %v", err)
		}

		entity, err := store.CreateEntity("TestEntity", "test-type", []string{"observation1"})
		if err != nil {
			t.Fatalf("CreateEntity failed: %v", err)
		}

		if entity.Name != "TestEntity" {
			t.Errorf("expected name 'TestEntity', got %s", entity.Name)
		}
		if entity.Type != "test-type" {
			t.Errorf("expected type 'test-type', got %s", entity.Type)
		}
		store.Close()
	})

	// Test entity get
	t.Run("GetEntity", func(t *testing.T) {
		store, err := getStore()
		if err != nil {
			t.Fatalf("getStore failed: %v", err)
		}
		defer store.Close()

		entity, err := store.GetEntity("TestEntity")
		if err != nil {
			t.Fatalf("GetEntity failed: %v", err)
		}

		if entity.Name != "TestEntity" {
			t.Errorf("expected name 'TestEntity', got %s", entity.Name)
		}
		if len(entity.Observations) != 1 {
			t.Errorf("expected 1 observation, got %d", len(entity.Observations))
		}
	})

	// Test entity list
	t.Run("ListEntities", func(t *testing.T) {
		store, err := getStore()
		if err != nil {
			t.Fatalf("getStore failed: %v", err)
		}
		defer store.Close()

		entities, err := store.ListEntities("")
		if err != nil {
			t.Fatalf("ListEntities failed: %v", err)
		}

		if len(entities) == 0 {
			t.Error("expected at least 1 entity")
		}
	})
}

func TestObservationCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Create entity first
	store.CreateEntity("ObsTest", "test", nil)

	// Test add observation
	t.Run("AddObservation", func(t *testing.T) {
		err := store.AddObservation("ObsTest", "new observation")
		if err != nil {
			t.Fatalf("AddObservation failed: %v", err)
		}

		entity, _ := store.GetEntity("ObsTest")
		if len(entity.Observations) != 1 {
			t.Errorf("expected 1 observation, got %d", len(entity.Observations))
		}
	})

	// Test delete observation
	t.Run("DeleteObservation", func(t *testing.T) {
		err := store.DeleteObservation("ObsTest", "new observation")
		if err != nil {
			t.Fatalf("DeleteObservation failed: %v", err)
		}

		entity, _ := store.GetEntity("ObsTest")
		if len(entity.Observations) != 0 {
			t.Errorf("expected 0 observations after delete, got %d", len(entity.Observations))
		}
	})

	store.Close()
}

func TestRelationCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Create entities first
	store.CreateEntity("EntityA", "test", nil)
	store.CreateEntity("EntityB", "test", nil)

	// Test create relation
	t.Run("CreateRelation", func(t *testing.T) {
		err := store.CreateRelation("EntityA", "EntityB", "relates_to")
		if err != nil {
			t.Fatalf("CreateRelation failed: %v", err)
		}

		relations, _ := store.ListRelations("EntityA")
		if len(relations) == 0 {
			t.Error("expected at least 1 relation")
		}
	})

	// Test list relations
	t.Run("ListRelations", func(t *testing.T) {
		relations, err := store.ListRelations("EntityA")
		if err != nil {
			t.Fatalf("ListRelations failed: %v", err)
		}

		if len(relations) != 1 {
			t.Errorf("expected 1 relation, got %d", len(relations))
		}
		if relations[0].Type != "relates_to" {
			t.Errorf("expected relation type 'relates_to', got %s", relations[0].Type)
		}
	})

	// Test delete relation
	t.Run("DeleteRelation", func(t *testing.T) {
		err := store.DeleteRelation("EntityA", "EntityB", "relates_to")
		if err != nil {
			t.Fatalf("DeleteRelation failed: %v", err)
		}

		relations, _ := store.ListRelations("EntityA")
		if len(relations) != 0 {
			t.Errorf("expected 0 relations after delete, got %d", len(relations))
		}
	})

	store.Close()
}

func TestSearchCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Create entities with observations for search
	store.CreateEntity("SearchTest", "pattern", []string{"This is a searchable observation about Go programming"})
	store.CreateEntity("Another", "thing", []string{"Python is also a programming language"})

	// Test search
	t.Run("BasicSearch", func(t *testing.T) {
		results, err := store.SearchWithLimit("Go programming", 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			t.Error("expected at least 1 search result")
		}
	})

	// Test search with limit
	t.Run("SearchWithLimit", func(t *testing.T) {
		results, err := store.SearchWithLimit("programming", 1)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) > 1 {
			t.Errorf("expected at most 1 result with limit=1, got %d", len(results))
		}
	})

	store.Close()
}

func TestGraphCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Create some data
	store.CreateEntity("GraphNode1", "node", []string{"obs1"})
	store.CreateEntity("GraphNode2", "node", []string{"obs2"})
	store.CreateRelation("GraphNode1", "GraphNode2", "links_to")

	// Test read graph
	graph, err := store.ReadGraph()
	if err != nil {
		t.Fatalf("ReadGraph failed: %v", err)
	}

	if len(graph.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(graph.Entities))
	}
	if len(graph.Relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(graph.Relations))
	}

	store.Close()
}

func TestMigrateCommand_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	// Create a JSON file in Memory MCP format
	jsonData := `{
		"entities": [
			{"name": "ImportedEntity", "entityType": "imported", "observations": ["obs1", "obs2"]}
		],
		"relations": []
	}`
	jsonFile := filepath.Join(tmpDir, "memory.json")
	if err := os.WriteFile(jsonFile, []byte(jsonData), 0o644); err != nil {
		t.Fatalf("failed to write JSON file: %v", err)
	}

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Parse and import
	var memory jsonMemory
	data, _ := os.ReadFile(jsonFile)
	if err := json.Unmarshal(data, &memory); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	for _, e := range memory.Entities {
		_, err := store.CreateEntity(e.Name, e.EntityType, e.Observations)
		if err != nil {
			t.Logf("CreateEntity warning: %v", err)
		}
	}

	// Verify import
	entity, err := store.GetEntity("ImportedEntity")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if entity.Type != "imported" {
		t.Errorf("expected type 'imported', got %s", entity.Type)
	}
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}

	store.Close()
}

func TestMigrateCommand_NDJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	// Create an NDJSON file (Docker MCP style)
	ndjsonData := `{"type":"entity","name":"NDJSONEntity","entityType":"ndjson","observations":["obs1"]}
{"type":"relation","from":"NDJSONEntity","to":"Other","relationType":"knows"}`
	ndjsonFile := filepath.Join(tmpDir, "memory.ndjson")
	if err := os.WriteFile(ndjsonFile, []byte(ndjsonData), 0o644); err != nil {
		t.Fatalf("failed to write NDJSON file: %v", err)
	}

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Parse NDJSON
	data, _ := os.ReadFile(ndjsonFile)
	lines := strings.Split(string(data), "\n")

	var entities []jsonEntity
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var record ndjsonRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}

		if record.Type == "entity" {
			entities = append(entities, jsonEntity{
				Name:         record.Name,
				EntityType:   record.EntityType,
				Observations: record.Observations,
			})
		}
	}

	// Import
	for _, e := range entities {
		store.CreateEntity(e.Name, e.EntityType, e.Observations)
	}

	// Verify
	entity, err := store.GetEntity("NDJSONEntity")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if entity.Type != "ndjson" {
		t.Errorf("expected type 'ndjson', got %s", entity.Type)
	}

	store.Close()
}

func TestStatsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Create some data
	store.CreateEntity("StatsEntity1", "type1", []string{"obs1", "obs2"})
	store.CreateEntity("StatsEntity2", "type2", []string{"obs3"})
	store.CreateRelation("StatsEntity1", "StatsEntity2", "related")

	// Read graph for stats
	graph, err := store.ReadGraph()
	if err != nil {
		t.Fatalf("ReadGraph failed: %v", err)
	}

	// Calculate stats
	obsCount := 0
	for _, e := range graph.Entities {
		obsCount += len(e.Observations)
	}

	if len(graph.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(graph.Entities))
	}
	if obsCount != 3 {
		t.Errorf("expected 3 observations, got %d", obsCount)
	}
	if len(graph.Relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(graph.Relations))
	}

	store.Close()
}

func TestStatsCommand_EmbeddingCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}
	store.CreateEntity("E1", "test", []string{"obs1"})
	store.Close()

	// Capture output from stats command
	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	statsCmd.RunE(statsCmd, nil)

	got := buf.String()
	if !strings.Contains(got, "Embeddings:") {
		t.Errorf("stats output should contain 'Embeddings:', got:\n%s", got)
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("itoa", func(t *testing.T) {
		tests := []struct {
			input    int
			expected string
		}{
			{0, "0"},
			{1, "1"},
			{42, "42"},
			{100, "100"},
			{-5, "-5"},
		}

		for _, tt := range tests {
			result := itoa(tt.input)
			if result != tt.expected {
				t.Errorf("itoa(%d) = %s, expected %s", tt.input, result, tt.expected)
			}
		}
	})
}

func TestWorkdirCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Run migrations
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity
	store.CreateEntity("WorkdirEntity", "test", []string{"test observation"})

	// Test set container tag
	t.Run("SetContainerTag", func(t *testing.T) {
		err := store.SetContainerTag("WorkdirEntity", "my-project")
		if err != nil {
			t.Fatalf("SetContainerTag failed: %v", err)
		}
	})

	// Test get container tag
	t.Run("GetContainerTag", func(t *testing.T) {
		tag, err := store.GetContainerTag("WorkdirEntity")
		if err != nil {
			t.Fatalf("GetContainerTag failed: %v", err)
		}
		if tag != "my-project" {
			t.Errorf("expected tag 'my-project', got %s", tag)
		}
	})

	// Test list by container tag
	t.Run("GetEntitiesByContainerTag", func(t *testing.T) {
		entities, err := store.GetEntitiesByContainerTag("my-project")
		if err != nil {
			t.Fatalf("GetEntitiesByContainerTag failed: %v", err)
		}
		if len(entities) != 1 {
			t.Errorf("expected 1 entity, got %d", len(entities))
		}
	})

	store.Close()
}

func TestDecayCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Run migrations
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity
	store.CreateEntity("DecayEntity", "test", []string{"test observation"})

	// Test decay stats
	t.Run("GetDecayStats", func(t *testing.T) {
		stats, err := store.GetDecayStats()
		if err != nil {
			t.Fatalf("GetDecayStats failed: %v", err)
		}
		if stats.TotalObservations != 1 {
			t.Errorf("expected 1 total observation, got %d", stats.TotalObservations)
		}
	})

	// Test archive command with persisted config (uses REAL decayArchiveCmd)
	t.Run("ArchiveUsesPersistedConfig", func(t *testing.T) {
		// Create test entity with observation backdated 30 days with low importance
		if _, err := store.CreateEntity("OldMemory", "dynamic", []string{"observation to archive"}); err != nil {
			t.Fatalf("CreateEntity failed: %v", err)
		}
		// Backdate the observation to 30 days ago
		if _, err := store.DB().Exec(`UPDATE observations SET created_at = datetime('now', '-30 days'), last_accessed = datetime('now', '-30 days') WHERE content = 'observation to archive'`); err != nil {
			t.Fatalf("backdate observation failed: %v", err)
		}
		// Set low importance that will trigger archival with persisted config
		if _, err := store.DB().Exec(`UPDATE observations SET importance = 0.02 WHERE content = 'observation to archive'`); err != nil {
			t.Fatalf("set importance failed: %v", err)
		}

		// Persist a non-default DecayConfig: archive after 7 days with importance < 0.05
		// Our 30-day-old observation with importance=0.02 will be archived
		custom := storage.DefaultDecayConfig()
		custom.ArchiveAfterDays = 7
		custom.MinImportanceToKeep = 0.05
		if err := store.SetDecayConfig(custom); err != nil {
			t.Fatalf("SetDecayConfig failed: %v", err)
		}

		// Test WITHOUT flags: should use persisted config and archive the observation
		var outBuf bytes.Buffer
		decayArchiveCmd.SetOut(&outBuf)
		decayArchiveCmd.SetErr(&outBuf)
		decayArchiveCmd.SetArgs([]string{}) // No flags

		if err := decayArchiveCmd.RunE(decayArchiveCmd, []string{}); err != nil {
			t.Fatalf("decayArchiveCmd without flags failed: %v", err)
		}

		// Verify observation was archived (moved to archived_observations table)
		var archivedCount int
		if err := store.DB().Get(&archivedCount, "SELECT COUNT(*) FROM archived_observations WHERE content = 'observation to archive'"); err != nil {
			t.Fatalf("query archived_observations failed: %v", err)
		}
		if archivedCount != 1 {
			t.Errorf("expected 1 archived observation with persisted config, got %d", archivedCount)
		}

		// Clean up for next test
		if _, err := store.DB().Exec(`DELETE FROM archived_observations WHERE content = 'observation to archive'`); err != nil {
			t.Fatalf("cleanup archived failed: %v", err)
		}

		// Test WITH flags: flags should override persisted config
		// Create another old observation
		if _, err := store.CreateEntity("OldMemory2", "dynamic", []string{"observation2"}); err != nil {
			t.Fatalf("CreateEntity for second test failed: %v", err)
		}
		if _, err := store.DB().Exec(`UPDATE observations SET created_at = datetime('now', '-5 days'), last_accessed = datetime('now', '-5 days') WHERE content = 'observation2'`); err != nil {
			t.Fatalf("backdate observation2 failed: %v", err)
		}
		if _, err := store.DB().Exec(`UPDATE observations SET importance = 0.02 WHERE content = 'observation2'`); err != nil {
			t.Fatalf("set importance for observation2 failed: %v", err)
		}

		// Test flag override: --days 14 should NOT archive (5 days old < 14 days threshold)
		// We need to carefully reset flag state for the next invocation
		// Set flags explicitly: this marks them as Changed
		decayArchiveCmd.Flags().Set("days", "14")
		decayArchiveCmd.Flags().Set("min-importance", "0.1")

		var outBuf2 bytes.Buffer
		decayArchiveCmd.SetOut(&outBuf2)
		decayArchiveCmd.SetErr(&outBuf2)

		if err := decayArchiveCmd.RunE(decayArchiveCmd, []string{}); err != nil {
			t.Fatalf("decayArchiveCmd with flags failed: %v", err)
		}

		// Verify observation2 was NOT archived (5 days < 14 days threshold)
		if err := store.DB().Get(&archivedCount, "SELECT COUNT(*) FROM archived_observations WHERE content = 'observation2'"); err != nil {
			t.Fatalf("query archived_observations for observation2 failed: %v", err)
		}
		if archivedCount != 0 {
			t.Errorf("expected 0 archived observations with flag override (--days 14), got %d", archivedCount)
		}

		// Reset flags to defaults for next test iteration
		decayArchiveCmd.Flags().Set("days", "90")
		decayArchiveCmd.Flags().Set("min-importance", "0.1")
	})

	store.Close()
}

func TestContextCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Run migrations
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with observation
	store.CreateEntity("ContextEntity", "pattern", []string{"This is a pattern observation"})

	// Test context retrieval
	t.Run("GetContextForInjection", func(t *testing.T) {
		cfg := store.DefaultContextConfig()
		cfg.MinImportance = 0 // Include all

		results, err := store.GetContextForInjection(cfg, "", "", nil)
		if err != nil {
			t.Fatalf("GetContextForInjection failed: %v", err)
		}
		if len(results) < 1 {
			t.Error("expected at least 1 context result")
		}
	})

	store.Close()
}

func TestImportanceCommands(t *testing.T) {
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test.db")

	oldDBPath := dbPath
	dbPath = testDBPath
	defer func() { dbPath = oldDBPath }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore failed: %v", err)
	}

	// Run migrations
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity
	store.CreateEntity("ImportanceEntity", "test", []string{"test observation"})

	// Test recalculate importance
	t.Run("RecalculateImportance", func(t *testing.T) {
		updated, err := store.RecalculateImportance()
		if err != nil {
			t.Fatalf("RecalculateImportance failed: %v", err)
		}
		if updated < 0 {
			t.Errorf("expected non-negative updated count, got %d", updated)
		}
	})

	store.Close()
}

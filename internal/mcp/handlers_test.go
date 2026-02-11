package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/claude-memory/internal/mcp"
	"github.com/mfenderov/claude-memory/internal/storage"
)

// newTestHandler creates a handler with a fresh test store
func newTestHandler(t *testing.T) (*mcp.Handler, *storage.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	handler := mcp.NewHandler(store)
	return handler, store
}

// --- Tools() tests ---

func TestHandler_Tools(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tools := handler.Tools()

	expectedTools := []string{
		"create_entities",
		"create_or_update_entities",
		"create_relations",
		"add_observations",
		"delete_entities",
		"delete_observations",
		"delete_relations",
		"read_graph",
		"search_nodes",
		"open_nodes",
		"get_context",
		"get_recent_context",
		"summarize_entity",
		"consolidate_memories",
	}

	if len(tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("expected tool %q not found", expected)
		}
	}
}

// --- CallTool unknown tool test ---

func TestHandler_CallTool_UnknownTool(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	_, err := handler.CallTool("nonexistent_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected 'unknown tool' error, got: %v", err)
	}
}

// --- create_entities tests ---

func TestHandler_CreateEntities(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		wantErr     bool
		wantCreated int
		errContains string
	}{
		{
			name: "create single entity",
			args: `{
				"entities": [
					{"name": "TDD", "entityType": "pattern", "observations": ["Test-Driven Development"]}
				]
			}`,
			wantCreated: 1,
		},
		{
			name: "create multiple entities",
			args: `{
				"entities": [
					{"name": "TDD", "entityType": "pattern", "observations": ["Test-Driven Development"]},
					{"name": "konfig", "entityType": "project", "observations": ["Config library"]}
				]
			}`,
			wantCreated: 2,
		},
		{
			name: "create entity without observations",
			args: `{
				"entities": [
					{"name": "Empty", "entityType": "test", "observations": []}
				]
			}`,
			wantCreated: 1,
		},
		{
			name:        "invalid JSON",
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
		{
			name:        "missing entities field",
			args:        `{}`,
			wantCreated: 0, // Empty array, no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			result, err := handler.CallTool("create_entities", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			if len(result.Content) == 0 {
				t.Error("expected content in result")
			}

			// Verify entities were created
			entities, _ := store.ListEntities("")
			if len(entities) != tt.wantCreated {
				t.Errorf("expected %d entities, got %d", tt.wantCreated, len(entities))
			}
		})
	}
}

// --- create_or_update_entities tests ---

func TestHandler_CreateOrUpdateEntities(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		errContains string
		checkResult func(t *testing.T, store *storage.Store, text string)
	}{
		{
			name:  "create new entity",
			setup: func(s *storage.Store) {},
			args: `{
				"entities": [
					{"name": "NewEntity", "entityType": "test", "observations": ["first observation"]}
				]
			}`,
			checkResult: func(t *testing.T, store *storage.Store, text string) {
				entity, err := store.GetEntity("NewEntity")
				if err != nil {
					t.Fatalf("entity not created: %v", err)
				}
				if entity.Version != 1 {
					t.Errorf("expected version 1, got %d", entity.Version)
				}
				if !entity.IsLatest {
					t.Error("expected entity to be latest")
				}
			},
		},
		{
			name: "update existing entity creates new version",
			setup: func(s *storage.Store) {
				s.CreateEntity("ExistingEntity", "test", []string{"original observation"})
			},
			args: `{
				"entities": [
					{"name": "ExistingEntity", "entityType": "test", "observations": ["updated observation"]}
				]
			}`,
			checkResult: func(t *testing.T, store *storage.Store, text string) {
				entity, err := store.GetEntity("ExistingEntity")
				if err != nil {
					t.Fatalf("entity not found: %v", err)
				}
				if entity.Version != 2 {
					t.Errorf("expected version 2, got %d", entity.Version)
				}
				if !entity.IsLatest {
					t.Error("expected entity to be latest")
				}
				// Check observations are from the new version
				if len(entity.Observations) != 1 || entity.Observations[0] != "updated observation" {
					t.Errorf("unexpected observations: %v", entity.Observations)
				}
				// Verify response includes version info
				if !strings.Contains(text, "v2") {
					t.Errorf("expected response to contain version info, got: %s", text)
				}
			},
		},
		{
			name:  "create multiple entities",
			setup: func(s *storage.Store) {},
			args: `{
				"entities": [
					{"name": "EntityA", "entityType": "test", "observations": ["obs A"]},
					{"name": "EntityB", "entityType": "test", "observations": ["obs B"]}
				]
			}`,
			checkResult: func(t *testing.T, store *storage.Store, text string) {
				entities, _ := store.ListEntities("")
				if len(entities) != 2 {
					t.Errorf("expected 2 entities, got %d", len(entities))
				}
			},
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("create_or_update_entities", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			if len(result.Content) == 0 {
				t.Error("expected content in result")
			}

			if tt.checkResult != nil {
				tt.checkResult(t, store, result.Content[0].Text)
			}
		})
	}
}

func TestHandler_CreateEntities_DuplicateAddsObservations(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	// First create
	args1 := `{"entities": [{"name": "TDD", "entityType": "pattern", "observations": ["obs1"]}]}`
	_, err := handler.CallTool("create_entities", json.RawMessage(args1))
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Second create with same name - should add observations
	args2 := `{"entities": [{"name": "TDD", "entityType": "pattern", "observations": ["obs2"]}]}`
	_, err = handler.CallTool("create_entities", json.RawMessage(args2))
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	// Verify observations were added
	entity, err := store.GetEntity("TDD")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}
}

// --- create_relations tests ---

func TestHandler_CreateRelations(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		wantCreated int
		errContains string
	}{
		{
			name: "create single relation",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
				s.CreateEntity("konfig", "project", nil)
			},
			args: `{
				"relations": [
					{"from": "TDD", "to": "konfig", "relationType": "used_by"}
				]
			}`,
			wantCreated: 1,
		},
		{
			name: "create multiple relations",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
				s.CreateEntity("konfig", "project", nil)
				s.CreateEntity("claude-memory", "project", nil)
			},
			args: `{
				"relations": [
					{"from": "TDD", "to": "konfig", "relationType": "used_by"},
					{"from": "TDD", "to": "claude-memory", "relationType": "used_by"}
				]
			}`,
			wantCreated: 2,
		},
		{
			name:  "relation with nonexistent entity",
			setup: func(s *storage.Store) {},
			args: `{
				"relations": [
					{"from": "nonexistent", "to": "also_nonexistent", "relationType": "relates"}
				]
			}`,
			wantCreated: 0, // Should fail silently
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("create_relations", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			// Check response text contains count
			if len(result.Content) > 0 {
				text := result.Content[0].Text
				if !strings.Contains(text, "Created") {
					t.Errorf("expected 'Created' in response, got: %s", text)
				}
			}
		})
	}
}

// --- add_observations tests ---

func TestHandler_AddObservations(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		wantAdded   int
		errContains string
	}{
		{
			name: "add single observation",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args: `{
				"observations": [
					{"entityName": "TDD", "contents": ["new observation"]}
				]
			}`,
			wantAdded: 1,
		},
		{
			name: "add multiple observations to same entity",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args: `{
				"observations": [
					{"entityName": "TDD", "contents": ["obs1", "obs2", "obs3"]}
				]
			}`,
			wantAdded: 3,
		},
		{
			name: "add observations with fact type",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args: `{
				"observations": [
					{"entityName": "TDD", "contents": ["static fact"], "factType": "static"}
				]
			}`,
			wantAdded: 1,
		},
		{
			name:  "add to nonexistent entity",
			setup: func(s *storage.Store) {},
			args: `{
				"observations": [
					{"entityName": "nonexistent", "contents": ["obs"]}
				]
			}`,
			wantAdded: 0, // Should fail silently
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("add_observations", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}
		})
	}
}

// --- delete_entities tests ---

func TestHandler_DeleteEntities(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		wantRemain  int
		errContains string
	}{
		{
			name: "delete single entity",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
				s.CreateEntity("konfig", "project", nil)
			},
			args:       `{"entityNames": ["TDD"]}`,
			wantRemain: 1,
		},
		{
			name: "delete multiple entities",
			setup: func(s *storage.Store) {
				s.CreateEntity("A", "test", nil)
				s.CreateEntity("B", "test", nil)
				s.CreateEntity("C", "test", nil)
			},
			args:       `{"entityNames": ["A", "B"]}`,
			wantRemain: 1,
		},
		{
			name: "delete nonexistent entity",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args:       `{"entityNames": ["nonexistent"]}`,
			wantRemain: 1, // Original still exists
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("delete_entities", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			// Verify remaining entities
			entities, _ := store.ListEntities("")
			if len(entities) != tt.wantRemain {
				t.Errorf("expected %d remaining entities, got %d", tt.wantRemain, len(entities))
			}
		})
	}
}

// --- delete_observations tests ---

func TestHandler_DeleteObservations(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		wantRemain  int
		errContains string
	}{
		{
			name: "delete single observation",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"obs1", "obs2"})
			},
			args: `{
				"deletions": [
					{"entityName": "TDD", "observations": ["obs1"]}
				]
			}`,
			wantRemain: 1,
		},
		{
			name: "delete multiple observations",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"obs1", "obs2", "obs3"})
			},
			args: `{
				"deletions": [
					{"entityName": "TDD", "observations": ["obs1", "obs2"]}
				]
			}`,
			wantRemain: 1,
		},
		{
			name: "delete nonexistent observation",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"real_obs"})
			},
			args: `{
				"deletions": [
					{"entityName": "TDD", "observations": ["nonexistent"]}
				]
			}`,
			wantRemain: 1,
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("delete_observations", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			// Verify remaining observations
			entity, err := store.GetEntity("TDD")
			if err == nil && len(entity.Observations) != tt.wantRemain {
				t.Errorf("expected %d remaining observations, got %d", tt.wantRemain, len(entity.Observations))
			}
		})
	}
}

// --- delete_relations tests ---

func TestHandler_DeleteRelations(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		errContains string
	}{
		{
			name: "delete single relation",
			setup: func(s *storage.Store) {
				s.CreateEntity("A", "test", nil)
				s.CreateEntity("B", "test", nil)
				s.CreateRelation("A", "B", "relates_to")
			},
			args: `{
				"relations": [
					{"from": "A", "to": "B", "relationType": "relates_to"}
				]
			}`,
		},
		{
			name: "delete nonexistent relation",
			setup: func(s *storage.Store) {
				s.CreateEntity("A", "test", nil)
				s.CreateEntity("B", "test", nil)
			},
			args: `{
				"relations": [
					{"from": "A", "to": "B", "relationType": "nonexistent"}
				]
			}`,
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("delete_relations", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}
		})
	}
}

// --- read_graph tests ---

func TestHandler_ReadGraph(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*storage.Store)
		wantErr    bool
		checkGraph func(t *testing.T, graphJSON string)
	}{
		{
			name:  "read empty graph",
			setup: func(s *storage.Store) {},
			checkGraph: func(t *testing.T, graphJSON string) {
				var graph map[string]any
				if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
					t.Fatalf("failed to parse graph JSON: %v", err)
				}
				// Empty graph may have null or empty array for Entities (PascalCase)
				entities, ok := graph["Entities"].([]any)
				if ok && len(entities) != 0 {
					t.Errorf("expected 0 entities, got %d", len(entities))
				}
			},
		},
		{
			name: "read graph with entities and relations",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
				s.CreateEntity("konfig", "project", []string{"Config library"})
				s.CreateRelation("TDD", "konfig", "used_by")
			},
			checkGraph: func(t *testing.T, graphJSON string) {
				var graph map[string]any
				if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
					t.Fatalf("failed to parse graph JSON: %v", err)
				}
				// Graph struct fields are "Entities" and "Relations" (PascalCase)
				entities, ok := graph["Entities"].([]any)
				if !ok {
					t.Logf("DEBUG graph JSON: %s", graphJSON)
					t.Fatal("Entities should be an array")
				}
				if len(entities) != 2 {
					t.Errorf("expected 2 entities, got %d", len(entities))
				}
				relations, ok := graph["Relations"].([]any)
				if !ok {
					t.Fatal("Relations should be an array")
				}
				if len(relations) != 1 {
					t.Errorf("expected 1 relation, got %d", len(relations))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("read_graph", json.RawMessage(`{}`))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			if len(result.Content) == 0 {
				t.Fatal("expected content in result")
			}

			if tt.checkGraph != nil {
				tt.checkGraph(t, result.Content[0].Text)
			}
		})
	}
}

// --- search_nodes tests ---

func TestHandler_SearchNodes(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*storage.Store)
		args         string
		wantErr      bool
		wantResults  int
		errContains  string
		checkResults func(t *testing.T, resultJSON string)
	}{
		{
			name: "search finds matching entities",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
				s.CreateEntity("konfig", "project", []string{"Configuration library"})
			},
			args:        `{"query": "Test-Driven"}`,
			wantResults: 1,
		},
		{
			name: "search with no matches",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
			},
			args:        `{"query": "nonexistent_query_xyz"}`,
			wantResults: 0,
		},
		{
			name:  "search empty database",
			setup: func(s *storage.Store) {},
			args:  `{"query": "anything"}`,
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("search_nodes", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			// Result should be valid JSON array
			if len(result.Content) > 0 {
				var entities []any
				if err := json.Unmarshal([]byte(result.Content[0].Text), &entities); err != nil {
					t.Fatalf("failed to parse search results: %v", err)
				}

				if tt.wantResults > 0 && len(entities) != tt.wantResults {
					t.Errorf("expected %d results, got %d", tt.wantResults, len(entities))
				}

				if tt.checkResults != nil {
					tt.checkResults(t, result.Content[0].Text)
				}
			}
		})
	}
}

// --- open_nodes tests ---

func TestHandler_OpenNodes(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*storage.Store)
		args         string
		wantErr      bool
		wantEntities int
		errContains  string
	}{
		{
			name: "open single node",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
			},
			args:         `{"names": ["TDD"]}`,
			wantEntities: 1,
		},
		{
			name: "open multiple nodes",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
				s.CreateEntity("konfig", "project", []string{"Config library"})
			},
			args:         `{"names": ["TDD", "konfig"]}`,
			wantEntities: 2,
		},
		{
			name: "open nonexistent node",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args:         `{"names": ["nonexistent"]}`,
			wantEntities: 0, // Should return empty array
		},
		{
			name: "open mix of existing and nonexistent",
			setup: func(s *storage.Store) {
				s.CreateEntity("TDD", "pattern", nil)
			},
			args:         `{"names": ["TDD", "nonexistent"]}`,
			wantEntities: 1, // Only existing one returned
		},
		{
			name:        "invalid JSON",
			setup:       func(s *storage.Store) {},
			args:        `{invalid}`,
			wantErr:     true,
			errContains: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("open_nodes", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			// Parse and verify entity count
			if len(result.Content) > 0 {
				var entities []any
				if err := json.Unmarshal([]byte(result.Content[0].Text), &entities); err != nil {
					t.Fatalf("failed to parse open_nodes results: %v", err)
				}

				if len(entities) != tt.wantEntities {
					t.Errorf("expected %d entities, got %d", tt.wantEntities, len(entities))
				}
			}
		})
	}
}

// --- get_context tests ---

func TestHandler_GetContext(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*storage.Store)
		args        string
		wantErr     bool
		checkResult func(t *testing.T, text string)
	}{
		{
			name: "get context with data",
			setup: func(s *storage.Store) {
				s.Migrate()
				s.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
				s.SetObservationImportance("TDD", "Test-Driven Development", 0.8)
			},
			args: `{}`,
			checkResult: func(t *testing.T, text string) {
				if !strings.Contains(text, "TDD") {
					t.Error("expected output to contain 'TDD'")
				}
			},
		},
		{
			name:  "get context empty database",
			setup: func(s *storage.Store) { s.Migrate() },
			args:  `{}`,
			checkResult: func(t *testing.T, text string) {
				if !strings.Contains(text, "No relevant memories") {
					t.Error("expected 'No relevant memories' message")
				}
			},
		},
		{
			name: "get context with project filter",
			setup: func(s *storage.Store) {
				s.Migrate()
				s.CreateEntity("claude-memory", "project", []string{"Memory system"})
				s.SetObservationImportance("claude-memory", "Memory system", 0.7)
			},
			args: `{"projectName": "claude-memory"}`,
			checkResult: func(t *testing.T, text string) {
				if !strings.Contains(text, "claude-memory") {
					t.Error("expected output to contain 'claude-memory'")
				}
			},
		},
		{
			name:    "invalid JSON",
			setup:   func(s *storage.Store) { s.Migrate() },
			args:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, store := newTestHandler(t)
			defer store.Close()

			if tt.setup != nil {
				tt.setup(store)
			}

			result, err := handler.CallTool("get_context", json.RawMessage(tt.args))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil || len(result.Content) == 0 {
				t.Fatal("expected result with content")
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result.Content[0].Text)
			}
		})
	}
}

// --- WithEmbedder tests ---

func TestHandler_WithEmbedder(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	// Create a mock embedder (nil is valid - just tests the builder pattern)
	handler2 := handler.WithEmbedder(nil)

	if handler2 == nil {
		t.Error("WithEmbedder should return handler")
	}

	// Should be same handler (fluent API)
	if handler2 != handler {
		t.Error("WithEmbedder should return same handler instance")
	}
}

// --- Auto-embed tests ---

type fakeEmbedder struct {
	calls int
}

func (f *fakeEmbedder) CreateEmbedding(_ context.Context, _ string) ([]float64, error) {
	f.calls++
	return []float64{0.1, 0.2, 0.3}, nil
}

func TestHandler_AutoEmbed_CreateEntities(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	embedder := &fakeEmbedder{}
	handler.WithEmbedder(embedder)

	args := `{"entities": [{"name": "Go", "entityType": "language", "observations": ["Compiled language", "Has goroutines"]}]}`
	_, err := handler.CallTool("create_entities", json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_entities failed: %v", err)
	}

	// Verify embeddings were generated
	if embedder.calls != 2 {
		t.Errorf("expected 2 embedding calls, got %d", embedder.calls)
	}

	// Verify embeddings stored in database
	_, withEmb, err := store.EmbeddingStats()
	if err != nil {
		t.Fatalf("EmbeddingStats failed: %v", err)
	}
	if withEmb != 2 {
		t.Errorf("expected 2 stored embeddings, got %d", withEmb)
	}
}

func TestHandler_AutoEmbed_AddObservations(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	store.CreateEntity("Go", "language", nil)

	embedder := &fakeEmbedder{}
	handler.WithEmbedder(embedder)

	args := `{"observations": [{"entityName": "Go", "contents": ["Fast compilation"]}]}`
	_, err := handler.CallTool("add_observations", json.RawMessage(args))
	if err != nil {
		t.Fatalf("add_observations failed: %v", err)
	}

	if embedder.calls != 1 {
		t.Errorf("expected 1 embedding call, got %d", embedder.calls)
	}

	_, withEmb, _ := store.EmbeddingStats()
	if withEmb != 1 {
		t.Errorf("expected 1 stored embedding, got %d", withEmb)
	}
}

func TestHandler_AutoEmbed_NoEmbedder(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	// No embedder configured — should work fine without embeddings
	args := `{"entities": [{"name": "Go", "entityType": "language", "observations": ["Compiled"]}]}`
	result, err := handler.CallTool("create_entities", json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_entities failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}

	_, withEmb, _ := store.EmbeddingStats()
	if withEmb != 0 {
		t.Errorf("expected 0 embeddings without embedder, got %d", withEmb)
	}
}

// --- Response format tests ---

func TestHandler_ResponseFormat(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	// Create some data
	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})

	// Test open_nodes response format
	result, err := handler.CallTool("open_nodes", json.RawMessage(`{"names": ["TDD"]}`))
	if err != nil {
		t.Fatalf("open_nodes failed: %v", err)
	}

	// Verify response structure
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}

	if result.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got %q", result.Content[0].Type)
	}

	// Verify JSON structure includes expected fields
	var entities []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &entities); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	entity := entities[0]
	if entity["name"] != "TDD" {
		t.Errorf("expected name 'TDD', got %v", entity["name"])
	}
	if entity["entityType"] != "pattern" {
		t.Errorf("expected entityType 'pattern', got %v", entity["entityType"])
	}
	if entity["observations"] == nil {
		t.Error("expected observations field")
	}
}

// --- NewHandler test ---

func TestNewHandler(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := mcp.NewHandler(store)
	if handler == nil {
		t.Error("NewHandler returned nil")
	}

	// Should have tools
	tools := handler.Tools()
	if len(tools) == 0 {
		t.Error("handler has no tools")
	}
}

// --- Boundary tests ---

func TestHandler_EmptyInputs(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tests := []struct {
		tool string
		args string
	}{
		{"create_entities", `{"entities": []}`},
		{"create_relations", `{"relations": []}`},
		{"add_observations", `{"observations": []}`},
		{"delete_entities", `{"entityNames": []}`},
		{"delete_observations", `{"deletions": []}`},
		{"delete_relations", `{"relations": []}`},
		{"open_nodes", `{"names": []}`},
	}

	for _, tt := range tests {
		t.Run(tt.tool+"_empty", func(t *testing.T) {
			result, err := handler.CallTool(tt.tool, json.RawMessage(tt.args))
			if err != nil {
				t.Errorf("%s with empty input failed: %v", tt.tool, err)
			}
			if result == nil {
				t.Errorf("%s with empty input returned nil result", tt.tool)
			}
		})
	}
}

// --- get_recent_context tests ---

func TestHandler_GetRecentContext(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	store.Migrate()
	store.CreateEntity("RecentWork", "project", []string{"Working on this now"})
	store.UpdateLastAccessed("RecentWork")

	result, err := handler.CallTool("get_recent_context", json.RawMessage(`{"hours": 24}`))
	if err != nil {
		t.Fatalf("get_recent_context failed: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	if !strings.Contains(result.Content[0].Text, "RecentWork") {
		t.Errorf("expected output to contain 'RecentWork', got: %s", result.Content[0].Text)
	}
}

func TestHandler_GetRecentContext_Empty(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	store.Migrate()

	result, err := handler.CallTool("get_recent_context", json.RawMessage(`{"hours": 1}`))
	if err != nil {
		t.Fatalf("get_recent_context failed: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "No recent memories") {
		t.Errorf("expected 'No recent memories' message, got: %s", result.Content[0].Text)
	}
}

// --- summarize_entity tests ---

func TestHandler_SummarizeEntity(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development", "Red-Green-Refactor"})
	store.CreateEntity("konfig", "project", nil)
	store.CreateRelation("TDD", "konfig", "used_by")

	result, err := handler.CallTool("summarize_entity", json.RawMessage(`{"entityName": "TDD"}`))
	if err != nil {
		t.Fatalf("summarize_entity failed: %v", err)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "TDD") {
		t.Error("expected entity name in summary")
	}
	if !strings.Contains(text, "pattern") {
		t.Error("expected entity type in summary")
	}
	if !strings.Contains(text, "used_by") {
		t.Error("expected relation type in summary")
	}
}

func TestHandler_SummarizeEntity_NotFound(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	_, err := handler.CallTool("summarize_entity", json.RawMessage(`{"entityName": "nonexistent"}`))
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

// --- consolidate_memories tests ---

func TestHandler_ConsolidateMemories(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	// Create entity with redundant observations
	store.CreateEntity("Go", "language", []string{
		"Compiled language",
		"Go is a compiled language with fast build times",
		"Has goroutines for concurrency",
	})

	result, err := handler.CallTool("consolidate_memories", json.RawMessage(`{"entityName": "Go"}`))
	if err != nil {
		t.Fatalf("consolidate_memories failed: %v", err)
	}

	text := result.Content[0].Text
	if !strings.Contains(text, "consolidated") {
		t.Errorf("expected 'consolidated' in result, got: %s", text)
	}

	// Verify: "Compiled language" should be removed (it's a substring of the longer one)
	entity, _ := store.GetEntity("Go")
	for _, obs := range entity.Observations {
		if obs == "Compiled language" {
			t.Error("short duplicate observation should have been removed")
		}
	}
}

func TestHandler_ConsolidateMemories_NothingToConsolidate(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	store.CreateEntity("Go", "language", []string{"Only one observation"})

	result, err := handler.CallTool("consolidate_memories", json.RawMessage(`{"entityName": "Go"}`))
	if err != nil {
		t.Fatalf("consolidate_memories failed: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, "nothing to consolidate") {
		t.Errorf("expected 'nothing to consolidate', got: %s", result.Content[0].Text)
	}
}

// --- Tools count test update ---

func TestHandler_Tools_Count(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tools := handler.Tools()
	// 11 original + 3 new (get_recent_context, summarize_entity, consolidate_memories)
	if len(tools) != 14 {
		t.Errorf("expected 14 tools, got %d", len(tools))
	}
}

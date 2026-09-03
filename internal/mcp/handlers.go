package mcp

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/log"

	"github.com/mfenderov/mark42/internal/storage"
)

var logger = log.NewWithOptions(os.Stderr, log.Options{
	ReportTimestamp: false,
})

// Handler processes MCP tool calls using the storage layer.
type Handler struct {
	store    *storage.Store
	embedder storage.Embedder // Optional: enables semantic search + auto-embed on write
}

// NewHandler creates a new MCP handler with the given store.
func NewHandler(store *storage.Store) *Handler {
	return &Handler{store: store}
}

// WithEmbedder adds an embedding client for semantic search and auto-embedding.
func (h *Handler) WithEmbedder(client storage.Embedder) *Handler {
	h.embedder = client
	return h
}

// Tools returns the list of available memory tools.
func (h *Handler) Tools() []Tool {
	return []Tool{
		{
			Name:        "create_entities",
			Description: "Create multiple new entities in the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entities": {
						Type:        "array",
						Description: "Array of entities to create",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"name":         {Type: "string", Description: "Entity name"},
								"entityType":   {Type: "string", Description: "Entity type"},
								"observations": {Type: "array", Description: "Initial observations", Items: &Items{Type: "string"}},
							},
							Required: []string{"name", "entityType", "observations"},
						},
					},
				},
				Required: []string{"entities"},
			},
		},
		{
			Name:        "create_or_update_entities",
			Description: "Create new entities or update existing ones with versioning support. If an entity exists, creates a new version.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entities": {
						Type:        "array",
						Description: "Array of entities to create or update",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"name":         {Type: "string", Description: "Entity name"},
								"entityType":   {Type: "string", Description: "Entity type"},
								"observations": {Type: "array", Description: "Observations for this version", Items: &Items{Type: "string"}},
							},
							Required: []string{"name", "entityType", "observations"},
						},
					},
				},
				Required: []string{"entities"},
			},
		},
		{
			Name:        "create_relations",
			Description: "Create multiple new relations between entities in the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"relations": {
						Type:        "array",
						Description: "Array of relations to create",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"from":         {Type: "string", Description: "Source entity name"},
								"to":           {Type: "string", Description: "Target entity name"},
								"relationType": {Type: "string", Description: "Relation type"},
							},
							Required: []string{"from", "to", "relationType"},
						},
					},
				},
				Required: []string{"relations"},
			},
		},
		{
			Name:        "add_observations",
			Description: "Add new observations to existing entities in the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"observations": {
						Type:        "array",
						Description: "Array of observations to add",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"entityName": {Type: "string", Description: "Entity name to add observations to"},
								"contents":   {Type: "array", Description: "Observation contents", Items: &Items{Type: "string"}},
								"factType":   {Type: "string", Description: "Optional fact type: 'static' (permanent), 'dynamic' (session), 'session_turn' (conversation)"},
							},
							Required: []string{"entityName", "contents"},
						},
					},
				},
				Required: []string{"observations"},
			},
		},
		{
			Name:        "delete_entities",
			Description: "Delete multiple entities and their associated relations from the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entityNames": {Type: "array", Description: "Entity names to delete", Items: &Items{Type: "string"}},
				},
				Required: []string{"entityNames"},
			},
		},
		{
			Name:        "delete_observations",
			Description: "Delete specific observations from entities in the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"deletions": {
						Type:        "array",
						Description: "Array of deletions",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"entityName":   {Type: "string", Description: "Entity name"},
								"observations": {Type: "array", Description: "Observations to delete", Items: &Items{Type: "string"}},
							},
							Required: []string{"entityName", "observations"},
						},
					},
				},
				Required: []string{"deletions"},
			},
		},
		{
			Name:        "delete_relations",
			Description: "Delete multiple relations from the knowledge graph",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"relations": {
						Type:        "array",
						Description: "Array of relations to delete",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"from":         {Type: "string", Description: "Source entity name"},
								"to":           {Type: "string", Description: "Target entity name"},
								"relationType": {Type: "string", Description: "Relation type"},
							},
							Required: []string{"from", "to", "relationType"},
						},
					},
				},
				Required: []string{"relations"},
			},
		},
		{
			Name:        "read_graph",
			Description: "Read the entire knowledge graph",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "search_nodes",
			Description: "Search for nodes in the knowledge graph based on a query",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {Type: "string", Description: "Search query"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "open_nodes",
			Description: "Open specific nodes in the knowledge graph by their names",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"names": {Type: "array", Description: "Entity names to retrieve", Items: &Items{Type: "string"}},
				},
				Required: []string{"names"},
			},
		},
		{
			Name:        "get_context",
			Description: "Get memories optimized for context injection, ordered by importance and fact type",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"projectName":   {Type: "string", Description: "Current project name for boosting relevant memories"},
					"tokenBudget":   {Type: "integer", Description: "Maximum tokens to include (default: 2000)"},
					"minImportance": {Type: "number", Description: "Minimum importance score (0-1, default: 0.3)"},
					"query":         {Type: "string", Description: "Optional search query to focus context on relevant entities"},
				},
			},
		},
		{
			Name:        "get_recent_context",
			Description: "Get recently accessed memories, prioritizing recency over importance. For mid-session use.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"hours":       {Type: "integer", Description: "Time window in hours (default: 24)"},
					"projectName": {Type: "string", Description: "Current project name for boosting relevant memories"},
					"tokenBudget": {Type: "integer", Description: "Maximum tokens to include (default: 1000)"},
				},
			},
		},
		{
			Name:        "summarize_entity",
			Description: "Get a consolidated summary of an entity with observations grouped by fact type and metadata",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entityName": {Type: "string", Description: "Name of the entity to summarize"},
				},
				Required: []string{"entityName"},
			},
		},
		{
			Name:        "consolidate_memories",
			Description: "Merge duplicate or similar observations for an entity, keeping the most comprehensive version",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entityName": {Type: "string", Description: "Name of the entity whose observations to consolidate"},
					"mode":       {Type: "string", Description: "Consolidation mode: 'semantic' uses embedding similarity, default uses substring matching"},
					"threshold":  {Type: "number", Description: "Similarity threshold for semantic mode (0.0-1.0, default 0.85)"},
				},
				Required: []string{"entityName"},
			},
		},
		{
			Name:        "capture_session",
			Description: "Capture a completed session with summary and optional tool-use events for cross-session recall",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"projectName": {Type: "string", Description: "Project name for the session"},
					"summary":     {Type: "string", Description: "What was accomplished in this session"},
					"events": {
						Type:        "array",
						Description: "Tool-use events from the session",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"toolName":  {Type: "string", Description: "Tool name (Edit, Bash, etc.)"},
								"filePath":  {Type: "string", Description: "File path if applicable"},
								"command":   {Type: "string", Description: "Command if Bash tool"},
								"timestamp": {Type: "string", Description: "ISO 8601 timestamp"},
							},
							Required: []string{"toolName"},
						},
					},
				},
				Required: []string{"projectName", "summary"},
			},
		},
		{
			Name:        "recall_sessions",
			Description: "Recall recent session summaries for a project to understand what was done in previous sessions",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"projectName": {Type: "string", Description: "Project name to filter sessions"},
					"hours":       {Type: "integer", Description: "Time window in hours (default: 72)"},
					"tokenBudget": {Type: "integer", Description: "Maximum tokens to include (default: 1500)"},
				},
			},
		},
		{
			Name:        "invalidate_observation",
			Description: "Mark a specific observation as no longer valid (expired). The observation will be hidden from normal queries but preserved in history.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entityName": {Type: "string", Description: "Entity name"},
					"content":    {Type: "string", Description: "Exact content of the observation to invalidate"},
				},
				Required: []string{"entityName", "content"},
			},
		},
		{
			Name:        "get_entity_history",
			Description: "Get the full history of observations for an entity, including expired ones with their validity timestamps.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entityName": {Type: "string", Description: "Name of the entity"},
				},
				Required: []string{"entityName"},
			},
		},
	}
}

// toolDispatch maps tool names to handler methods (method expressions).
var toolDispatch = map[string]func(*Handler, json.RawMessage) (*ToolCallResult, error){
	"create_entities":           (*Handler).createEntities,
	"create_or_update_entities": (*Handler).createOrUpdateEntities,
	"create_relations":          (*Handler).createRelations,
	"add_observations":          (*Handler).addObservations,
	"delete_entities":           (*Handler).deleteEntities,
	"delete_observations":       (*Handler).deleteObservations,
	"delete_relations":          (*Handler).deleteRelations,
	"read_graph":                func(h *Handler, _ json.RawMessage) (*ToolCallResult, error) { return h.readGraph() },
	"search_nodes":              (*Handler).searchNodes,
	"open_nodes":                (*Handler).openNodes,
	"get_context":               (*Handler).getContext,
	"get_recent_context":        (*Handler).getRecentContext,
	"summarize_entity":          (*Handler).summarizeEntity,
	"consolidate_memories":      (*Handler).consolidateMemories,
	"capture_session":           (*Handler).captureSession,
	"recall_sessions":           (*Handler).recallSessions,
	"invalidate_observation":    (*Handler).invalidateObservation,
	"get_entity_history":        (*Handler).getEntityHistory,
}

// CallTool executes the named tool with the given arguments.
func (h *Handler) CallTool(name string, args json.RawMessage) (*ToolCallResult, error) {
	if fn, ok := toolDispatch[name]; ok {
		return fn(h, args)
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

// Embedder generates vector embeddings for text.
type Embedder interface {
	CreateEmbedding(ctx context.Context, text string) ([]float64, error)
}

// Handler processes MCP tool calls using the storage layer.
type Handler struct {
	store    *storage.Store
	embedder Embedder // Optional: enables semantic search + auto-embed on write
}

// NewHandler creates a new MCP handler with the given store.
func NewHandler(store *storage.Store) *Handler {
	return &Handler{store: store}
}

// WithEmbedder adds an embedding client for semantic search and auto-embedding.
func (h *Handler) WithEmbedder(client Embedder) *Handler {
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
	}
}

// CallTool executes the named tool with the given arguments.
func (h *Handler) CallTool(name string, args json.RawMessage) (*ToolCallResult, error) {
	switch name {
	case "create_entities":
		return h.createEntities(args)
	case "create_or_update_entities":
		return h.createOrUpdateEntities(args)
	case "create_relations":
		return h.createRelations(args)
	case "add_observations":
		return h.addObservations(args)
	case "delete_entities":
		return h.deleteEntities(args)
	case "delete_observations":
		return h.deleteObservations(args)
	case "delete_relations":
		return h.deleteRelations(args)
	case "read_graph":
		return h.readGraph()
	case "search_nodes":
		return h.searchNodes(args)
	case "open_nodes":
		return h.openNodes(args)
	case "get_context":
		return h.getContext(args)
	case "get_recent_context":
		return h.getRecentContext(args)
	case "summarize_entity":
		return h.summarizeEntity(args)
	case "consolidate_memories":
		return h.consolidateMemories(args)
	case "capture_session":
		return h.captureSession(args)
	case "recall_sessions":
		return h.recallSessions(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *Handler) createEntities(args json.RawMessage) (*ToolCallResult, error) {
	var input CreateEntitiesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var created []string
	for _, e := range input.Entities {
		entity, err := h.store.CreateEntity(e.Name, e.EntityType, e.Observations)
		if err != nil {
			// Entity may already exist, try adding observations
			for _, obs := range e.Observations {
				_ = h.store.AddObservation(e.Name, obs)
			}
		} else {
			created = append(created, entity.Name)
		}
		h.embedObservations(e.Name, e.Observations)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Created entities: %v", created)}},
	}, nil
}

func (h *Handler) createOrUpdateEntities(args json.RawMessage) (*ToolCallResult, error) {
	var input CreateEntitiesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var results []string
	for _, e := range input.Entities {
		entity, err := h.store.CreateOrUpdateEntity(e.Name, e.EntityType, e.Observations)
		if err != nil {
			results = append(results, fmt.Sprintf("Error: %s - %v", e.Name, err))
		} else {
			results = append(results, fmt.Sprintf("%s (v%d)", entity.Name, entity.Version))
			h.embedObservations(e.Name, e.Observations)
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Created/updated: %s", strings.Join(results, ", "))}},
	}, nil
}

func (h *Handler) createRelations(args json.RawMessage) (*ToolCallResult, error) {
	var input CreateRelationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var created int
	for _, r := range input.Relations {
		if err := h.store.CreateRelation(r.From, r.To, r.RelationType); err == nil {
			created++
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Created %d relations", created)}},
	}, nil
}

func (h *Handler) addObservations(args json.RawMessage) (*ToolCallResult, error) {
	var input AddObservationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var added int
	for _, obs := range input.Observations {
		// Determine fact type (default to dynamic for API compatibility)
		factType := storage.FactTypeDynamic
		if obs.FactType != "" {
			factType = storage.FactType(obs.FactType)
		}

		var addedContents []string
		for _, content := range obs.Contents {
			var err error
			if factType != storage.FactTypeDynamic {
				err = h.store.AddObservationWithType(obs.EntityName, content, factType)
			} else {
				err = h.store.AddObservation(obs.EntityName, content)
			}
			if err == nil {
				added++
				addedContents = append(addedContents, content)
			}
		}
		h.embedObservations(obs.EntityName, addedContents)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Added %d observations", added)}},
	}, nil
}

func (h *Handler) deleteEntities(args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteEntitiesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	for _, name := range input.EntityNames {
		if err := h.store.DeleteEntity(name); err == nil {
			deleted++
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Deleted %d entities", deleted)}},
	}, nil
}

func (h *Handler) deleteObservations(args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteObservationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	for _, d := range input.Deletions {
		for _, obs := range d.Observations {
			if err := h.store.DeleteObservation(d.EntityName, obs); err == nil {
				deleted++
			}
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Deleted %d observations", deleted)}},
	}, nil
}

func (h *Handler) deleteRelations(args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteRelationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	for _, r := range input.Relations {
		if err := h.store.DeleteRelation(r.From, r.To, r.RelationType); err == nil {
			deleted++
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Deleted %d relations", deleted)}},
	}, nil
}

func (h *Handler) readGraph() (*ToolCallResult, error) {
	graph, err := h.store.ReadGraph()
	if err != nil {
		return nil, fmt.Errorf("failed to read graph: %w", err)
	}

	data, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal graph: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (h *Handler) searchNodes(args json.RawMessage) (*ToolCallResult, error) {
	var input SearchNodesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Try hybrid search (FTS + vector) if embedder is a full EmbeddingClient
	if ec, ok := h.embedder.(*storage.EmbeddingClient); ok && ec != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		results, err := h.store.HybridSearchWithEmbedder(ctx, input.Query, ec, 20)
		if err == nil && len(results) > 0 {
			return h.formatHybridResults(results)
		}
		// Fall through to FTS-only on error
	}

	// Fallback: FTS-only search
	results, err := h.store.SearchWithLimit(input.Query, 20)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert to entity list for output
	entities := make([]map[string]any, len(results))
	for i, r := range results {
		entities[i] = map[string]any{
			"name":         r.Name,
			"entityType":   r.Type,
			"observations": r.Observations,
		}
	}

	data, err := json.Marshal(entities)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

// formatHybridResults converts FusedResults to MCP output format.
func (h *Handler) formatHybridResults(results []storage.FusedResult) (*ToolCallResult, error) {
	// Group results by entity to match expected output format
	entityMap := make(map[string]*struct {
		Name         string
		Type         string
		Observations []string
		Score        float64
	})

	for _, r := range results {
		key := r.EntityName
		if existing, ok := entityMap[key]; ok {
			// Add observation to existing entity
			existing.Observations = append(existing.Observations, r.Content)
			if r.FusionScore > existing.Score {
				existing.Score = r.FusionScore
			}
		} else {
			entityMap[key] = &struct {
				Name         string
				Type         string
				Observations []string
				Score        float64
			}{
				Name:         r.EntityName,
				Type:         r.EntityType,
				Observations: []string{r.Content},
				Score:        r.FusionScore,
			}
		}
	}

	// Convert to output format
	entities := make([]map[string]any, 0, len(entityMap))
	for _, e := range entityMap {
		entities = append(entities, map[string]any{
			"name":         e.Name,
			"entityType":   e.Type,
			"observations": e.Observations,
		})
	}

	data, err := json.Marshal(entities)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (h *Handler) openNodes(args json.RawMessage) (*ToolCallResult, error) {
	var input OpenNodesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var entities []map[string]any
	for _, name := range input.Names {
		entity, err := h.store.GetEntity(name)
		if err != nil {
			continue
		}
		entities = append(entities, map[string]any{
			"name":         entity.Name,
			"entityType":   entity.Type,
			"observations": entity.Observations,
		})
	}

	data, err := json.Marshal(entities)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entities: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (h *Handler) getRecentContext(args json.RawMessage) (*ToolCallResult, error) {
	var input GetRecentContextInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	hours := input.Hours
	if hours <= 0 {
		hours = 24
	}
	tokenBudget := input.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = 1000
	}

	results, err := h.store.GetRecentContext(hours, input.ProjectName, tokenBudget)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent context: %w", err)
	}

	formatted := storage.FormatContextResults(results)
	if formatted == "" {
		formatted = "No recent memories found."
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: formatted}},
	}, nil
}

func (h *Handler) summarizeEntity(args json.RawMessage) (*ToolCallResult, error) {
	var input SummarizeEntityInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	entity, err := h.store.GetEntity(input.EntityName)
	if err != nil {
		return nil, fmt.Errorf("entity not found: %w", err)
	}

	relations, _ := h.store.ListRelations(input.EntityName)
	history, _ := h.store.GetEntityHistory(input.EntityName)

	// Build summary
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s (%s)\n", entity.Name, entity.Type))
	sb.WriteString(fmt.Sprintf("Version: %d | Relations: %d\n\n", entity.Version, len(relations)))

	// Group observations by fact type
	if len(entity.Observations) > 0 {
		sb.WriteString("## Observations\n")
		for _, obs := range entity.Observations {
			sb.WriteString("- " + obs + "\n")
		}
		sb.WriteString("\n")
	}

	// Relations
	if len(relations) > 0 {
		sb.WriteString("## Relations\n")
		for _, r := range relations {
			sb.WriteString(fmt.Sprintf("- %s -[%s]-> %s\n", r.From, r.Type, r.To))
		}
		sb.WriteString("\n")
	}

	// Version history
	if len(history) > 1 {
		sb.WriteString(fmt.Sprintf("## History (%d versions)\n", len(history)))
		for _, v := range history {
			sb.WriteString(fmt.Sprintf("- v%d (created: %s)\n", v.Version, v.CreatedAt.Format("2006-01-02")))
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: sb.String()}},
	}, nil
}

func (h *Handler) consolidateMemories(args json.RawMessage) (*ToolCallResult, error) {
	var input ConsolidateMemoriesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	result, err := h.store.ConsolidateObservations(input.EntityName)
	if err != nil {
		return nil, fmt.Errorf("consolidation failed: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: result}},
	}, nil
}

// embedObservations generates and stores embeddings for the given observations.
// Fails silently if the embedder is not configured or embedding generation fails.
func (h *Handler) embedObservations(entityName string, contents []string) {
	if h.embedder == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, content := range contents {
		embedding, err := h.embedder.CreateEmbedding(ctx, content)
		if err != nil {
			continue // Degrade gracefully — don't fail the write operation
		}

		obs := h.store.GetObservationWithID(entityName, content)
		if obs == nil {
			continue
		}

		_ = h.store.StoreEmbedding(obs.ID, embedding, "nomic-embed-text")
	}
}

func (h *Handler) getContext(args json.RawMessage) (*ToolCallResult, error) {
	var input GetContextInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	cfg := storage.DefaultContextConfig()
	if input.TokenBudget > 0 {
		cfg.TokenBudget = input.TokenBudget
	}
	if input.MinImportance > 0 {
		cfg.MinImportance = input.MinImportance
	}

	results, err := h.store.GetContextForInjection(cfg, input.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get context: %w", err)
	}

	formatted := storage.FormatContextResults(results)
	if formatted == "" {
		formatted = "No relevant memories found."
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: formatted}},
	}, nil
}

func (h *Handler) captureSession(args json.RawMessage) (*ToolCallResult, error) {
	var input CaptureSessionInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	session, err := h.store.CreateSession(input.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	for _, evt := range input.Events {
		h.store.CaptureSessionEvent(session.Name, storage.SessionEvent{
			ToolName:  evt.ToolName,
			FilePath:  evt.FilePath,
			Command:   evt.Command,
			Timestamp: evt.Timestamp,
		})
	}

	if err := h.store.CompleteSession(session.Name, input.Summary); err != nil {
		return nil, fmt.Errorf("failed to complete session: %w", err)
	}

	// Auto-embed the summary
	h.embedObservations(session.Name, []string{input.Summary})

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Session captured: %s (%d events)", session.Name, len(input.Events))}},
	}, nil
}

func (h *Handler) recallSessions(args json.RawMessage) (*ToolCallResult, error) {
	var input RecallSessionsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	results, err := h.store.GetRecentSessionSummaries(input.ProjectName, input.Hours, input.TokenBudget)
	if err != nil {
		return nil, fmt.Errorf("failed to recall sessions: %w", err)
	}

	formatted := storage.FormatSessionRecall(results)
	if formatted == "" {
		formatted = "No recent sessions found."
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: formatted}},
	}, nil
}

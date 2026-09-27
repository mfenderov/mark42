package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mfenderov/mark42/internal/storage"
)

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
	if h.store != nil {
		h.store.WithEmbedder(client)
	}
	return h
}

// Tools returns the list of available memory tools.
func (h *Handler) Tools() []Tool {
	return []Tool{
		{
			Name:        "remember",
			Description: "WHEN: when you newly discover project conventions, architectural decisions, user preferences, or conclude a task. Store or update knowledge in memory under a topic. Call proactively whenever learning user preferences, personal facts, important decisions, rules, recurring patterns, or session milestones across any subject.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"topic": {
						Type:        "string",
						Description: "The subject, entity, person, project, or concept name (e.g. 'user-preferences', 'alice', 'travel-plans', 'auth-system')",
					},
					"facts": {
						Type:        "array",
						Description: "One or more statements, rules, or observations to record",
						Items:       &Items{Type: "string"},
					},
					"type": {
						Type:        "string",
						Description: "Optional category or entity type (e.g. 'person', 'preference', 'decision', 'project', 'concept'). Default: 'concept'",
					},
					"fact_type": {
						Type:        "string",
						Description: "Optional persistence level: 'static' (durable/evergreen fact or preference), 'dynamic' (active/changing state), or 'session' (conversation milestone/summary). Default: 'static'",
						Enum:        []string{"static", "dynamic", "session"},
					},
					"relations": {
						Type:        "array",
						Description: "Optional links to related topics",
						Items: &Items{
							Type: "object",
							Properties: map[string]Property{
								"to":   {Type: "string", Description: "Target topic name"},
								"type": {Type: "string", Description: "Relationship type (e.g. 'depends_on', 'implements', 'relates_to')"},
							},
							Required: []string{"to", "type"},
						},
					},
					"project": {
						Type:        "string",
						Description: "Optional namespace or container tag to scope this memory (e.g. workspace, project, or domain)",
					},
				},
				Required: []string{"topic", "facts"},
			},
		},
		{
			Name:        "recall",
			Description: "WHEN: at session start and before coding or answering, when you need prior decisions. Retrieve memories from mark42. Call without arguments at the start of a conversation to load core preferences, durable facts, and recent context. Provide 'query' to search across memories using semantic and keyword search, or 'topic' to inspect a specific subject (takes precedence over query).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {
						Type:        "string",
						Description: "Search query to find relevant memories, facts, preferences, or past discussions",
					},
					"topic": {
						Type:        "string",
						Description: "Specific topic, entity, person, or concept name to inspect in detail",
					},
					"project": {
						Type:        "string",
						Description: "Optional namespace, project, or container tag to scope the search or context",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of results to return (default: 10)",
					},
				},
			},
		},
		{
			Name:        "forget",
			Description: "WHEN: when information changes, is superseded, or is no longer true. Remove or invalidate knowledge. By default, soft-invalidates the fact (hiding it from future recall while preserving history). Set 'permanent: true' only to permanently delete. WARNING: Omitting 'fact' invalidates the ENTIRE topic.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"topic": {
						Type:        "string",
						Description: "Topic, entity, person, or concept name to forget from",
					},
					"fact": {
						Type:        "string",
						Description: "Exact text or substring of the specific fact to forget. If omitted, invalidates all facts under the entire topic.",
					},
					"permanent": {
						Type:        "boolean",
						Description: "Whether to permanently delete from SQLite (true) or soft-invalidate with valid_until timestamp (false). Default: false",
					},
				},
				Required: []string{"topic"},
			},
		},
	}
}

// toolDispatch maps tool names to handler methods (method expressions).
var toolDispatch = map[string]func(*Handler, context.Context, json.RawMessage) (*ToolCallResult, error){
	"remember": (*Handler).handleRemember,
	"recall":   (*Handler).handleRecall,
	"forget":   (*Handler).handleForget,
}

// CallToolContext executes the named tool with the given context and arguments.
func (h *Handler) CallToolContext(ctx context.Context, name string, args json.RawMessage) (*ToolCallResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if fn, ok := toolDispatch[name]; ok {
		return fn(h, ctx, args)
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

// CallTool executes the named tool with the given arguments.
func (h *Handler) CallTool(name string, args json.RawMessage) (*ToolCallResult, error) {
	return h.CallToolContext(context.Background(), name, args)
}

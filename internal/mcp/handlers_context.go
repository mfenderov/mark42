package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mfenderov/mark42/internal/storage"
)

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

	seen := make(map[string]struct{})
	for _, r := range results {
		if _, ok := seen[r.EntityName]; !ok {
			seen[r.EntityName] = struct{}{}
			if err := h.store.UpdateLastAccessed(r.EntityName); err != nil {
				logger.Warn("failed to update last accessed", "entity", r.EntityName, "error", err)
			}
		}
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

	results, err := h.store.GetContextForInjection(cfg, input.ProjectName, input.Query, h.embedder)
	if err != nil {
		return nil, fmt.Errorf("failed to get context: %w", err)
	}

	seen := make(map[string]struct{})
	for _, r := range results {
		if _, ok := seen[r.EntityName]; !ok {
			seen[r.EntityName] = struct{}{}
			if err := h.store.UpdateLastAccessed(r.EntityName); err != nil {
				logger.Warn("failed to update last accessed", "entity", r.EntityName, "error", err)
			}
		}
	}

	formatted := storage.FormatContextResults(results)
	if formatted == "" {
		formatted = "No relevant memories found."
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: formatted}},
	}, nil
}

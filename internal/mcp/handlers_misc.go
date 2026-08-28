package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

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

func (h *Handler) consolidateMemories(args json.RawMessage) (*ToolCallResult, error) {
	var input ConsolidateMemoriesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var result string
	var err error

	if input.Mode == "semantic" {
		if h.embedder == nil {
			return &ToolCallResult{
				Content: []ContentBlock{{Type: "text", Text: "semantic mode requires embedder"}},
				IsError: true,
			}, nil
		}
		threshold := input.Threshold
		if threshold == 0 {
			threshold = storage.DefaultSupersessionThreshold
		}
		result, err = h.store.ConsolidateWithSimilarity(input.EntityName, threshold)
	} else {
		result, err = h.store.ConsolidateObservations(input.EntityName)
	}

	if err != nil {
		return nil, fmt.Errorf("consolidation failed: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: result}},
	}, nil
}

func (h *Handler) invalidateObservation(args json.RawMessage) (*ToolCallResult, error) {
	var input InvalidateObservationInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if err := h.store.InvalidateObservation(input.EntityName, input.Content); err != nil {
		return nil, fmt.Errorf("invalidate failed: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: `{"status":"invalidated"}`}},
	}, nil
}

func (h *Handler) getEntityHistory(args json.RawMessage) (*ToolCallResult, error) {
	var input GetEntityHistoryInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	history, err := h.store.GetObservationHistory(input.EntityName)
	if err != nil {
		return nil, fmt.Errorf("get history failed: %w", err)
	}

	type historyEntry struct {
		Content    string  `json:"content"`
		FactType   string  `json:"factType"`
		ValidFrom  string  `json:"validFrom"`
		ValidUntil *string `json:"validUntil"`
	}

	entries := make([]historyEntry, len(history))
	for i, h := range history {
		entry := historyEntry{
			Content:   h.Content,
			FactType:  h.FactType,
			ValidFrom: h.ValidFrom().Format(time.RFC3339),
		}
		if h.ValidUntil.Valid {
			var validUntilTime time.Time
			if t, err := time.Parse("2006-01-02 15:04:05", h.ValidUntil.String); err == nil {
				validUntilTime = t
			} else if t, err := time.Parse(time.RFC3339, h.ValidUntil.String); err == nil {
				validUntilTime = t
			}
			if !validUntilTime.IsZero() {
				s := validUntilTime.Format(time.RFC3339)
				entry.ValidUntil = &s
			}
		}
		entries[i] = entry
	}

	data, err := json.Marshal(map[string]any{
		"entityName": input.EntityName,
		"history":    entries,
		"count":      len(entries),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal history: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func isSessionFactType(ft storage.FactType) bool {
	return ft == storage.FactTypeSessionEvent ||
		ft == storage.FactTypeSessionSummary ||
		ft == storage.FactTypeSessionTurn
}

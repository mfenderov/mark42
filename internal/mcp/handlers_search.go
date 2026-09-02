package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

const (
	maxObservationsPerEntity = 3
	maxObservationLength     = 240
)

func (h *Handler) searchNodes(args json.RawMessage) (*ToolCallResult, error) {
	var input SearchNodesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Try hybrid search (FTS + vector) if embedder is available
	if h.embedder != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		results, err := h.store.HybridSearchWithEmbedder(ctx, input.Query, h.embedder, 20)
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
		obs, err := h.store.TopObservations(r.ID, maxObservationsPerEntity)
		if err != nil {
			return nil, fmt.Errorf("failed to load observations: %w", err)
		}
		entities[i] = map[string]any{
			"name":         r.Name,
			"entityType":   r.Type,
			"observations": truncateObservations(obs),
		}
		if err := h.store.UpdateLastAccessed(r.Name); err != nil {
			logger.Warn("failed to update last accessed", "entity", r.Name, "error", err)
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

func truncateObservations(obs []string) []string {
	out := make([]string, len(obs))
	for i, o := range obs {
		out[i] = truncateObservation(o)
	}
	return out
}

func (h *Handler) filterSessionEvents(entityName string, observations []string) []string {
	events, err := h.store.GetSessionEventObservations(entityName)
	if err != nil || len(events) == 0 {
		return observations
	}
	eventSet := make(map[string]bool, len(events))
	for _, c := range events {
		eventSet[c] = true
	}
	filtered := make([]string, 0, len(observations))
	for _, o := range observations {
		if !eventSet[o] {
			filtered = append(filtered, o)
		}
	}
	return filtered
}

func truncateObservation(s string) string {
	r := []rune(s)
	if len(r) <= maxObservationLength {
		return s
	}
	return string(r[:maxObservationLength]) + "…"
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
			if len(existing.Observations) < maxObservationsPerEntity {
				existing.Observations = append(existing.Observations, truncateObservation(r.Content))
			}
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
				Observations: []string{truncateObservation(r.Content)},
				Score:        r.FusionScore,
			}
		}
	}

	// Convert to output format, tracking access per entity
	entities := make([]map[string]any, 0, len(entityMap))
	for _, e := range entityMap {
		entities = append(entities, map[string]any{
			"name":         e.Name,
			"entityType":   e.Type,
			"observations": e.Observations,
		})
		if err := h.store.UpdateLastAccessed(e.Name); err != nil {
			logger.Warn("failed to update last accessed", "entity", e.Name, "error", err)
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
			"observations": h.filterSessionEvents(name, entity.Observations),
		})
		if err := h.store.UpdateLastAccessed(name); err != nil {
			logger.Warn("failed to update last accessed", "entity", name, "error", err)
		}
	}

	data, err := json.Marshal(entities)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entities: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

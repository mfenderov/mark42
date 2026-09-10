package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

func (h *Handler) createEntities(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
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
		h.embedObservations(ctx, e.Name, e.Observations)
		if e.EntityType != "session" {
			h.autoDetectSuperseded(ctx, e.Name, e.Observations, storage.FactTypeDynamic)
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Created entities: %v", created)}},
	}, nil
}

func (h *Handler) createOrUpdateEntities(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
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
			h.embedObservations(ctx, e.Name, e.Observations)
			if e.EntityType != "session" {
				h.autoDetectSuperseded(ctx, e.Name, e.Observations, storage.FactTypeDynamic)
			}
		}
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Created/updated: %s", strings.Join(results, ", "))}},
	}, nil
}

func (h *Handler) addObservations(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input AddObservationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var added int
	var errs []string
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
			} else {
				errs = append(errs, fmt.Sprintf("%s: %q: %v", obs.EntityName, content, err))
			}
		}
		h.embedObservations(ctx, obs.EntityName, addedContents)
		h.autoDetectSuperseded(ctx, obs.EntityName, addedContents, factType)
	}

	msg := fmt.Sprintf("Added %d observations", added)
	if len(errs) > 0 {
		msg += fmt.Sprintf(" (failed: %s)", strings.Join(errs, "; "))
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

func (h *Handler) deleteEntities(_ context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteEntitiesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	var errs []string
	for _, name := range input.EntityNames {
		if err := h.store.DeleteEntity(name); err == nil {
			deleted++
		} else {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	msg := fmt.Sprintf("Deleted %d entities", deleted)
	if len(errs) > 0 {
		msg += fmt.Sprintf(" (failed: %s)", strings.Join(errs, "; "))
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

func (h *Handler) deleteObservations(_ context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteObservationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	var errs []string
	for _, d := range input.Deletions {
		for _, obs := range d.Observations {
			if err := h.store.DeleteObservation(d.EntityName, obs); err == nil {
				deleted++
			} else {
				errs = append(errs, fmt.Sprintf("%s: %q: %v", d.EntityName, obs, err))
			}
		}
	}

	msg := fmt.Sprintf("Deleted %d observations", deleted)
	if len(errs) > 0 {
		msg += fmt.Sprintf(" (failed: %s)", strings.Join(errs, "; "))
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

func (h *Handler) autoDetectSuperseded(ctx context.Context, entityName string, contents []string, factType storage.FactType) {
	if h.embedder == nil || ctx.Err() != nil {
		return
	}
	if isSessionFactType(factType) {
		return
	}
	expired, err := h.store.DetectAndExpireSupersededBatch(entityName, contents, h.embedder, storage.DefaultSupersessionThreshold)
	if err != nil {
		logger.Warn("failed to detect superseded observations", "entity", entityName, "error", err)
	} else if len(expired) > 0 {
		logger.Info("auto-expired superseded observations", "entity", entityName, "count", len(expired))
	}
}

func (h *Handler) embedObservations(ctx context.Context, entityName string, contents []string) {
	if h.embedder == nil || ctx.Err() != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	loggedWarning := false
	for _, content := range contents {
		if ctx.Err() != nil {
			return
		}
		embedding, err := h.embedder.CreateEmbedding(ctx, content)
		if err != nil {
			if !loggedWarning {
				logger.Warn("embedding failed, semantic search degraded",
					"entity", entityName, "error", err)
				loggedWarning = true
			}
			continue
		}

		obs := h.store.GetObservationWithID(entityName, content)
		if obs == nil {
			continue
		}

		_ = h.store.StoreEmbedding(obs.ID, embedding, "nomic-embed-text")
	}
}

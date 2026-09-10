package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (h *Handler) createRelations(_ context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input CreateRelationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var created int
	var errs []string
	for _, r := range input.Relations {
		if err := h.store.CreateRelation(r.From, r.To, r.RelationType); err == nil {
			created++
		} else {
			errs = append(errs, fmt.Sprintf("%s -> %s (%s): %v", r.From, r.To, r.RelationType, err))
		}
	}

	msg := fmt.Sprintf("Created %d relations", created)
	if len(errs) > 0 {
		msg += fmt.Sprintf(" (failed: %s)", strings.Join(errs, "; "))
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

func (h *Handler) deleteRelations(_ context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input DeleteRelationsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	var deleted int
	var errs []string
	for _, r := range input.Relations {
		if err := h.store.DeleteRelation(r.From, r.To, r.RelationType); err == nil {
			deleted++
		} else {
			errs = append(errs, fmt.Sprintf("%s -> %s (%s): %v", r.From, r.To, r.RelationType, err))
		}
	}

	msg := fmt.Sprintf("Deleted %d relations", deleted)
	if len(errs) > 0 {
		msg += fmt.Sprintf(" (failed: %s)", strings.Join(errs, "; "))
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

package mcp

import (
	"encoding/json"
	"fmt"
)

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

package mcp

import (
	"encoding/json"
	"fmt"
)

func (h *Handler) getMemoryAnalytics(args json.RawMessage) (*ToolCallResult, error) {
	var input GetMemoryAnalyticsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	topN := input.TopN
	if topN <= 0 {
		topN = 10
	}

	analytics, err := h.store.GetMemoryAnalytics(topN)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory analytics: %w", err)
	}

	data, err := json.Marshal(analytics)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analytics: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (h *Handler) getTuningRecommendation() (*ToolCallResult, error) {
	recommendation, err := h.store.RecommendTuning()
	if err != nil {
		return nil, fmt.Errorf("failed to get tuning recommendation: %w", err)
	}

	data, err := json.Marshal(recommendation)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tuning recommendation: %w", err)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

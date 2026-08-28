package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mfenderov/mark42/internal/storage"
)

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
		_ = h.store.CaptureSessionEvent(session.Name, storage.SessionEvent{
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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mfenderov/mark42/internal/storage"
)

// RememberInput defines JSON arguments for the remember tool.
type RememberInput struct {
	Topic     string                  `json:"topic"`
	Facts     []string                `json:"facts"`
	Type      string                  `json:"type,omitempty"`
	FactType  string                  `json:"fact_type,omitempty"`
	Relations []storage.RelationParam `json:"relations,omitempty"`
	Project   string                  `json:"project,omitempty"`
}

// RecallInput defines JSON arguments for the recall tool.
type RecallInput struct {
	Query   string `json:"query,omitempty"`
	Topic   string `json:"topic,omitempty"`
	Project string `json:"project,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// ForgetInput defines JSON arguments for the forget tool.
type ForgetInput struct {
	Topic     string `json:"topic"`
	Fact      string `json:"fact,omitempty"`
	Permanent bool   `json:"permanent,omitempty"`
}

func (h *Handler) handleRemember(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input RememberInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(input.Topic) == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if len(input.Facts) == 0 {
		return nil, fmt.Errorf("at least one fact is required")
	}

	factType := storage.FactTypeStatic
	if input.FactType != "" {
		factType = storage.FactType(input.FactType)
	}

	res, err := h.store.Remember(ctx, storage.RememberParams{
		Topic:      input.Topic,
		Facts:      input.Facts,
		EntityType: input.Type,
		FactType:   factType,
		Relations:  input.Relations,
		Project:    input.Project,
	})
	if err != nil {
		return nil, err
	}

	msg := fmt.Sprintf("Remembered %d fact(s) under %q", res.ObservationsAdded, res.Topic)
	if res.RelationsCreated > 0 {
		msg += fmt.Sprintf(" with %d relation(s)", res.RelationsCreated)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

func (h *Handler) handleRecall(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input RecallInput
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	res, err := h.store.Recall(ctx, storage.RecallParams{
		Query:   input.Query,
		Topic:   input.Topic,
		Project: input.Project,
		Limit:   input.Limit,
	})
	if err != nil {
		return nil, err
	}

	var text string
	switch res.Type {
	case "topic":
		text = formatRecallTopic(input.Topic, res)
	case "search":
		text = formatRecallSearch(input.Query, res)
	case "context":
		text = formatRecallContext(res)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: strings.TrimSpace(text)}},
	}, nil
}

func formatRecallTopic(topicName string, res *storage.RecallResult) string {
	if res.Topic == nil {
		return fmt.Sprintf("Topic %q not found", topicName)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Topic: %s (%s)\n\n", res.Topic.Name, res.Topic.Type))
	sb.WriteString("## Facts\n")
	if len(res.Topic.Observations) == 0 {
		sb.WriteString("(no active facts)\n")
	} else {
		for _, obs := range res.Topic.Observations {
			sb.WriteString(fmt.Sprintf("- %s\n", obs))
		}
	}
	if len(res.Relations) > 0 {
		sb.WriteString("\n## Relations\n")
		for _, rel := range res.Relations {
			sb.WriteString(fmt.Sprintf("- %s --[%s]--> %s\n", rel.From, rel.Type, rel.To))
		}
	}
	return sb.String()
}

func formatRecallSearch(query string, res *storage.RecallResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Search Results for %q\n\n", query))
	if len(res.SearchResults) == 0 {
		sb.WriteString("No matching memories found.\n")
	} else {
		for _, item := range res.SearchResults {
			sb.WriteString(fmt.Sprintf("- **[%s]** %s\n", item.EntityName, item.Content))
		}
	}
	return sb.String()
}

func formatRecallContext(res *storage.RecallResult) string {
	var sb strings.Builder
	sb.WriteString("# Project Context\n\n")
	if len(res.Context) == 0 && len(res.Sessions) == 0 {
		sb.WriteString("No memories found for this project.\n")
		return sb.String()
	}
	if len(res.Context) > 0 {
		sb.WriteString("## Core Rules & Conventions\n")
		for _, c := range res.Context {
			sb.WriteString(fmt.Sprintf("- **[%s]** %s\n", c.EntityName, c.Content))
		}
	}
	if len(res.Sessions) > 0 {
		sb.WriteString("\n## Recent Session Summaries\n")
		for _, s := range res.Sessions {
			sb.WriteString(fmt.Sprintf("- **[%s]** %s\n", s.EntityName, s.Content))
		}
	}
	return sb.String()
}

func (h *Handler) handleForget(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	var input ForgetInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(input.Topic) == "" {
		return nil, fmt.Errorf("topic is required")
	}

	res, err := h.store.Forget(ctx, storage.ForgetParams{
		Topic:     input.Topic,
		Fact:      input.Fact,
		Permanent: input.Permanent,
	})
	if err != nil {
		return nil, err
	}

	action := "Invalidated"
	if res.Permanent {
		action = "Permanently deleted"
	}

	var msg string
	if input.Fact != "" {
		msg = fmt.Sprintf("%s fact from %q", action, res.Topic)
	} else {
		msg = fmt.Sprintf("%s %d item(s) from topic %q", action, res.Count, res.Topic)
	}

	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: msg}},
	}, nil
}

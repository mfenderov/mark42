package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/mcp"
	"github.com/mfenderov/mark42/internal/storage"
)

// newTestHandler creates a handler with a fresh test store
func newTestHandler(t *testing.T) (*mcp.Handler, *storage.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	handler := mcp.NewHandler(store)
	return handler, store
}

// --- Tools() tests ---

func TestHandler_Tools(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tools := handler.Tools()

	expectedTools := []string{
		"remember",
		"recall",
		"forget",
	}

	if len(tools) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d", len(expectedTools), len(tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("expected tool %q not found", expected)
		}
	}
}

func TestHandler_ToolsSchemaConstraints(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tools := handler.Tools()
	toolMap := make(map[string]mcp.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	// Verify remember requires topic and facts
	rememberTool, ok := toolMap["remember"]
	if !ok {
		t.Fatal("tool remember not found")
	}
	reqRemember := strings.Join(rememberTool.InputSchema.Required, ",")
	if !strings.Contains(reqRemember, "topic") || !strings.Contains(reqRemember, "facts") {
		t.Errorf("expected remember to require topic and facts, got %v", rememberTool.InputSchema.Required)
	}

	// Verify forget requires topic
	forgetTool, ok := toolMap["forget"]
	if !ok {
		t.Fatal("tool forget not found")
	}
	reqForget := strings.Join(forgetTool.InputSchema.Required, ",")
	if !strings.Contains(reqForget, "topic") {
		t.Errorf("expected forget to require topic, got %v", forgetTool.InputSchema.Required)
	}
}

// --- CallTool unknown tool test ---

func TestHandler_CallTool_UnknownTool(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	_, err := handler.CallTool("nonexistent_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected 'unknown tool' error, got: %v", err)
	}
}

// --- Remember, Recall, Forget End-to-End Tests ---

func TestHandler_RememberRecallForget_Workflow(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()
	ctx := context.Background()

	// 1. Remember with relations
	remArgs := `{
		"topic": "auth-service",
		"facts": ["Uses JWT authentication", "Token TTL is 15 minutes"],
		"type": "architecture",
		"relations": [
			{"to": "user-db", "type": "connects_to"}
		],
		"project": "mark42"
	}`
	remRes, err := handler.CallToolContext(ctx, "remember", json.RawMessage(remArgs))
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if remRes.IsError {
		t.Fatalf("remember returned error: %s", remRes.Content[0].Text)
	}
	if !strings.Contains(remRes.Content[0].Text, "auth-service") {
		t.Errorf("expected text to mention topic, got: %s", remRes.Content[0].Text)
	}

	// 2. Recall by Topic
	recTopicArgs := `{"topic": "auth-service"}`
	recTopicRes, err := handler.CallToolContext(ctx, "recall", json.RawMessage(recTopicArgs))
	if err != nil {
		t.Fatalf("recall topic failed: %v", err)
	}
	topicText := recTopicRes.Content[0].Text
	if !strings.Contains(topicText, "Uses JWT authentication") || !strings.Contains(topicText, "user-db") {
		t.Errorf("expected topic recall to include facts and relations, got: %s", topicText)
	}

	// 3. Recall by Query
	recQueryArgs := `{"query": "JWT authentication"}`
	recQueryRes, err := handler.CallToolContext(ctx, "recall", json.RawMessage(recQueryArgs))
	if err != nil {
		t.Fatalf("recall query failed: %v", err)
	}
	queryText := recQueryRes.Content[0].Text
	if !strings.Contains(queryText, "auth-service") {
		t.Errorf("expected query recall to find auth-service, got: %s", queryText)
	}

	// 4. Recall Default Context
	recCtxRes, err := handler.CallToolContext(ctx, "recall", json.RawMessage(`{"project": "mark42"}`))
	if err != nil {
		t.Fatalf("recall context failed: %v", err)
	}
	if len(recCtxRes.Content) == 0 {
		t.Fatalf("expected content in context recall")
	}

	// 5. Forget Fact (soft invalidate)
	forgetFactArgs := `{"topic": "auth-service", "fact": "Token TTL is 15 minutes"}`
	forgetRes, err := handler.CallToolContext(ctx, "forget", json.RawMessage(forgetFactArgs))
	if err != nil {
		t.Fatalf("forget fact failed: %v", err)
	}
	if !strings.Contains(forgetRes.Content[0].Text, "Invalidated") {
		t.Errorf("expected output to mention Invalidated, got: %s", forgetRes.Content[0].Text)
	}

	// Verify forgotten fact is omitted
	recTopicRes2, _ := handler.CallToolContext(ctx, "recall", json.RawMessage(recTopicArgs))
	if strings.Contains(recTopicRes2.Content[0].Text, "Token TTL is 15 minutes") {
		t.Errorf("expected forgotten fact to be omitted, got: %s", recTopicRes2.Content[0].Text)
	}

	// 6. Forget Entire Topic (permanent)
	forgetTopicArgs := `{"topic": "auth-service", "permanent": true}`
	forgetTopicRes, err := handler.CallToolContext(ctx, "forget", json.RawMessage(forgetTopicArgs))
	if err != nil {
		t.Fatalf("forget topic permanently failed: %v", err)
	}
	if !strings.Contains(forgetTopicRes.Content[0].Text, "Permanently deleted") {
		t.Errorf("expected output to mention Permanently deleted, got: %s", forgetTopicRes.Content[0].Text)
	}
}

func TestHandler_ValidationErrors(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()
	ctx := context.Background()

	// Missing topic in remember
	_, err := handler.CallToolContext(ctx, "remember", json.RawMessage(`{"facts": ["fact 1"]}`))
	if err == nil {
		t.Error("expected error for missing topic in remember")
	}

	// Empty facts in remember
	_, err = handler.CallToolContext(ctx, "remember", json.RawMessage(`{"topic": "test", "facts": []}`))
	if err == nil {
		t.Error("expected error for empty facts in remember")
	}

	// Missing topic in forget
	_, err = handler.CallToolContext(ctx, "forget", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing topic in forget")
	}
}

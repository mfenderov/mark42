package mcp_test

import (
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/mcp"
)

// RED: tool descriptions must tell the model WHEN to call them.
// Without triggers the model skips memory unless prompted.
func TestHandler_Tools_DescriptionsHaveUsageTriggers(t *testing.T) {
	handler, store := newTestHandler(t)
	defer store.Close()

	tools := handler.Tools()
	toolMap := make(map[string]string)
	for _, tool := range tools {
		toolMap[tool.Name] = tool.Description
	}

	triggers := map[string][]string{
		"get_context":      {"session start"},
		"recall_sessions":  {"session start", "previous"},
		"search_nodes":     {"before"},
		"add_observations": {"newly", "discover"},
		"capture_session":  {"conclud", "end"},
	}

	for name, keywords := range triggers {
		desc, ok := toolMap[name]
		if !ok {
			t.Fatalf("tool %q not found", name)
		}
		lower := strings.ToLower(desc)
		matched := false
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("tool %q description lacks usage trigger (want one of %v), got: %q", name, keywords, desc)
		}
	}
}

// RED: server must advertise global memory instructions on initialize.
// Without this the model never learns the recall-then-save loop.
func TestInitializeResult_HasInstructions(t *testing.T) {
	result := mcp.InitializeResult{Instructions: mcp.ServerInstructions()}
	if result.Instructions == "" {
		t.Error("InitializeResult.Instructions is empty, want non-empty memory loop instructions")
		return
	}
	lower := strings.ToLower(result.Instructions)
	for _, kw := range []string{"get_context", "recall_sessions", "capture_session"} {
		if !strings.Contains(lower, kw) {
			t.Errorf("Instructions lacks %q, got: %q", kw, result.Instructions)
		}
	}
}

func TestServerInstructions_Content(t *testing.T) {
	text := mcp.ServerInstructions()
	lower := strings.ToLower(text)
	for _, kw := range []string{"get_context", "recall_sessions", "search_nodes", "add_observations", "capture_session"} {
		if !strings.Contains(lower, kw) {
			t.Errorf("ServerInstructions lacks %q, got: %q", kw, text)
		}
	}
}

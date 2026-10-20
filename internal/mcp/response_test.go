package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/mcp"
)

func TestResponse_SerializesNullIDWhenNil(t *testing.T) {
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      nil,
		Error: &mcp.Error{
			Code:    mcp.ErrCodeParse,
			Message: "Parse error",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, `"id":null`) {
		t.Errorf("expected response to contain `\"id\":null`, got: %s", output)
	}
}

func TestResponse_SerializesValueID(t *testing.T) {
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      "req-42",
		Result:  map[string]any{"ok": true},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, `"id":"req-42"`) {
		t.Errorf("expected response to contain `\"id\":\"req-42\"`, got: %s", output)
	}
}

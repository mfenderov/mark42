package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mfenderov/mark42/internal/mcp"
	"github.com/mfenderov/mark42/internal/storage"
)

func newTestHandler(t *testing.T) *mcp.Handler {
	t.Helper()
	store, err := storage.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return mcp.NewHandler(store)
}

func TestServer_StopsOnContextCancel(t *testing.T) {
	handler := newTestHandler(t)
	server := &Server{handler: handler}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestServer_NotificationsNeverReceiveResponse(t *testing.T) {
	handler := newTestHandler(t)
	var outBuf bytes.Buffer
	input := `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"custom/notification"}` + "\n"

	server := &Server{
		handler: handler,
		in:      strings.NewReader(input),
		out:     &outBuf,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := server.Run(ctx); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if outBuf.Len() > 0 {
		t.Errorf("expected zero responses for notifications, got: %s", outBuf.String())
	}
}

func TestServer_ToolCallAsNotificationExecutesWithoutResponse(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewStore(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	handler := mcp.NewHandler(store)
	var outBuf bytes.Buffer
	input := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"create_entities","arguments":{"entities":[{"name":"NotificationEntity","entityType":"test","observations":["created via notification"]}]}}}` + "\n"

	server := &Server{
		handler: handler,
		in:      strings.NewReader(input),
		out:     &outBuf,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := server.Run(ctx); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 1. Must NOT emit any response to stdout
	if outBuf.Len() > 0 {
		t.Errorf("expected zero responses for notification tools/call, got: %s", outBuf.String())
	}

	// 2. But the tool MUST have executed and created the entity in the database
	entity, err := store.GetEntity("NotificationEntity")
	if err != nil {
		t.Fatalf("expected entity to be created by notification tools/call, got err: %v", err)
	}
	if entity.Name != "NotificationEntity" {
		t.Errorf("expected entity name NotificationEntity, got: %s", entity.Name)
	}
}

func TestServer_InvalidRequestResponses(t *testing.T) {
	handler := newTestHandler(t)

	tests := []struct {
		name         string
		input        string
		expectedCode int
		expectedID   string
	}{
		{
			name:         "invalid json syntax -> parse error",
			input:        `{"jsonrpc":"2.0", broken}` + "\n",
			expectedCode: mcp.ErrCodeParse,
			expectedID:   `"id":null`,
		},
		{
			name:         "json array instead of request object -> invalid request",
			input:        `[1, 2, 3]` + "\n",
			expectedCode: mcp.ErrCodeInvalidRequest,
			expectedID:   `"id":null`,
		},
		{
			name:         "missing jsonrpc version -> invalid request",
			input:        `{"method":"tools/list"}` + "\n",
			expectedCode: mcp.ErrCodeInvalidRequest,
			expectedID:   `"id":null`,
		},
		{
			name:         "wrong jsonrpc version with id -> invalid request with id",
			input:        `{"jsonrpc":"1.0","id":42,"method":"tools/list"}` + "\n",
			expectedCode: mcp.ErrCodeInvalidRequest,
			expectedID:   `"id":42`,
		},
		{
			name:         "empty method -> invalid request",
			input:        `{"jsonrpc":"2.0","method":""}` + "\n",
			expectedCode: mcp.ErrCodeInvalidRequest,
			expectedID:   `"id":null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outBuf bytes.Buffer
			server := &Server{
				handler: handler,
				in:      strings.NewReader(tt.input),
				out:     &outBuf,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			if err := server.Run(ctx); err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			output := strings.TrimSpace(outBuf.String())
			if output == "" {
				t.Fatalf("expected error response, got empty output")
			}

			var resp mcp.Response
			if err := json.Unmarshal([]byte(output), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v, raw: %s", err, output)
			}

			if resp.Error == nil {
				t.Fatalf("expected error in response, got nil: %s", output)
			}
			if resp.Error.Code != tt.expectedCode {
				t.Errorf("expected error code %d, got %d", tt.expectedCode, resp.Error.Code)
			}
			if !strings.Contains(output, tt.expectedID) {
				t.Errorf("expected output to contain %s, got: %s", tt.expectedID, output)
			}
		})
	}
}

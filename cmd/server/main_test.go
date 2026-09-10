package main

import (
	"bytes"
	"context"
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

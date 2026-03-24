package main

import (
	"context"
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

package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddingClient_CreateEmbedding(t *testing.T) {
	// Mock DMR server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected path /embeddings, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Return mock embedding response (OpenAI format)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"object": "list",
			"data": [{
				"object": "embedding",
				"index": 0,
				"embedding": [0.1, 0.2, 0.3, 0.4, 0.5]
			}],
			"model": "nomic-embed-text",
			"usage": {
				"prompt_tokens": 5,
				"total_tokens": 5
			}
		}`))
	}))
	defer server.Close()

	client := NewEmbeddingClient(server.URL)
	ctx := context.Background()

	embedding, err := client.CreateEmbedding(ctx, "test text")
	if err != nil {
		t.Fatalf("CreateEmbedding failed: %v", err)
	}

	if len(embedding) != 5 {
		t.Errorf("expected 5 dimensions, got %d", len(embedding))
	}

	expected := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	for i, v := range embedding {
		if v != expected[i] {
			t.Errorf("embedding[%d] = %f, expected %f", i, v, expected[i])
		}
	}
}

func TestEmbeddingClient_CreateBatchEmbedding(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Return different embeddings for batch
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{"object": "embedding", "index": 0, "embedding": [0.1, 0.2, 0.3]},
				{"object": "embedding", "index": 1, "embedding": [0.4, 0.5, 0.6]},
				{"object": "embedding", "index": 2, "embedding": [0.7, 0.8, 0.9]}
			],
			"model": "nomic-embed-text",
			"usage": {"prompt_tokens": 15, "total_tokens": 15}
		}`))
	}))
	defer server.Close()

	client := NewEmbeddingClient(server.URL)
	ctx := context.Background()

	texts := []string{"first", "second", "third"}
	embeddings, err := client.CreateBatchEmbedding(ctx, texts)
	if err != nil {
		t.Fatalf("CreateBatchEmbedding failed: %v", err)
	}

	if len(embeddings) != 3 {
		t.Fatalf("expected 3 embeddings, got %d", len(embeddings))
	}

	// Verify embeddings are in correct order
	if embeddings[0][0] != 0.1 {
		t.Errorf("first embedding[0] = %f, expected 0.1", embeddings[0][0])
	}
	if embeddings[1][0] != 0.4 {
		t.Errorf("second embedding[0] = %f, expected 0.4", embeddings[1][0])
	}
	if embeddings[2][0] != 0.7 {
		t.Errorf("third embedding[0] = %f, expected 0.7", embeddings[2][0])
	}

	// Should be a single batch call
	if callCount != 1 {
		t.Errorf("expected 1 API call for batch, got %d", callCount)
	}
}

func TestEmbeddingClient_EmptyInput(t *testing.T) {
	client := NewEmbeddingClient("http://localhost:12434/engines/v1")
	ctx := context.Background()

	// Empty text should still work (API handles it)
	_, err := client.CreateEmbedding(ctx, "")
	// We expect an error for empty input
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestEmbeddingClient_BatchEmptySlice(t *testing.T) {
	client := NewEmbeddingClient("http://localhost:12434/engines/v1")
	ctx := context.Background()

	embeddings, err := client.CreateBatchEmbedding(ctx, []string{})
	if err != nil {
		t.Fatalf("unexpected error for empty batch: %v", err)
	}
	if len(embeddings) != 0 {
		t.Errorf("expected 0 embeddings for empty input, got %d", len(embeddings))
	}
}

func TestDefaultDMRBaseURL(t *testing.T) {
	url := DefaultDMRBaseURL()
	expected := "http://127.0.0.1:12434/engines/v1"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestDefaultOllamaBaseURL(t *testing.T) {
	url := DefaultOllamaBaseURL()
	expected := "http://localhost:11434/v1"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestNewOllamaEmbeddingClient(t *testing.T) {
	client := NewOllamaEmbeddingClient()
	if client.baseURL != DefaultOllamaBaseURL() {
		t.Errorf("expected Ollama base URL, got %q", client.baseURL)
	}
}

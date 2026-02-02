package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
)

// EmbeddingClient handles embedding generation via DMR (Docker Model Runner).
// Uses OpenAI-compatible API at http://127.0.0.1:12434/engines/v1/
type EmbeddingClient struct {
	baseURL    string
	httpClient *http.Client
	model      string
}

// DefaultDMRBaseURL returns the default DMR API endpoint (Docker Desktop).
func DefaultDMRBaseURL() string {
	return "http://127.0.0.1:12434/engines/v1"
}

// DefaultOllamaBaseURL returns the default Ollama API endpoint.
func DefaultOllamaBaseURL() string {
	return "http://localhost:11434/v1"
}

// NewEmbeddingClient creates an embedding client for the given base URL.
func NewEmbeddingClient(baseURL string) *EmbeddingClient {
	return &EmbeddingClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
		model:      "nomic-embed-text",
	}
}

// NewDMREmbeddingClient creates an embedding client using DMR (requires Docker Desktop).
func NewDMREmbeddingClient() *EmbeddingClient {
	return NewEmbeddingClient(DefaultDMRBaseURL())
}

// NewOllamaEmbeddingClient creates an embedding client using Ollama (no Docker required).
func NewOllamaEmbeddingClient() *EmbeddingClient {
	return NewEmbeddingClient(DefaultOllamaBaseURL())
}

// embeddingRequest is the OpenAI-compatible embedding request format.
type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// embeddingResponse is the OpenAI-compatible embedding response format.
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// CreateEmbedding generates an embedding for a single text.
func (c *EmbeddingClient) CreateEmbedding(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, errors.New("empty text input")
	}

	embeddings, err := c.CreateBatchEmbedding(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, errors.New("no embedding returned")
	}

	return embeddings[0], nil
}

// CreateBatchEmbedding generates embeddings for multiple texts in a single API call.
func (c *EmbeddingClient) CreateBatchEmbedding(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	reqBody := embeddingRequest{
		Input: texts,
		Model: c.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Sort by index to ensure correct order
	sort.Slice(embResp.Data, func(i, j int) bool {
		return embResp.Data[i].Index < embResp.Data[j].Index
	})

	embeddings := make([][]float64, len(embResp.Data))
	for i, d := range embResp.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// SetModel changes the embedding model (default: nomic-embed-text).
func (c *EmbeddingClient) SetModel(model string) {
	c.model = model
}

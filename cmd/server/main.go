package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mfenderov/mark42/internal/mcp"
	"github.com/mfenderov/mark42/internal/storage"
)

var Version = "dev"

func main() {
	// Determine database path
	dbPath := os.Getenv("CLAUDE_MEMORY_DB")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".claude", "memory.db")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		logError("failed to create database directory: %v", err)
		os.Exit(1)
	}

	// Open storage
	store, err := storage.NewStore(dbPath)
	if err != nil {
		logError("failed to open database: %v", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create handler
	handler := mcp.NewHandler(store)

	// Optionally enable semantic search with embeddings
	embedderURL := os.Getenv("CLAUDE_MEMORY_EMBEDDER_URL")
	if embedderURL == "" {
		embedderURL = storage.DefaultOllamaBaseURL() // Try Ollama by default
	}
	if embedderURL != "disabled" {
		embedder := storage.NewEmbeddingClient(embedderURL)
		handler.WithEmbedder(embedder)
	}

	// Run server
	server := &Server{handler: handler}
	if err := server.Run(); err != nil {
		logError("server error: %v", err)
		os.Exit(1)
	}
}

// Server handles MCP JSON-RPC communication over stdio.
type Server struct {
	handler     *mcp.Handler
	initialized bool
}

// Run starts the server's main loop.
func (s *Server) Run() error {
	scanner := bufio.NewScanner(os.Stdin)

	// Increase buffer size for large requests
	const maxScannerSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxScannerSize)
	scanner.Buffer(buf, maxScannerSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req mcp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, mcp.ErrCodeParse, "Parse error", err)
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req *mcp.Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		s.initialized = true
		// No response for notifications
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendError(req.ID, mcp.ErrCodeMethodNotFound, "Method not found", nil)
	}
}

func (s *Server) handleInitialize(req *mcp.Request) {
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ServerCapabilities{
			Tools: &mcp.ToolsCapability{},
		},
		ServerInfo: mcp.ServerInfo{
			Name:    "mark42",
			Version: Version,
		},
	}

	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *mcp.Request) {
	result := mcp.ToolsListResult{
		Tools: s.handler.Tools(),
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsCall(req *mcp.Request) {
	var params mcp.ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, mcp.ErrCodeInvalidParams, "Invalid params", err)
		return
	}

	result, err := s.handler.CallTool(params.Name, params.Arguments)
	if err != nil {
		s.sendResult(req.ID, &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	s.sendResult(req.ID, result)
}

func (s *Server) sendResult(id, result any) {
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.send(resp)
}

func (s *Server) sendError(id any, code int, message string, data any) {
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &mcp.Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.send(resp)
}

func (s *Server) send(resp mcp.Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		logError("failed to marshal response: %v", err)
		return
	}
	fmt.Println(string(data))
}

func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mark42] "+format+"\n", args...)
}

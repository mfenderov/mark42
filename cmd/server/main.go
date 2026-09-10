package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mfenderov/mark42/internal/mcp"
	"github.com/mfenderov/mark42/internal/paths"
	"github.com/mfenderov/mark42/internal/storage"
)

var Version = "dev"

func main() {
	var dbFlag string
	flag.StringVar(&dbFlag, "db", "", "path to SQLite database")
	flag.Parse()

	// Determine database path
	home, _ := os.UserHomeDir()
	dbPath := paths.ResolveDBPath(home)
	if dbFlag != "" {
		dbPath = paths.ResolvePath(dbFlag)
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
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := embedder.CreateEmbedding(ctx, "test"); err != nil {
			logError("embedder unavailable at %s — semantic search disabled", embedderURL)
		} else {
			handler.WithEmbedder(embedder)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &Server{handler: handler}
	if err := server.Run(ctx); err != nil {
		logError("server error: %v", err)
		os.Exit(1)
	}
}

// Server handles MCP JSON-RPC communication over stdio.
type Server struct {
	handler     *mcp.Handler
	initialized bool
	in          io.Reader
	out         io.Writer
}

// Run starts the server's main loop. Stops when ctx is cancelled or stdin is closed.
func (s *Server) Run(ctx context.Context) error {
	in := s.in
	if in == nil {
		in = os.Stdin
	}
	scanner := bufio.NewScanner(in)

	const maxScannerSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxScannerSize)
	scanner.Buffer(buf, maxScannerSize)

	lines := make(chan []byte)
	scanErr := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			line := make([]byte, len(scanner.Bytes()))
			copy(line, scanner.Bytes())
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		scanErr <- scanner.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-scanErr:
			return err
		case line := <-lines:
			if len(line) == 0 {
				continue
			}
			if !json.Valid(line) {
				s.sendError(nil, mcp.ErrCodeParse, "Parse error", nil)
				continue
			}
			var req mcp.Request
			if err := json.Unmarshal(line, &req); err != nil {
				s.sendError(nil, mcp.ErrCodeInvalidRequest, "Invalid Request", err)
				continue
			}
			if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
				s.sendError(req.ID, mcp.ErrCodeInvalidRequest, "Invalid Request", nil)
				continue
			}
			s.handleRequest(ctx, &req)
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req *mcp.Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		s.initialized = true
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	default:
		// Per JSON-RPC 2.0, notifications (no ID or notifications/*) must never receive error responses.
		if req.ID != nil && !strings.HasPrefix(req.Method, "notifications/") {
			s.sendError(req.ID, mcp.ErrCodeMethodNotFound, "Method not found", nil)
		}
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

func (s *Server) handleToolsCall(ctx context.Context, req *mcp.Request) {
	var params mcp.ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, mcp.ErrCodeInvalidParams, "Invalid params", err)
		return
	}

	result, err := s.handler.CallToolContext(ctx, params.Name, params.Arguments)
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
	if id == nil {
		return
	}
	resp := mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.send(resp)
}

func (s *Server) sendError(id any, code int, message string, data any) {
	if id == nil && code != mcp.ErrCodeParse && code != mcp.ErrCodeInvalidRequest {
		return
	}
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
	out := s.out
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintln(out, string(data))
}

func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mark42] "+format+"\n", args...)
}

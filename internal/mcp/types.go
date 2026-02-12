package mcp

import "encoding/json"

// JSON-RPC 2.0 types

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// MCP Protocol types

type InitializeParams struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ClientInfo      ClientInfo   `json:"clientInfo"`
}

type Capabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type SamplingCapability struct{}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool definitions

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Items       *Items `json:"items,omitempty"`
}

type Items struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Memory tool input types (matching @modelcontextprotocol/server-memory API)

type CreateEntitiesInput struct {
	Entities []EntityInput `json:"entities"`
}

type EntityInput struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

type CreateRelationsInput struct {
	Relations []RelationInput `json:"relations"`
}

type RelationInput struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
}

type AddObservationsInput struct {
	Observations []ObservationInput `json:"observations"`
}

type ObservationInput struct {
	EntityName string   `json:"entityName"`
	Contents   []string `json:"contents"`
	FactType   string   `json:"factType,omitempty"` // Optional: "static", "dynamic", "session_turn"
}

type DeleteEntitiesInput struct {
	EntityNames []string `json:"entityNames"`
}

type DeleteObservationsInput struct {
	Deletions []DeletionInput `json:"deletions"`
}

type DeletionInput struct {
	EntityName   string   `json:"entityName"`
	Observations []string `json:"observations"`
}

type DeleteRelationsInput struct {
	Relations []RelationInput `json:"relations"`
}

type SearchNodesInput struct {
	Query string `json:"query"`
}

type OpenNodesInput struct {
	Names []string `json:"names"`
}

type GetContextInput struct {
	ProjectName   string  `json:"projectName,omitempty"`
	TokenBudget   int     `json:"tokenBudget,omitempty"`
	MinImportance float64 `json:"minImportance,omitempty"`
}

type GetRecentContextInput struct {
	Hours       int    `json:"hours,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	TokenBudget int    `json:"tokenBudget,omitempty"`
}

type SummarizeEntityInput struct {
	EntityName string `json:"entityName"`
}

type ConsolidateMemoriesInput struct {
	EntityName string `json:"entityName"`
}

type CaptureSessionEventInput struct {
	ToolName  string `json:"toolName"`
	FilePath  string `json:"filePath,omitempty"`
	Command   string `json:"command,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type CaptureSessionInput struct {
	ProjectName string                     `json:"projectName"`
	Summary     string                     `json:"summary"`
	Events      []CaptureSessionEventInput `json:"events,omitempty"`
}

type RecallSessionsInput struct {
	ProjectName string `json:"projectName,omitempty"`
	Hours       int    `json:"hours,omitempty"`
	TokenBudget int    `json:"tokenBudget,omitempty"`
}

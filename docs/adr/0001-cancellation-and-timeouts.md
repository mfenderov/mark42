# ADR 0001: Context Cancellation and HTTP Timeout Protection

## Status
Accepted

## Context
In `mark42`, the MCP server runs as a long-lived process communicating via JSON-RPC 2.0 over standard I/O, delegating knowledge graph queries and embeddings to SQLite and local LLM endpoints (Ollama / Docker Model Runner).
Previously:
1. `EmbeddingClient` created an `http.Client` without an explicit timeout (`&http.Client{}`). When a local embedding server (e.g. Ollama) became slow, unresponsive, or crashed, network requests would hang indefinitely, locking writes across the system.
2. The MCP server's main loop in `cmd/server/main.go` accepted a root `context.Context` (wired to OS signals `SIGINT`/`SIGTERM`), but did not propagate this context into `CallTool` and downstream handlers.

## Decision
1. Configure a mandatory default timeout of 30 seconds on `EmbeddingClient.httpClient` in `internal/storage/embedding.go`.
2. Add `CallToolContext(ctx context.Context, name string, args json.RawMessage)` to `internal/mcp/Handler` and propagate the cancellation context from the server's main select loop into tool execution.

## Consequences
- **Positive**: Network hangs on local model servers fail fast after 30 seconds rather than locking the process permanently.
- **Positive**: Process shutdown signals (`SIGINT`/`SIGTERM`) cleanly propagate to running tool calls.
- **Neutral**: Embeddings for very large batches must finish within 30 seconds, which is appropriate for local embedding models with batch sizes $\le 50$.

# mark42 — pi adapter (recall-only)

Pi integration is **recall-only** for now: Pi reads memories back through the
mark42 MCP server via `pi-mcp-adapter`. Capture (writing session events back)
is deliberately deferred — see [Capture status](#capture-status) below.

## Install

Point `pi-mcp-adapter` at the mark42 MCP server. Add this to any MCP config
file Pi reads (see [Config precedence](#config-precedence)):

```json
{
  "mcpServers": {
    "mark42": {
      "command": "mark42-server",
      "args": [],
      "env": { "CLAUDE_MEMORY_DB": "${HOME}/.mark42/memory.db" }
    }
  }
}
```

This matches the repo's `.mcp.json.example` shape. If `mark42-server` is not on
`PATH` (e.g. installed via Homebrew), use the absolute path instead:

```json
{
  "mcpServers": {
    "mark42": {
      "command": "/opt/homebrew/bin/mark42-server",
      "args": [],
      "env": { "CLAUDE_MEMORY_DB": "${HOME}/.mark42/memory.db" }
    }
  }
}
```

`mark42-server` is the stdio MCP server binary (`make build-server`). It speaks
JSON-RPC 2.0 over stdio and exposes the full mark42 tool surface — including
the recall tools Pi uses to read memories back:

- `recall_sessions` — recent session summaries (cross-session recall)
- `get_context` / `get_recent_context` — context injection
- `search_nodes` / `open_nodes` / `read_graph` — knowledge-graph recall
- `summarize_entity` / `get_entity_history` — entity detail

## Config precedence

`pi-mcp-adapter` merges MCP config from these locations, highest precedence
first:

1. `~/.config/mcp/mcp.json` — user-global shared config
2. `~/.agents/mcp.json` — tool-agnostic global config
3. `~/.agents/mcp/mcp.json` — tool-agnostic global config
4. `~/.pi/agent/mcp.json` — Pi global override
5. `.mcp.json` — project-local shared config
6. `.pi/mcp.json` — Pi project override

The recommended place for the mark42 entry is `~/.config/mcp/mcp.json` (shared
across all projects) or a project's `.mcp.json`. Pi picks these up immediately;
run `/mcp` to confirm the server is detected, or `/reload` after editing.

## Capture status

Capture is a **future gate**, not implemented yet. Pi's plugin/extension API is
pre-1.0 and still in flux, so wiring session-event capture against it now would
lock us to an unstable surface. The plan is a thin TypeScript extension that
mirrors the OpenCode adapter (`adapters/opencode/mark42.js`): a bounded
in-memory queue drained on session end into `mark42 session capture`, then
`mark42 distill`. See `adapters/README.md` for the adapter contract.

Until then, Pi sessions are not captured — only recall works.

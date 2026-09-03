# mark42 — harness adapters

This directory holds per-harness adapters. Each adapter is a **thin shim** that
translates a harness's lifecycle events into `mark42` CLI calls and MCP recall.
The neutral core (`internal/storage`, `internal/mcp`) is harness-agnostic and is
never modified by an adapter.

| Adapter | Capture | Recall | Notes |
|---------|---------|--------|-------|
| `claude/` (Go) | ✅ hooks | ✅ MCP | `internal/adapter/claude` |
| `opencode/` (JS) | ✅ plugin | ✅ MCP | `adapters/opencode/mark42.js` |
| `pi/` | ⏳ deferred | ✅ MCP | recall-only via `pi-mcp-adapter` |

## The adapter contract

An adapter builds on these neutral surfaces. Nothing else is required.

### 1. CLI commands (capture)

- `mark42 session capture <project>` — reads a capture JSON object on stdin and
  creates + completes a session. See the worked example below for the exact
  stdin schema.
- `mark42 distill <session-name>` — turns a session's raw events into a
  structural summary (and consolidates observations).
- `mark42 consolidate` — deduplicates observations across all entities.
- `mark42 session list|get|recall` — inspect captured sessions.

### 2. MCP tools (recall)

The `mark42-server` binary (stdio, JSON-RPC 2.0) exposes the recall surface.
Adapters must **never** call these tools from within a capture path — a capture
MCP call is itself a tool execution and would recurse back into the adapter's
own hook. Recall is a separate, user-driven concern:

- `recall_sessions`, `get_context`, `get_recent_context`
- `search_nodes`, `open_nodes`, `read_graph`, `summarize_entity`

### 3. State files

- `~/.mark42/state/<slug>/current-session` — the active session name, written
  by `mark42 session capture` (when `CLAUDE_PROJECT_DIR` is set) and read back
  by the adapter to know which session to distill.
- `<slug>` = the project directory with `/` replaced by `-`
  (`internal/state.ProjectSlug`). Adapters must replicate this scheme exactly.

### 4. Environment

- `MARK42_DB` → `CLAUDE_MEMORY_DB` (legacy) → `~/.mark42/memory.db` — the store
  path resolution order (`internal/paths.ResolveDBPath`).
- `CLAUDE_PROJECT_DIR` — set by the adapter when spawning `mark42 session
  capture`; tells the CLI which project the session belongs to and where to
  write `current-session`.

## Worked example: capture → distill

The flow every capture adapter implements (see `adapters/opencode/mark42.js`
for a working reference):

1. **Queue** tool events in a bounded in-memory queue (no spawn per event).
   Map harness tool names to the neutral vocabulary (`bash→Bash`, `edit→Edit`,
   `write→Write`, `read→Read`, `grep→Grep`, unknown→`Other`).

2. **Flush** on session end — spawn, fire-and-forget:

   ```sh
   CLAUDE_PROJECT_DIR=/path/to/project \
     mark42 session capture my-project
   ```

   with this exact JSON on stdin:

   ```json
   {
     "summary": "OpenCode session: 3 tool events",
     "events": [
       { "toolName": "Bash",    "filePath": "",                 "command": "go test ./...", "timestamp": "2026-09-02T11:30:00Z" },
       { "toolName": "Edit",    "filePath": "/p/src/main.go",   "command": "",              "timestamp": "2026-09-02T11:31:00Z" },
       { "toolName": "Grep",    "filePath": "/p/src",           "command": "",              "timestamp": "2026-09-02T11:32:00Z" }
     ]
   }
   ```

   `summary` (string) and `events[]` (objects with `toolName` + optional
   `filePath`, `command`, `timestamp`) is the complete schema — this is the
   exact shape `internal/cli/session.go` decodes. A mismatch here is a silent
   capture failure.

3. **Distill** — after capture exits, read
   `~/.mark42/state/<slug>/current-session` for the session name, then spawn:

   ```sh
   mark42 distill session-my-project-20260902-113000-123456789
   ```

   `distill` replaces the placeholder `summary` with a structural summary
   derived from the events and consolidates duplicate observations.

4. **Failures** — every spawn is fire-and-forget with `stdio` ignored; errors
   are appended to `~/.mark42/state/adapter-errors.log` (and logged to the
   harness console). An adapter must never throw out of a lifecycle hook.

# mark42 — OpenCode adapter

A self-contained OpenCode plugin that captures tool-usage events into a bounded
in-memory queue and flushes them to the mark42 neutral core on session end.

Recall (reading memories back) is handled separately via the mark42 MCP server —
this adapter only does **capture**. It never calls MCP tools directly (a capture
MCP call is itself a tool execution, which would recurse back into this hook).

## Install

Two options — pick one.

### Option A — copy into the global plugin directory

```sh
mkdir -p ~/.config/opencode/plugins
cp adapters/opencode/mark42.js ~/.config/opencode/plugins/mark42.js
```

OpenCode auto-loads every `.js` file in `~/.config/opencode/plugins/` at startup.

### Option B — reference by `file://` path

Add the absolute path to your `opencode.json` (or `opencode.jsonc`) plugin array:

```jsonc
{
  "plugin": [
    "file:///Users/you/dev/mark42/adapters/opencode/mark42.js"
  ]
}
```

## Prerequisites

- `mark42` must be on `PATH` (the `mark42` CLI binary — `make build` produces it).
- `mark42` must be able to open its store: it resolves the DB via
  `MARK42_DB` → `CLAUDE_MEMORY_DB` → `~/.mark42/memory.db` (see `internal/paths`).

## How it works

The plugin exports a single `Mark42` function and returns two handlers:

- **`event`** — dispatches on `event.type`:
  - `session.created` → resets the queue and the flush guard (no spawn).
  - `session.idle` / `session.error` → **flush** (see below).
- **`"tool.execute.after"`** — maps the OpenCode tool name to the neutral
  event vocabulary and enqueues one event (no spawn):

  | OpenCode tool | neutral `toolName` |
  |---------------|--------------------|
  | `bash`        | `Bash`             |
  | `edit`        | `Edit`             |
  | `write`       | `Write`            |
  | `read`        | `Read`             |
  | `grep`        | `Grep`             |
  | anything else | `Other`            |

  The enqueued event is `{ toolName, filePath, command }` where `filePath`
  comes from `args.filePath` (or `args.path` for `grep`) and `command` from
  `args.command` for `bash` (truncated to 200 chars).

**Queue**: a bounded in-memory array (cap **500**, drop-oldest on overflow).
Nothing is spawned per event — the queue only drains on flush.

**Flush** (on `session.idle` or `session.error`, once per session):

1. Spawns `mark42 session capture <project-name>` (fire-and-forget) with
   `CLAUDE_PROJECT_DIR=<directory>` in the env and the capture JSON on stdin:

   ```json
   { "summary": "OpenCode session: N tool events",
     "events": [ { "toolName": "Bash", "filePath": "", "command": "go test ./...", "timestamp": "..." } ] }
   ```

   This matches the exact stdin schema of `mark42 session capture`
   (`internal/cli/session.go`): `summary` + `events[]` of
   `{toolName, filePath?, command?, timestamp?}`.

2. When capture exits, reads the neutral current-session file
   (`~/.mark42/state/<slug>/current-session`, where `slug` is the project dir
   with `/` replaced by `-` — identical to `internal/state`) and spawns
   `mark42 distill <session-name>` to turn the raw events into a summary.

**Failure handling**: every spawn is fire-and-forget with `stdio` ignored.
Errors are appended to `~/.mark42/state/adapter-errors.log` **and** written to
`console.warn`. The plugin never throws.

## Tuning

- **Queue cap** — `QUEUE_CAP` at the top of `mark42.js` (default 500). Lower it
  if a single session routinely produces far more than 500 tool events and you
  want to bound memory more tightly.
- **Command truncation** — `bash` commands are truncated to 200 chars to match
  the Claude adapter. Adjust the `200` literal in `mapTool` if needed.
- **Summary text** — the capture `summary` is a placeholder
  (`OpenCode session: N tool events`); `mark42 distill` replaces it with a
  structural summary derived from the events.

## Verifying

Syntax check (no test framework):

```sh
node --check adapters/opencode/mark42.js
```

Manual smoke test — imports the plugin, dispatches synthetic events, and
asserts the capture + distill spawns are correct (uses a fake `mark42` on
`PATH`, so it touches nothing real):

```sh
node adapters/opencode/scripts/smoke.mjs
# PASS: dispatched events, capture + distill spawned correctly
```

# Async Hooks — Design Spec

**Date**: 2026-04-08
**Status**: Approved

## Context

Claude Code has added `"async": true` support for `type: "command"` hooks. Async hooks fire in the background — Claude does not wait for them to complete, and their exit codes / stdout are ignored (logged only).

mark42's `PostToolUse` and `Stop` hooks are currently synchronous. Both are pure side-effect hooks (zero stdout, exit 0 always) that unnecessarily block Claude on every tool call and at session end. Making them async removes that latency. It also unlocks a capability improvement: `PostToolUse` can write events directly to SQLite in real time, eliminating the intermediate `session-events` flat file and enabling all-tool tracking.

## Goals

1. Remove latency from every tool call (`PostToolUse` sync → async)
2. Remove latency at session end (`Stop` sync → async)
3. Eliminate the `session-events` flat file (real-time SQLite writes instead)
4. Expand `PostToolUse` coverage to all tools (matcher `.*`)

## Non-Goals

- New hooks (`SubagentStop`, `TaskCompleted`) — deferred, add after this lands
- Inline embedding during `PostToolUse` — deferred
- Eliminating `dirty-files` flat file — kept for now (simpler path accumulation)

## Data Flow

### Before

```
SessionStart (sync)   → clear stop flag, inject context
PostToolUse (sync)    → append to session-events (flat file)
                        append to dirty-files (flat file)
Stop (sync)           → read session-events flat file
                        CREATE session in SQLite
                        write events as observations
                        generate summary → CompleteSession
                        clear session-events, dirty-files
```

### After

```
SessionStart (sync)   → clear stop flag, inject context
                        CREATE pending session in SQLite
                        write session name → .claude/mark42/current-session

PostToolUse (async)   → read current-session name
                        write event directly to SQLite (CaptureSessionEvent)
                        append to dirty-files (unchanged)
                        matcher: .* (all tools)

Stop (async)          → read current-session name
                        build digest from transcript
                        read dirty-files for summary
                        CompleteSession in SQLite
                        delete current-session file
                        clear dirty-files
                        sync CC memory
```

## Component Changes

### `hooks/hooks.json`

```json
"PostToolUse": [{
  "matcher": ".*",
  "hooks": [{ "type": "command", "command": "...", "timeout": 15, "async": true }]
}],
"Stop": [{
  "hooks": [{ "type": "command", "command": "...", "timeout": 120, "async": true }]
}]
```

`SessionStart` and `PreCompact` remain synchronous — both output content Claude must read.

### `hook_session_start.go`

After existing context injection, append:
1. Call `store.CreateSession(projectName)` → returns session entity
2. Write `session.Name` to `.claude/mark42/current-session`
3. On any error: log and continue silently (context injection must not fail)

### `hook_post_tool_use.go`

Replace session-events flat file logic with SQLite writes:
1. Read `.claude/mark42/current-session` to get session name
2. If present: call `store.CaptureSessionEvent(sessionName, event)`
3. If absent: skip silently (Stop will handle gracefully via fallback)
4. Remove all `session-events` file append logic
5. Keep `dirty-files` logic unchanged

Matcher expansion (`.*`) is a config-only change — the hook already handles unknown tools via the `default` case in the switch.

### `hook_stop.go`

1. Read `.claude/mark42/current-session`
2. If present: use that session name directly
3. If absent (edge case — SessionStart never ran): fall back to `store.CreateSession()` then complete (same as today)
4. Remove `readJSONLines[eventEntry]` call (no more session-events file)
5. Remove `clearFile(session-events)` call
6. Add `os.Remove(currentSessionFilePath)` at the end

### Storage layer

No changes. `CreateSession`, `CaptureSessionEvent`, and `CompleteSession` already have the right signatures.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| SessionStart can't create session | Log, skip writing current-session; PostToolUse skips SQLite; Stop uses fallback CreateSession |
| PostToolUse can't read current-session | Skip SQLite write silently |
| PostToolUse SQLite write fails | Log to stderr (async, so Claude never sees it), continue |
| Stop can't read current-session | Fallback: create session from scratch (as today) |
| current-session file left over from crashed session | Stop reads it, tries to complete non-existent session; storage returns ErrNotFound → fallback to CreateSession |

## Testing

1. Unit tests for `runSessionStartHook` — verify `current-session` file is written with correct name
2. Unit tests for `runPostToolUseHook` — verify SQLite write when current-session present; verify skip when absent
3. Unit tests for `runStopHook` — verify session completion using current-session; verify fallback path when file absent
4. Integration test: full session lifecycle (start → tool use × N → stop) verifies events in SQLite, no session-events file created

## Files Modified

- `hooks/hooks.json`
- `cmd/memory/hook_session_start.go`
- `cmd/memory/hook_post_tool_use.go`
- `cmd/memory/hook_stop.go`

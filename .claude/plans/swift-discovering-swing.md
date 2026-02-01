# Implementation Plan: Claude Memory CLI

## Overview

Build a local, privacy-first memory system for Claude Code using SQLite + FTS5, exposed via CLI instead of MCP.

**Key insight**: MCP adds ~1000 lines of protocol code for what is essentially CRUD + search. A CLI is simpler, more debuggable, and works outside Claude Code.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     Claude Code                           │
│                                                           │
│  SessionStart Hook          Stop Hook                     │
│  (inject context)           (persist memories)            │
│         │                          │                      │
│         └──────────┬───────────────┘                      │
│                    ▼                                      │
│               Bash Tool                                   │
└──────────────────────────────────────────────────────────┘
                     │
                     ▼
           ┌─────────────────┐
           │  claude-memory  │
           │      CLI        │
           └─────────────────┘
                     │
                     ▼
           ┌─────────────────┐
           │ SQLite + FTS5   │
           │ ~/.claude/      │
           │ memory.db       │
           └─────────────────┘
```

## Deliverables

| # | Component | Description |
|---|-----------|-------------|
| 1 | CLI binary | `claude-memory` - Go binary with entity/search/relation commands |
| 2 | SQLite schema | entities, observations, relations tables + FTS5 index |
| 3 | SessionStart hook | Injects relevant memories at session start |
| 4 | Stop hook update | Update existing hook to use CLI instead of MCP |
| 5 | Migration command | Import data from current JSON Memory MCP |
| 6 | CLAUDE.md updates | Instructions for memory usage |

---

## Phase 1: CLI Core (TDD)

### 1.1 Project Setup

**Files to create/modify:**
- `cmd/memory/main.go` - CLI entry point with cobra
- `internal/storage/store.go` - SQLite connection management
- `internal/storage/schema.go` - Schema definitions and migrations
- `internal/storage/entity.go` - Entity CRUD operations
- `internal/storage/observation.go` - Observation operations
- `internal/storage/relation.go` - Relation operations
- `internal/storage/search.go` - FTS5 search implementation

**Dependencies:**
```go
require (
    github.com/spf13/cobra v1.8.0        // CLI framework
    modernc.org/sqlite v1.28.0           // Pure Go SQLite
)
```

### 1.2 Database Schema

```sql
-- Core tables
CREATE TABLE entities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    entity_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(entity_id, content)
);

CREATE TABLE relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    to_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_entity_id, to_entity_id, relation_type)
);

-- FTS5 indexes
CREATE VIRTUAL TABLE observations_fts USING fts5(
    content,
    content='observations',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE VIRTUAL TABLE entities_fts USING fts5(
    name,
    entity_type,
    content='entities',
    content_rowid='id',
    tokenize='porter unicode61'
);

-- Sync triggers for FTS
CREATE TRIGGER obs_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
END;
-- (delete and update triggers similar)
```

### 1.3 CLI Commands

```bash
# Entity operations
claude-memory entity create <name> <type> [--obs "observation"]...
claude-memory entity get <name>
claude-memory entity list [--type <type>]
claude-memory entity delete <name>

# Observation operations
claude-memory obs add <entity> <content>
claude-memory obs delete <entity> <content>

# Relation operations
claude-memory rel create <from> <to> <type>
claude-memory rel list <entity>
claude-memory rel delete <from> <to> <type>

# Search (key feature)
claude-memory search <query> [--limit N]

# Graph operations
claude-memory graph [--format json|dot]

# Database management
claude-memory init [--path <path>]
claude-memory migrate --from <json-file>
claude-memory stats
```

### 1.4 TDD Order

1. **storage/store_test.go** - Database connection, schema creation
2. **storage/entity_test.go** - Entity CRUD
3. **storage/observation_test.go** - Observation add/delete
4. **storage/relation_test.go** - Relation CRUD
5. **storage/search_test.go** - FTS5 search
6. **cmd/memory/*** - Integration tests for CLI commands

---

## Phase 2: Hooks Integration

### 2.1 SessionStart Hook

**File:** `~/.claude/hooks/memory-context-inject.sh`

```bash
#!/bin/bash
# Inject relevant memories at session start

DB_PATH="${CLAUDE_MEMORY_DB:-$HOME/.claude/memory.db}"

if [[ ! -f "$DB_PATH" ]]; then
    exit 0  # No memory yet, skip
fi

# Get recent and project-relevant memories
CONTEXT=$(claude-memory search "$(basename $(pwd))" --limit 5 --format context 2>/dev/null)

if [[ -n "$CONTEXT" ]]; then
    echo "Recent relevant memories:"
    echo "$CONTEXT"
fi
```

**Register in settings.json:**
```json
"SessionStart": [
  {
    "matcher": "startup",
    "hooks": [
      {
        "type": "command",
        "command": "~/.claude/hooks/memory-context-inject.sh",
        "timeout": 5
      }
    ]
  }
]
```

### 2.2 Stop Hook Update

**File:** `~/.claude/hooks/memory-mcp-sync.sh` (update existing)

Change prompt from using `mcp__memory__*` tools to:

```
Memory sync: If learnings worth persisting, use Bash to call:
  claude-memory entity create "<name>" "<type>" --obs "<observation>"
  claude-memory rel create "<from>" "<to>" "<relation>"
If nothing significant, say 'nothing to persist' and proceed.
```

---

## Phase 3: Migration & Rollout

### 3.1 Migration Command

```bash
claude-memory migrate --from ~/.docker-volumes/claude-memory/memory.json
```

Implementation:
1. Read JSON file (same format as Memory MCP)
2. Parse entities, observations, relations
3. Insert into SQLite with conflict handling
4. Report statistics

### 3.2 Rollout Steps

1. **Build & test CLI** locally
2. **Run migration** from Docker volume JSON
3. **Verify data** with `claude-memory stats` and `claude-memory graph`
4. **Update hooks** to use CLI
5. **Test in new session** - verify SessionStart injects context
6. **Test persistence** - verify Stop hook stores memories
7. **Disable old MCP** - remove/comment Docker MCP from `.mcp.json`
8. **Monitor** for issues over a few sessions

### 3.3 Rollback Plan

If issues occur:
1. Re-enable Docker MCP in `.mcp.json`
2. Revert hook changes
3. Data is preserved in SQLite for debugging

---

## Phase 4: Plugin Packaging (Optional)

Once stable, package as Claude Code plugin:

```
claude-memory-plugin/
├── plugin.json
├── bin/
│   └── claude-memory          # Pre-built binary
├── hooks/
│   ├── session-start.sh
│   └── stop.sh
└── README.md
```

This makes installation: `claude plugin install claude-memory-plugin`

---

## Implementation Order

| Step | Task | Est. Time |
|------|------|-----------|
| 1 | Project setup, dependencies | 15 min |
| 2 | Schema + migrations (TDD) | 30 min |
| 3 | Entity CRUD (TDD) | 30 min |
| 4 | Observation operations (TDD) | 20 min |
| 5 | Relation operations (TDD) | 20 min |
| 6 | FTS5 search (TDD) | 30 min |
| 7 | CLI commands with cobra | 45 min |
| 8 | Migration command | 30 min |
| 9 | SessionStart hook | 15 min |
| 10 | Stop hook update | 10 min |
| 11 | Integration testing | 30 min |
| 12 | Rollout & verify | 20 min |

**Total: ~5 hours**

---

## Verification Plan

### Unit Tests
```bash
make test          # All unit tests pass
make test-coverage # >80% coverage
```

### Integration Tests
```bash
# Create test data
claude-memory entity create "test-entity" "test-type" --obs "test observation"

# Verify search
claude-memory search "test" | grep "test-entity"

# Verify graph
claude-memory graph | jq '.entities | length'
```

### End-to-End Tests
1. Start new Claude session → SessionStart hook injects context
2. Ask Claude to remember something → Stop hook prompts, Claude calls CLI
3. Start another session → Previous memory appears in context

---

## Critical Files

| File | Purpose |
|------|---------|
| `cmd/memory/main.go` | CLI entry point |
| `internal/storage/store.go` | SQLite connection |
| `internal/storage/schema.go` | Schema + migrations |
| `internal/storage/search.go` | FTS5 implementation |
| `~/.claude/hooks/memory-context-inject.sh` | SessionStart hook |
| `~/.claude/hooks/memory-mcp-sync.sh` | Stop hook (update) |
| `~/.claude/memory.db` | SQLite database |

---

## Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| MCP vs CLI? | **CLI** - simpler, debuggable, portable |
| Language? | **Go** - single binary, good SQLite support |
| Search? | **FTS5** Phase 1, vectors Phase 2 |
| Integration? | **Hooks** - SessionStart + Stop |
| Skills/Agents? | **Later** - if CLI feels clunky |

# Claude Memory Plugin - Implementation Plan

## Overview

Replace auto-memory with a feature-rich SQLite-based alternative that provides:
1. **CLAUDE.md management** (like auto-memory) - AUTO-MANAGED sections
2. **Knowledge persistence** (SQLite) - Entities, observations, relations with FTS5 search
3. **Dual interface** - CLI for hooks/humans + MCP server for Claude Code tools

## Current State

- ✅ SQLite storage layer complete (`internal/storage/`)
- ✅ CLI complete (`cmd/memory/main.go`)
- ✅ Database initialized (`~/.claude/memory.db` - 32 entities, 273 observations, 38 relations)
- ❌ MCP server not implemented (`cmd/server/` empty)
- ❌ Plugin structure not created

---

## Phase 1: MCP Server (Foundation)

### Files to Create

**`cmd/server/main.go`** (~200 lines)
```
- JSON-RPC 2.0 over stdio
- MCP lifecycle: initialize, tools/list, tools/call
- Log to stderr (not stdout)
```

**`internal/mcp/handlers.go`** (~150 lines)
```
- 9 tool handlers mapping to storage layer:
  create_entities, create_relations, add_observations,
  delete_entities, delete_observations, delete_relations,
  read_graph, search_nodes, open_nodes
```

**`internal/mcp/types.go`** (~100 lines)
```
- MCPRequest, MCPResponse, MCPError structs
- Tool input/output types matching Memory MCP spec
```

### Testing
```bash
# Manual protocol test
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/mark42-server
```

---

## Phase 2: Plugin Structure

### Directory Layout
```
mark42/
├── .claude-plugin/
│   └── plugin.json           # Plugin metadata
├── hooks/
│   ├── hooks.json            # Hook registration
│   ├── post-tool-use.py      # Track dirty files (zero-token)
│   ├── stop.py               # Spawn memory-updater
│   └── session-start.py      # Context injection (NEW)
├── agents/
│   ├── memory-updater.md     # Orchestrate updates
│   └── knowledge-extractor.md # Extract to SQLite (NEW)
├── skills/
│   ├── memory-processor/SKILL.md
│   └── codebase-analyzer/SKILL.md
├── commands/
│   ├── init.md               # /memory:init
│   ├── status.md             # /memory:status
│   ├── sync.md               # /memory:sync
│   └── calibrate.md          # /memory:calibrate
├── bin/                      # Compiled binaries
│   ├── mark42
│   └── mark42-server
└── .mcp.json                 # MCP server registration
```

### Key Configurations

**`.claude-plugin/plugin.json`**
```json
{
  "name": "mark42",
  "description": "SQLite-backed memory with CLAUDE.md management",
  "version": "1.0.0"
}
```

**`hooks/hooks.json`**
```json
{
  "hooks": {
    "PostToolUse": [{"matcher": "Edit|Write|Bash", "hooks": [...]}],
    "Stop": [{"hooks": [...]}],
    "SessionStart": [{"hooks": [...]}]
  }
}
```

**`.mcp.json`**
```json
{
  "mcpServers": {
    "memory": {
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/mark42-server",
      "args": ["--db", "~/.claude/memory.db"]
    }
  }
}
```

---

## Phase 3: Hook Scripts

### PostToolUse (`hooks/post-tool-use.py`)
- Fires after Edit/Write/Bash
- Appends changed files to `.claude/auto-memory/dirty-files`
- **Zero output** (critical for token efficiency)
- Parse Bash for: rm, mv, git commit

### Stop (`hooks/stop.py`)
- Check dirty-files exists and non-empty
- Output `{"decision": "block", "reason": "...spawn memory-updater..."}`
- Pass-through if no changes

### SessionStart (`hooks/session-start.py`) **NEW**
- Query SQLite for project-relevant memories
- Inject top 5 results as context
- Non-blocking (fails silently)

---

## Phase 4: Agents

### memory-updater (adapt from auto-memory)
1. Load dirty files
2. Gather file context (max 7 files, 200 lines each)
3. **NEW**: Spawn knowledge-extractor for SQLite updates
4. Invoke memory-processor skill for CLAUDE.md
5. Clear dirty-files

### knowledge-extractor **NEW**
- Extract meaningful patterns, decisions, conventions
- Create entities via CLI: `mark42 entity create ...`
- Create relations via CLI: `mark42 rel create ...`
- Criteria: Cross-cutting patterns, architectural decisions, tool usage

---

## Phase 5: Skills & Commands

### Skills (adapt from auto-memory)
- **memory-processor**: Update AUTO-MANAGED sections
- **codebase-analyzer**: Initialize CLAUDE.md wizard

### Commands
- `/memory:init` - Initialize database + CLAUDE.md
- `/memory:status` - Show stats for both systems
- `/memory:sync` - Trigger manual update
- `/memory:calibrate` - Review AUTO-MANAGED sections

---

## Implementation Order

### Week 1: MCP Server
1. `cmd/server/main.go` - JSON-RPC stdio server
2. `internal/mcp/handlers.go` - Tool implementations
3. Unit tests + manual protocol testing
4. Update Makefile: `build-server`, `install-plugin`

### Week 2: Plugin Core
1. Create directory structure
2. Write plugin.json, hooks.json, .mcp.json
3. Port hook scripts from auto-memory
4. Add SessionStart context injection

### Week 3: Agents & Skills
1. Adapt memory-updater agent
2. Create knowledge-extractor agent
3. Port memory-processor skill
4. Port codebase-analyzer skill

### Week 4: Commands & Testing
1. Implement 4 slash commands
2. End-to-end testing
3. Migration testing (auto-memory → mark42)
4. Documentation updates

---

## Verification Checklist

### MCP Server
- [ ] `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/mark42-server` returns tools
- [ ] Create entity via MCP → appears in `mark42 entity list`
- [ ] Search via MCP → returns FTS5 ranked results

### Hooks
- [ ] Edit file → dirty-files updated (silent)
- [ ] End turn with dirty files → memory-updater spawns
- [ ] Session start → relevant context injected

### Integration
- [ ] Full cycle: Edit → Stop → Update CLAUDE.md → Extract knowledge → Clear
- [ ] `/memory:status` shows both CLAUDE.md and SQLite stats
- [ ] Search finds knowledge extracted from code changes

---

## Key Design Decisions

1. **Hybrid storage**: CLAUDE.md (human-readable) + SQLite (machine-queryable)
2. **Dual interface**: CLI (hooks/humans) + MCP (Claude Code tools)
3. **Batch processing**: Accumulate changes, process at turn end
4. **Zero-token hooks**: PostToolUse produces no output
5. **Backward compatible**: Same AUTO-MANAGED format as auto-memory

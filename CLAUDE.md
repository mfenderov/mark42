# Claude Memory - Project Guide

## Project Overview

A local, privacy-first RAG memory system for Claude Code, built on SQLite with Go.

**Purpose**: Replace JSON-based Memory MCP with SQLite-backed implementation offering superior search capabilities (FTS5 + future vector search).

**Status**: Phase 1 - Foundation (FTS5 full-text search)

**Key differentiators**:
- Privacy-first: All data stays local (no cloud, no telemetry)
- Single-file portability: One `memory.db` file for backup/sync
- Drop-in replacement: Same MCP API as `@modelcontextprotocol/server-memory`
- Incremental complexity: Start with FTS5, add vectors when needed

## Quick Reference

<!-- AUTO-MANAGED: build-commands -->
| Command | Purpose |
|---------|---------|
| `make build` | Build CLI binary with version info |
| `make build-server` | Build MCP server binary |
| `make build-all` | Build both CLI and server binaries |
| `make test` | Run tests with race detector |
| `make test-coverage` | Generate coverage report (HTML) |
| `make run` | Build and run CLI with test.db |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code (go fmt + goimports) |
| `make tidy` | Clean up go.mod dependencies |
| `make clean` | Remove binaries and test artifacts |
| `make install` | Install CLI binary to ~/bin |
| `make install-server` | Install MCP server to ~/bin |
| `make install-all` | Install both binaries to ~/bin |
| `make install-plugin` | Build binaries to bin/ for plugin deployment |
| `make migrate` | Migrate from JSON Memory MCP to SQLite |
<!-- END AUTO-MANAGED -->

## Architecture

<!-- AUTO-MANAGED: architecture -->
```
cmd/
  ├── memory/main.go   → CLI entry point (cobra, lipgloss)
  └── server/main.go   → MCP server entry point (JSON-RPC over stdio)
internal/
  ├── storage/         → SQLite operations (Store, Entity, Relation, Observation, Search)
  │   ├── store.go     → Database initialization, schema, lifecycle
  │   ├── entity.go    → Entity CRUD with transactions
  │   ├── observation.go → Observation add/delete operations
  │   ├── relation.go  → Relation CRUD (bidirectional queries)
  │   └── search.go    → FTS5 search with BM25 ranking
  └── mcp/             → MCP protocol implementation
      ├── types.go     → JSON-RPC 2.0 types, MCP protocol types
      └── handlers.go  → Tool handlers (create_entities, search_nodes, etc.)
.claude-plugin/
  ├── plugin.json      → Plugin metadata
  ├── .mcp.json        → MCP server configuration
  └── hooks/           → Lifecycle hooks (post-tool-use, stop, session-start)
agents/                → Specialized agents (memory-updater, knowledge-extractor)
skills/                → Skill definitions (memory-processor, codebase-analyzer)
commands/              → Command documentation (init, status, sync, calibrate)
```

**Data flow**: Claude Code (stdio) → MCP Server (Go, JSON-RPC) → Storage Layer → SQLite (FTS5 + sqlite-vec)

**Storage patterns**:
- Transactions for atomic operations (CreateEntity uses tx.Begin/Commit/Rollback)
- Foreign key cascades (ON DELETE CASCADE for observations/relations)
- Duplicate prevention (INSERT OR IGNORE, UNIQUE constraints)
- WAL mode enabled for better concurrency
- FTS5 kept in sync via triggers

**MCP integration**:
- JSON-RPC 2.0 protocol over stdio
- Standard Memory MCP API compatible
- Tool handlers map to storage operations
- Hooks for auto-memory capture (Edit, Write, Bash)
<!-- END AUTO-MANAGED -->

See `docs/ARCHITECTURE.md` for:
- Complete database schema (entities, observations, relations)
- FTS5 indexing strategy with triggers
- Hybrid search algorithm (keyword + vector)
- Phase 2 embedding pipeline design

## CLI Commands

<!-- AUTO-MANAGED: cli-commands -->
**Entity management**:
- `claude-memory entity create <name> <type> [--obs "observation"]` - Create entity with observations
- `claude-memory entity get <name>` - Retrieve entity with observations
- `claude-memory entity list [--type <type>]` - List all entities, optionally filtered by type
- `claude-memory entity delete <name>` - Delete entity (cascades to observations/relations)

**Observation management**:
- `claude-memory obs add <entity-name> <content>` - Add observation to entity
- `claude-memory obs delete <entity-name> <content>` - Remove specific observation

**Relation management**:
- `claude-memory rel create <from> <to> <type>` - Create relation between entities
- `claude-memory rel list <entity-name>` - List all relations (bidirectional)
- `claude-memory rel delete <from> <to> <type>` - Delete specific relation

**Search and exploration**:
- `claude-memory search <query>` - FTS5 full-text search (BM25 ranked)
- `claude-memory graph` - Export entire knowledge graph

**Utilities**:
- `claude-memory init` - Initialize database schema
- `claude-memory stats` - Show database statistics
- `claude-memory version` - Display version info
- `claude-memory migrate --from <json> --to <db>` - Migrate from JSON Memory MCP

**Default database**: `~/.claude/memory.db` (override with `--db <path>`)
<!-- END AUTO-MANAGED -->

## Development Workflow

1. **TDD Required**: Write failing test first
2. **FTS5 Focus**: Phase 1 is keyword search only
3. **API Compatibility**: Match existing Memory MCP tools exactly

## Plugin Structure

The project includes a complete Claude Code plugin implementation:

**Agents** (specialized behavior):
- `memory-updater.md` - Orchestrates CLAUDE.md updates and knowledge extraction
- `knowledge-extractor.md` - Extracts entities, observations, relations from context

**Skills** (reusable operations):
- `memory-processor/SKILL.md` - Updates AUTO-MANAGED sections in CLAUDE.md
- `codebase-analyzer/SKILL.md` - Analyzes code patterns and conventions

**Commands** (user-facing):
- `init.md` - Initialize plugin and database
- `status.md` - Show memory system status
- `sync.md` - Synchronize dirty files to memory
- `calibrate.md` - Tune memory extraction parameters

**Hooks** (lifecycle integration):
- `post-tool-use.py` - Tracks file modifications (Edit, Write, Bash)
- `stop.py` - Triggers memory-updater on session end
- `session-start.py` - Loads context from SQLite

## Key Files

- `docs/ARCHITECTURE.md` - System design, schema, search algorithms
- `docs/DESIGN_DECISIONS.md` - Rationale for SQLite, Go, FTS5-first, hybrid search
- `internal/storage/store.go` - Database schema definitions and initialization
- `internal/storage/search.go` - FTS5 search implementation (BM25 ranking)
- `internal/mcp/handlers.go` - MCP tool implementations (JSON-RPC handlers)
- `cmd/server/main.go` - MCP server entry point (stdio communication)
- `Makefile` - Build commands with version tagging
- `.gitignore` - Excludes binary, test.db, coverage reports, IDE files

## Testing Strategy

- **Unit tests**: Each package has `*_test.go`
- **Integration tests**: `test/integration/` with real SQLite
- **Benchmark**: Compare against JSON Memory MCP

## Dependencies

| Package | Purpose |
|---------|---------|
| `modernc.org/sqlite` | Pure Go SQLite driver (no CGO) |
| `github.com/spf13/cobra` | CLI framework for commands |
| `github.com/charmbracelet/lipgloss` | Terminal styling for output |
| `github.com/charmbracelet/log` | Structured logging |

## MCP Tools Implementation Status

| Tool | Storage Layer | MCP Handler | Status |
|------|---------------|-------------|--------|
| `create_entities` | ✅ CreateEntity | ✅ DONE | Implemented |
| `create_relations` | ✅ CreateRelation | ✅ DONE | Implemented |
| `add_observations` | ✅ AddObservation | ✅ DONE | Implemented |
| `delete_entities` | ✅ DeleteEntity | ✅ DONE | Implemented |
| `delete_observations` | ✅ DeleteObservation | ✅ DONE | Implemented |
| `delete_relations` | ✅ DeleteRelation | ✅ DONE | Implemented |
| `read_graph` | ✅ ReadGraph | ✅ DONE | Implemented |
| `search_nodes` | ✅ Search | ✅ DONE | Implemented |
| `open_nodes` | ✅ GetEntity | ✅ DONE | Implemented |

**All MCP tools implemented**. Server communicates via JSON-RPC 2.0 over stdio.

## Roadmap

**Phase 1 (Current)**: Foundation
- ✅ SQLite schema for knowledge graph (entities, observations, relations)
- ✅ FTS5 full-text search with BM25 ranking
- ✅ Storage layer complete (CRUD operations)
- ✅ MCP server with standard Memory API (JSON-RPC 2.0 over stdio)
- ✅ Claude Code plugin structure (agents, skills, commands, hooks)
- 🔲 Plugin installation and testing
- 🔲 Drop-in replacement for JSON Memory MCP verified

**Phase 2**: Semantic Search
- sqlite-vec integration
- Ollama embedding generation (nomic-embed-text)
- Hybrid search (keyword + vector with weighted fusion)
- Relevance tuning

**Phase 3**: Intelligence
- Auto-context injection at session start
- Importance scoring for memories
- Decay/consolidation of old memories
- Cross-session continuity

## Go Conventions

<!-- AUTO-MANAGED: conventions -->
**Error handling**:
- Return `ErrNotFound` for missing entities (defined in `entity.go`)
- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Check `sql.ErrNoRows` and convert to domain error

**Transaction safety**:
- Use `defer tx.Rollback()` immediately after `Begin()`
- Explicit `tx.Commit()` on success
- Pattern: Begin → defer Rollback → operations → Commit

**SQL patterns**:
- `INSERT OR IGNORE` for duplicate prevention
- `UNIQUE` constraints over application-level checks
- Foreign keys with `ON DELETE CASCADE`
- Join entity IDs back to names in queries

**CLI structure**:
- Cobra command tree with subcommands (entity, obs, rel, search, graph)
- Lipgloss styles for terminal output (entityStyle, typeStyle, obsStyle)
- Charmbracelet log for structured logging
- Default DB path: `~/.claude/memory.db`

**Testing**:
- Table-driven tests in `*_test.go` files
- Integration tests use real SQLite (not mocks)
- Test files in same package as implementation (`storage_test`)
<!-- END AUTO-MANAGED -->

## Design Principles

See `docs/DESIGN_DECISIONS.md` for detailed rationale:

1. **SQLite over alternatives**: Zero config, single file, battle-tested, FTS5 + sqlite-vec extensions
2. **Go over TypeScript/Python/Rust**: Single binary, fast startup, excellent SQLite bindings, cross-compilation
3. **FTS5 first, vectors second**: 80% value with 20% effort, validates architecture before complexity
4. **Ollama for embeddings**: Local privacy, simple setup, model flexibility, already common
5. **Hybrid search**: Exact matches + semantic understanding, tunable weights
6. **Drop-in API compatibility**: Zero migration effort, reversible, existing workflows preserved

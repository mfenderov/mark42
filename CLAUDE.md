# Claude Memory - Project Guide

## Project Overview

A local, privacy-first RAG memory system for Claude Code, built on SQLite with Go.

**Purpose**: Replace JSON-based Memory MCP with SQLite-backed implementation offering superior search capabilities (FTS5 + future vector search).

**Status**: Phase 2 Complete - Semantic Search (FTS5 + Vector Hybrid Search)

**Key differentiators**:
- Privacy-first: All data stays local (no cloud, no telemetry)
- Single-file portability: One `memory.db` file for backup/sync
- Drop-in replacement: Same MCP API as `@modelcontextprotocol/server-memory`
- Incremental complexity: FTS5 + vector hybrid search with RRF fusion

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
  ├── storage/         → SQLite operations (sqlx-based)
  │   ├── store.go     → Database initialization, schema, lifecycle
  │   ├── entity.go    → Entity CRUD with versioning support
  │   ├── observation.go → Observation add/delete with fact types
  │   ├── relation.go  → Relation CRUD (bidirectional queries)
  │   ├── search.go    → FTS5 search with BM25 ranking
  │   ├── hybrid.go    → Hybrid search (FTS5 + vector with RRF fusion)
  │   ├── embedding.go → Ollama/DMR embedding client
  │   ├── vector.go    → Vector storage and cosine similarity
  │   ├── fusion.go    → RRF and weighted score fusion
  │   ├── migration.go → Goose migration runner
  │   └── migrations/  → Goose Go migrations (001-006)
  └── mcp/             → MCP protocol implementation
      ├── types.go     → JSON-RPC 2.0 types, MCP protocol types
      └── handlers.go  → Tool handlers with hybrid search support
.claude-plugin/
  ├── plugin.json      → Plugin metadata
  ├── hooks.json       → Hook configuration
  └── hooks/           → Lifecycle hooks
      ├── post-tool-use.py → Tracks file modifications (Edit, Write, Bash)
      ├── session-start.py → Loads context from SQLite
      └── stop.py          → Triggers memory sync on session end
.mcp.json              → MCP server configuration
agents/                → Specialized agents (memory-updater, knowledge-extractor)
skills/                → Skill definitions (memory-processor, codebase-analyzer)
commands/              → Command documentation (init, status, sync, calibrate)
```

**Data flow**: Claude Code (stdio) → MCP Server (Go, JSON-RPC) → Storage Layer → SQLite (FTS5 + embeddings)

**Storage patterns**:
- **sqlx** for struct scanning (db tags, no manual Scan calls)
- **goose** for migrations (versioned, idempotent, rollback support)
- Transactions for atomic operations
- Foreign key cascades (ON DELETE CASCADE)
- WAL mode for better concurrency
- FTS5 kept in sync via triggers

**Phase 2 Features**:
- Hybrid search with RRF fusion (k=60)
- Ollama embeddings (nomic-embed-text)
- Fact types: static, dynamic, session_turn
- Entity versioning (supersedes_id, is_latest, version)
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
| `github.com/jmoiron/sqlx` | SQL extensions with struct scanning |
| `github.com/pressly/goose/v3` | Database migrations |
| `github.com/spf13/cobra` | CLI framework for commands |
| `github.com/charmbracelet/lipgloss` | Terminal styling for output |
| `github.com/charmbracelet/log` | Structured logging |

## MCP Tools Implementation Status

| Tool | Storage Layer | MCP Handler | Status |
|------|---------------|-------------|--------|
| `create_entities` | ✅ CreateEntity | ✅ DONE | Implemented |
| `create_or_update_entities` | ✅ CreateOrUpdateEntity | ✅ DONE | Versioning support |
| `create_relations` | ✅ CreateRelation | ✅ DONE | Implemented |
| `add_observations` | ✅ AddObservation | ✅ DONE | Implemented |
| `delete_entities` | ✅ DeleteEntity | ✅ DONE | Implemented |
| `delete_observations` | ✅ DeleteObservation | ✅ DONE | Implemented |
| `delete_relations` | ✅ DeleteRelation | ✅ DONE | Implemented |
| `read_graph` | ✅ ReadGraph | ✅ DONE | Implemented |
| `search_nodes` | ✅ Search | ✅ DONE | Implemented |
| `open_nodes` | ✅ GetEntity | ✅ DONE | Implemented |
| `get_context` | ✅ GetContextForInjection | ✅ DONE | Context injection |

**All 11 MCP tools implemented**. Server communicates via JSON-RPC 2.0 over stdio.

## Roadmap

**Phase 1**: Foundation ✅
- ✅ SQLite schema for knowledge graph (entities, observations, relations)
- ✅ FTS5 full-text search with BM25 ranking
- ✅ Storage layer complete (CRUD operations)
- ✅ MCP server with standard Memory API (JSON-RPC 2.0 over stdio)
- ✅ Claude Code plugin structure (agents, skills, commands, hooks)

**Phase 2 (Complete)**: Semantic Search ✅
- ✅ Hybrid search infrastructure (FTS5 + vector with RRF fusion)
- ✅ Ollama embedding client (nomic-embed-text compatible)
- ✅ Static/dynamic fact types for context injection
- ✅ Entity versioning (version chains, is_latest flag)
- ✅ sqlx for struct scanning (eliminates manual SQL parsing)
- ✅ goose for database migrations (versioned, idempotent)

**Phase 3**: Intelligence (Future)
- Auto-context injection at session start
- Importance scoring for memories
- Decay/consolidation of old memories
- Cross-session continuity

**Completed (Phase 2)**:
- ✅ Plugin installation and end-to-end testing (integration tests in `test/integration/`)
- ✅ Drop-in replacement for JSON Memory MCP verified (all tools + `docs/MIGRATION_GUIDE.md`)
- ✅ Wire `CreateOrUpdateEntity` to MCP for versioning (`create_or_update_entities` tool)
- ✅ Add `get_context` MCP tool for fact-type-aware retrieval

## Go Conventions

<!-- AUTO-MANAGED: conventions -->
**Error handling**:
- Return `ErrNotFound` for missing entities (defined in `entity.go`)
- Return `ErrEntityExists` for duplicate entity creation
- Wrap errors with context: `fmt.Errorf("failed to X: %w", err)`
- Check `sql.ErrNoRows` and convert to domain error

**sqlx patterns**:
- Use `db.Get(&struct, query)` for single-row queries
- Use `db.Select(&slice, query)` for multi-row queries
- Add `db:"column_name"` tags to structs for column mapping
- Column aliases in SQL must match db tags

**Migrations (goose)**:
- Go migrations in `internal/storage/migrations/`
- All migrations must be idempotent (check before alter)
- Use `goose.AddMigrationContext()` in init()
- Run with `store.Migrate()` or `store.MigrateWithLogging()`

**Transaction safety**:
- Use `defer tx.Rollback()` immediately after `Begin()`
- Explicit `tx.Commit()` on success
- Pattern: Begin → defer Rollback → operations → Commit

**Testing**:
- Table-driven tests in `*_test.go` files
- Integration tests use real SQLite (not mocks)
- Test files in same package (`storage_test`)
- `ExpectedMigrationCount` constant for migration tests
<!-- END AUTO-MANAGED -->

## Design Principles

See `docs/DESIGN_DECISIONS.md` for detailed rationale:

1. **SQLite over alternatives**: Zero config, single file, battle-tested, FTS5 + sqlite-vec extensions
2. **Go over TypeScript/Python/Rust**: Single binary, fast startup, excellent SQLite bindings, cross-compilation
3. **FTS5 first, vectors second**: 80% value with 20% effort, validates architecture before complexity
4. **Ollama for embeddings**: Local privacy, simple setup, model flexibility, already common
5. **Hybrid search**: Exact matches + semantic understanding, tunable weights
6. **Drop-in API compatibility**: Zero migration effort, reversible, existing workflows preserved

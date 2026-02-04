# Claude Memory

A local, privacy-first RAG memory system for Claude Code, built on SQLite.

## Why This Exists

### The Problem

Claude Code sessions are ephemeral. Valuable context—patterns learned, decisions made, codebase knowledge—disappears when a session ends. Current solutions have trade-offs:

| Solution | Issue |
|----------|-------|
| **Memory MCP (JSON)** | No search capability, linear scan, no semantic understanding |
| **Cloud services (supermemory)** | Privacy concerns, subscription cost, external dependency |
| **Manual notes** | Friction, inconsistent, not queryable |

### The Solution

A **local SQLite database** with:
- **Full-text search** (FTS5) for keyword queries
- **Vector search** (sqlite-vec) for semantic retrieval
- **Hybrid ranking** combining both approaches
- **MCP interface** for seamless Claude Code integration

### Design Principles

1. **Privacy first**: All data stays local. No cloud, no telemetry, no third parties.
2. **Single file portability**: One `memory.db` file contains everything. `cp` to backup, `scp` to sync.
3. **Incremental complexity**: Start simple (FTS5), add vectors when needed.
4. **Drop-in replacement**: Same MCP API as existing Memory server.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       Claude Code                                │
│                    mcp__memory__* tools                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Claude Memory MCP Server                      │
│                         (this project)                           │
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   Create    │  │   Search    │  │   Retrieve              │  │
│  │   Entities  │  │   (hybrid)  │  │   (by name/relation)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         SQLite                                   │
│                                                                  │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐   │
│  │   Entities    │  │  Observations │  │    Relations      │   │
│  │   (nodes)     │  │  (properties) │  │    (edges)        │   │
│  └───────────────┘  └───────────────┘  └───────────────────┘   │
│                                                                  │
│  ┌───────────────┐  ┌───────────────────────────────────────┐   │
│  │    FTS5       │  │         sqlite-vec (Phase 2)          │   │
│  │  (keywords)   │  │         (semantic vectors)            │   │
│  └───────────────┘  └───────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼ (Phase 2)
┌─────────────────────────────────────────────────────────────────┐
│                    Ollama (local embeddings)                     │
│                    nomic-embed-text / mxbai-embed-large          │
└─────────────────────────────────────────────────────────────────┘
```

## Roadmap

### Phase 1: Foundation ✅ Complete
- [x] SQLite schema for knowledge graph
- [x] FTS5 full-text search (BM25 ranking)
- [x] MCP server with standard Memory API (JSON-RPC 2.0 over stdio)
- [x] CLI tool for database management
- [x] Claude Code plugin structure (hooks, agents, skills, commands)

### Phase 2: Semantic Search
- [ ] sqlite-vec integration
- [ ] Ollama embedding generation
- [ ] Hybrid search (keyword + vector)
- [ ] Relevance tuning

### Phase 3: Intelligence
- [ ] Auto-context injection at session start
- [ ] Importance scoring for memories
- [ ] Decay/consolidation of old memories
- [ ] Cross-session continuity

## Quick Start

```bash
# Clone and build
cd ~/dev/private/claude-memory
make build-all

# Register the MCP server with Claude Code
claude mcp add --scope user --transport stdio mark42 -- \
  /path/to/claude-memory/bin/claude-memory-server

# Verify server is connected
# Use /mcp command in Claude Code to check status

# Restart Claude Code - mcp__mark42__* tools now available
```

**Note**: Plugin `.mcp.json` files are NOT auto-discovered by Claude Code. You must use `claude mcp add` to register MCP servers explicitly. The server config is stored in `~/.claude.json`.

## API Compatibility

This server implements the same tools as `@modelcontextprotocol/server-memory`:

| Tool | Description |
|------|-------------|
| `create_entities` | Create nodes in the knowledge graph |
| `create_relations` | Create edges between nodes |
| `add_observations` | Add properties to existing nodes |
| `delete_entities` | Remove nodes and their relations |
| `delete_observations` | Remove specific observations |
| `delete_relations` | Remove edges |
| `read_graph` | Retrieve the entire graph |
| `search_nodes` | **Enhanced**: FTS5 + vector search |
| `open_nodes` | Retrieve specific nodes by name |

## Comparison

| Feature | JSON Memory MCP | This Project | supermemory |
|---------|----------------|--------------|-------------|
| Storage | JSON file | SQLite | Cloud |
| Privacy | ✅ Local | ✅ Local | ❌ Third-party |
| Keyword search | ❌ None | ✅ FTS5/BM25 | ✅ Yes |
| Semantic search | ❌ None | ✅ sqlite-vec | ✅ Yes |
| Query performance | O(n) | O(log n) | O(log n) |
| Portability | ✅ Single file | ✅ Single file | ❌ Cloud-locked |
| Cost | Free | Free | Subscription |
| Dependencies | None | SQLite | API key |

## License

MIT

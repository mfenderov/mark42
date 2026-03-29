# mark42

A local, privacy-first RAG memory system for Claude Code, built on SQLite with Go.

## Why This Exists

Claude Code sessions are ephemeral. Valuable context—patterns learned, decisions made, codebase knowledge—disappears when a session ends.

| Solution | Issue |
|----------|-------|
| **Memory MCP (JSON)** | No search, linear scan, no semantic understanding |
| **Cloud services** | Privacy concerns, subscription cost, external dependency |
| **Manual notes** | Friction, inconsistent, not queryable |

**mark42** is a local SQLite database with:
- **Full-text search** (FTS5) for keyword queries
- **Vector search** (Ollama embeddings) for semantic retrieval
- **Hybrid ranking** (RRF fusion) combining both approaches
- **Session capture & recall** for cross-session continuity
- **MCP interface** for seamless Claude Code integration

## Installation

```bash
claude plugin install mark42@mark42
```

That's it. The MCP server registers automatically. The binary downloads on first Claude Code start (~30s one-time). All subsequent starts are instant.

### Updating

```bash
claude plugin update mark42@mark42
```

### Migration from brew

If you previously installed via brew:

```bash
claude mcp remove mark42 --scope user
claude plugin install mark42@mark42
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Claude Code                           │
│                    mcp__mark42__* tools                      │
└──────────────────────────┬──────────────────────────────────┘
                           │ JSON-RPC 2.0 (stdio)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                    mark42-server (Go)                        │
│                                                             │
│  Knowledge Graph    Session Capture    Context Injection     │
│  (entities, obs,    (capture, recall,  (importance, recency, │
│   relations)         events, summary)   project boost)       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     SQLite (memory.db)                       │
│                                                             │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐     │
│  │  Entities   │  │ Observations │  │   Relations    │     │
│  │  (nodes)    │  │ (properties) │  │   (edges)      │     │
│  └─────────────┘  └──────────────┘  └────────────────┘     │
│                                                             │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐     │
│  │    FTS5     │  │  Embeddings  │  │   Archive      │     │
│  │  (keywords) │  │  (vectors)   │  │   (decayed)    │     │
│  └─────────────┘  └──────────────┘  └────────────────┘     │
└─────────────────────────────────────────────────────────────┘
                           │ (optional)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│              Ollama (local embeddings)                       │
│              nomic-embed-text / mxbai-embed-large            │
└─────────────────────────────────────────────────────────────┘
```

## MCP Tools (16 total)

| Tool | Description |
|------|-------------|
| `create_entities` | Create nodes in the knowledge graph |
| `create_or_update_entities` | Create or update with versioning support |
| `create_relations` | Create edges between nodes |
| `add_observations` | Add properties with optional fact types |
| `delete_entities` | Remove nodes (cascades to observations/relations) |
| `delete_observations` | Remove specific observations |
| `delete_relations` | Remove edges |
| `read_graph` | Retrieve the entire graph |
| `search_nodes` | Hybrid search: FTS5 + vector (RRF fusion) |
| `open_nodes` | Retrieve specific nodes by name |
| `get_context` | Importance-ranked memories for context injection |
| `get_recent_context` | Recency-first retrieval for mid-session use |
| `summarize_entity` | Entity summary with observations, relations, history |
| `consolidate_memories` | Deduplicate similar observations |
| `capture_session` | Capture session summary + tool-use events |
| `recall_sessions` | Recall recent session summaries for continuity |

## CLI

```bash
# Entity management
mark42 entity create "Go Conventions" "pattern" --obs "Use table-driven tests"
mark42 entity get "Go Conventions"
mark42 entity list --type pattern
mark42 search "testing patterns"

# Session management
echo '{"summary":"Built auth module","events":[...]}' | mark42 session capture my-project
mark42 session list --project my-project
mark42 session recall my-project --hours 72

# Embeddings & search
mark42 embed generate          # Generate vector embeddings via Ollama
mark42 hybrid-search "testing" # FTS5 + vector hybrid search

# Maintenance
mark42 importance recalculate  # Update importance scores
mark42 decay archive           # Archive old, low-importance memories
mark42 context --project my-project  # Preview context injection output
```

## Plugin Hooks

mark42 includes Claude Code plugin hooks for automatic memory management:

| Hook | Trigger | Action |
|------|---------|--------|
| `mark42 hook session-start` | Session begins | Injects session recall + knowledge graph context |
| `mark42 hook post-tool-use` | After Edit/Write/Bash | Tracks modified files + session events (zero tokens) |
| `mark42 hook stop` | Session ends | Triggers `capture_session` + memory sync |

## Comparison

| Feature | JSON Memory MCP | mark42 | supermemory |
|---------|----------------|--------|-------------|
| Storage | JSON file | SQLite | Cloud |
| Privacy | ✅ Local | ✅ Local | ❌ Third-party |
| Keyword search | ❌ None | ✅ FTS5/BM25 | ✅ Yes |
| Semantic search | ❌ None | ✅ Ollama embeddings | ✅ Yes |
| Session continuity | ❌ None | ✅ Capture & recall | ❌ None |
| Importance scoring | ❌ None | ✅ Recency + centrality | ❌ None |
| Query performance | O(n) | O(log n) | O(log n) |
| Portability | ✅ Single file | ✅ Single file | ❌ Cloud-locked |
| Cost | Free | Free | Subscription |

## Roadmap

- **Phase 1** ✅ Foundation — SQLite schema, FTS5 search, MCP server, CLI, plugin structure
- **Phase 2** ✅ Semantic Search — Hybrid search (FTS5 + vector), Ollama embeddings, fact types, entity versioning
- **Phase 3** ✅ Intelligence — Auto-embed on write, recency-boosted context injection, consolidation
- **Phase 4** ✅ Session Capture & Recall — Cross-session continuity, capture/recall tools, hook integration
- **Phase 5** 🔮 Analytics & Advanced Decay — Automatic importance decay, memory analytics

## License

MIT

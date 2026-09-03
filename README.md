# mark42

A local, privacy-first memory layer for AI coding harnesses, built on SQLite with Go.

## Why This Exists

AI coding sessions are ephemeral. Whether you're working in Claude Code, pi, or opencode, valuable context—patterns learned, decisions made, codebase knowledge—disappears when a session ends.

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
- **MCP interface + harness adapters** for Claude Code, pi, and opencode

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

### Other harnesses

mark42 is harness-agnostic. See [`adapters/`](adapters/README.md) for setup: opencode (JS plugin, capture + recall) and pi (MCP recall, capture deferred).

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│          AI Harness (Claude Code / pi / opencode)           │
│          mcp__mark42__* tools / mark42 CLI hooks            │
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

## MCP Tools (20 total)

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
| `invalidate_observation` | Mark an observation as no longer valid (temporal) |
| `get_entity_history` | Full observation history, including superseded |
| `get_memory_analytics` | Aggregate stats: overview, decay curve, access hotspots, activity |
| `get_tuning_recommendation` | Usage-driven importance/decay config suggestions with rationale |

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

# Analytics
mark42 analytics               # Dashboard: overview, decay curve, hotspots, activity
mark42 analytics tune          # Usage-driven config suggestions (add --apply to persist)
```

## Harness Integration

**Claude Code** — plugin hooks for automatic memory management:

| Hook | Trigger | Action |
|------|---------|--------|
| `mark42 hook session-start` | Session begins | Injects session recall + knowledge graph context |
| `mark42 hook post-tool-use` | After Edit/Write/Bash | Tracks modified files + session events (zero tokens) |
| `mark42 hook stop` | Session ends | Triggers `capture_session` + memory sync |

**opencode** — JS plugin adapter (`adapters/opencode/`), capture + recall. **pi** — MCP recall adapter (`adapters/pi/`), capture deferred.

## Analytics

`mark42 analytics` shows a dashboard of overview counts, a decay curve by memory age, top-accessed observations, dormant/fact-type breakdown, and recent session activity (`--json` for machine-readable output, `--top N` to control the hotspot list size). `mark42 analytics tune` compares your persisted importance/decay config against usage-driven suggestions, with a rationale per changed parameter; add `--apply` to persist the suggestion.

```bash
$ mark42 analytics
Memory Analytics
  Entities: 42   Observations: 310   Relations: 18   Sessions: 12
  Embedding coverage: 91%   DB size: 3.2 MB

Decay Curve
  0-7d    120 obs  avg importance 0.82
  8-30d    90 obs  avg importance 0.61
  31d+     100 obs  avg importance 0.34
...

$ mark42 analytics tune --apply
Tuning Recommendation
  DecayConstant    30 → 45   (low overall access frequency favors a slower decay)
Applied. New config saved.
```

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
- **Phase 5** ✅ Cross-Harness & Lifecycle — Harness adapters (Claude Code, pi, opencode), neutral config paths (`~/.mark42`), distill pipeline, importance scoring, decay/archive commands, per-project workdirs
- **Phase 6** ✅ Analytics — Memory analytics dashboard (`mark42 analytics`), usage-driven tuning recommendations (`mark42 analytics tune`), persisted importance/decay config, read-only MCP tools (`get_memory_analytics`, `get_tuning_recommendation`)

## License

MIT

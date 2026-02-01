# Design Decisions

This document captures the reasoning behind key architectural and implementation choices.

## 1. Why SQLite?

### Decision
Use SQLite as the storage backend instead of:
- JSON files (current Memory MCP)
- PostgreSQL/MySQL (traditional RDBMS)
- Specialized vector databases (ChromaDB, Pinecone, Weaviate)

### Rationale

| Alternative | Why Not |
|-------------|---------|
| **JSON files** | O(n) search, no indexing, must load entire graph |
| **PostgreSQL** | Requires running server, complex setup, overkill for single-user |
| **ChromaDB** | Python dependency, separate process, less portable |
| **Pinecone** | Cloud service, privacy concerns, cost |
| **Weaviate** | Heavy, requires Docker, complex configuration |

**SQLite advantages:**
1. **Zero configuration**: No server to run, just a file
2. **Single file**: Trivial backup (`cp`), sync (`scp`), versioning
3. **Battle-tested**: 20+ years of production use, incredibly reliable
4. **Extensions**: FTS5 (full-text) and sqlite-vec (vectors) provide RAG capabilities
5. **Performance**: Fast enough for millions of records on a single machine
6. **Portability**: Works on every platform, no dependencies

### Trade-offs Accepted
- Single-writer limitation (acceptable for personal use)
- No built-in replication (acceptable for local-first design)
- Limited concurrent connections (acceptable, not multi-user)

---

## 2. Why Go for the MCP Server?

### Decision
Implement the MCP server in Go instead of:
- TypeScript (common for MCP servers)
- Python (has good SQLite/ML libraries)
- Rust (maximum performance)

### Rationale

| Language | Pros | Cons |
|----------|------|------|
| **TypeScript** | MCP SDK available, familiar | Node runtime required, larger bundle |
| **Python** | Excellent ML/embedding libraries | Runtime required, slower startup |
| **Rust** | Best performance, smallest binary | Longer development time, complex |
| **Go** | Single binary, fast startup, good SQLite support | Less ML ecosystem |

**Go advantages for this project:**
1. **Single binary**: No runtime dependencies, easy distribution
2. **Fast startup**: MCP servers should start instantly
3. **Excellent SQLite bindings**: `modernc.org/sqlite` is pure Go
4. **Concurrency model**: Goroutines handle async embedding well
5. **Cross-compilation**: Easy to build for different platforms
6. **Author familiarity**: Matches existing skill set

### Trade-offs Accepted
- Will need to shell out to Ollama for embeddings (not a Go library)
- Less MCP ecosystem support than TypeScript
- Manual JSON-RPC implementation (no official Go MCP SDK)

---

## 3. Why FTS5 First, Vectors Second?

### Decision
Phase 1 uses only FTS5 full-text search. Vector search is Phase 2.

### Rationale

**Tidy First principle**: Make the simple improvement first, then add complexity.

**FTS5-only provides:**
- 80% of the value with 20% of the effort
- No external dependencies (Ollama not required)
- Instant search performance improvement over JSON
- Validates the architecture before adding vectors

**Vector search adds:**
- Semantic understanding ("refactoring" finds "code cleanup")
- Better recall for conceptual queries
- But requires: Ollama, embedding model, more storage

**Risk mitigation:**
- If FTS5 is "good enough," vectors may never be needed
- If vectors prove essential, the architecture supports adding them
- No wasted effort either way

---

## 4. Why Ollama for Embeddings?

### Decision
Use Ollama for local embeddings instead of:
- OpenAI embeddings API
- Voyage AI
- Sentence-transformers (Python)
- Built-in embedding model

### Rationale

| Option | Privacy | Latency | Setup | Quality |
|--------|---------|---------|-------|---------|
| **OpenAI** | ❌ Cloud | 100-200ms | API key | Excellent |
| **Voyage AI** | ❌ Cloud | 100-150ms | API key | Excellent |
| **sentence-transformers** | ✅ Local | 30-50ms | Python env | Good |
| **Ollama** | ✅ Local | 50-100ms | `brew install` | Good |

**Ollama advantages:**
1. **Privacy**: All embeddings generated locally
2. **Simple setup**: `brew install ollama && ollama pull nomic-embed-text`
3. **Model flexibility**: Can swap models without code changes
4. **Already common**: Many users already have Ollama installed
5. **HTTP API**: Simple REST interface, no SDK needed

### Trade-offs Accepted
- Slightly lower embedding quality than OpenAI/Voyage
- Requires Ollama running (optional dependency)
- ~50-100ms latency per embedding (acceptable)

---

## 5. Why Hybrid Search?

### Decision
Combine FTS5 (keyword) and vector (semantic) search with weighted score fusion.

### Rationale

Neither approach alone is sufficient:

| Query Type | FTS5 | Vector |
|------------|------|--------|
| Exact term: "TDD" | ✅ Perfect | ⚠️ May miss |
| Concept: "testing approach" | ❌ Misses "TDD" | ✅ Finds related |
| Name lookup: "konfig project" | ✅ Perfect | ⚠️ Fuzzy |
| Semantic: "code quality" | ❌ Too broad | ✅ Relevant results |

**Hybrid benefits:**
- Exact matches rank highest (user expectation)
- Semantic matches fill gaps
- Tunable weights for different use cases

**Implementation:**
```
combined_score = (keyword_score × 0.3) + (vector_score × 0.7)
```

The 0.3/0.7 weighting favors semantic search while ensuring exact matches surface.

---

## 6. Why Drop-in API Compatibility?

### Decision
Implement the exact same tool API as `@modelcontextprotocol/server-memory`.

### Rationale

1. **Zero migration effort**: Replace server, keep workflows
2. **Existing hooks work**: `memory-mcp-sync.sh` unchanged
3. **CLAUDE.md unchanged**: No instruction updates needed
4. **Reversible**: Can switch back to JSON Memory if issues arise
5. **Testing baseline**: Can compare behavior directly

### Implementation
Same tool names, same parameter shapes, same response formats:
- `create_entities`
- `create_relations`
- `add_observations`
- `delete_entities`
- `delete_observations`
- `delete_relations`
- `read_graph`
- `search_nodes` (enhanced with better search)
- `open_nodes`

---

## 7. Why Not Use LangChain/LlamaIndex?

### Decision
Build custom retrieval logic instead of using RAG frameworks.

### Rationale

| Framework | Pros | Cons |
|-----------|------|------|
| **LangChain** | Feature-rich, many integrations | Python, heavy, over-abstracted |
| **LlamaIndex** | Good for RAG specifically | Python, complex, overkill |
| **Custom** | Minimal, focused, no dependencies | More initial work |

**Custom advantages:**
1. **Simplicity**: We need ~500 lines, not a framework
2. **No Python dependency**: Go binary is self-contained
3. **Full control**: Can optimize for our specific use case
4. **Debuggability**: Simple code is easier to debug
5. **Performance**: No framework overhead

**Merciless Simplification principle**: Don't add abstraction layers we don't need.

---

## 8. Why Single Database File?

### Decision
Store everything in one `memory.db` file instead of:
- Separate files per entity type
- Separate databases per project
- Embedded vs. external storage split

### Rationale

1. **Atomic operations**: Transactions span all tables
2. **Simple backup**: One file to copy
3. **Simple sync**: One file to transfer
4. **Consistent state**: No split-brain scenarios
5. **WAL mode**: Concurrent reads with single writer

**File location**: `~/.claude/memory.db` (same directory as other Claude Code state)

---

## 9. Why Incremental Migration Path?

### Decision
Support importing existing Memory MCP JSON data.

### Rationale

1. **Preserve investment**: Don't lose existing memories
2. **Low-risk adoption**: Can migrate and verify before committing
3. **Fallback option**: Keep JSON as backup during transition

**Migration command:**
```bash
claude-memory migrate --from ~/.claude/memory.json --to ~/.claude/memory.db
```

---

## 10. Future Considerations

### Considered but Deferred

| Feature | Reason to Defer |
|---------|-----------------|
| **Multi-user support** | Not needed for personal use |
| **Network API** | Local-only is simpler and safer |
| **Web UI** | CLI/MCP interface sufficient |
| **Auto-compaction** | Wait to see if memory grows too large |
| **Importance scoring** | Need usage data to design properly |

### Open Questions

1. **Embedding on write vs. lazy**: Generate embeddings immediately or on first search?
2. **Memory decay**: Should old memories be deprioritized or archived?
3. **Context window budget**: How many memories to inject at session start?
4. **Cross-project isolation**: Separate databases or filtered views?

These will be addressed based on real usage patterns.

---

## Decision Log

| Date | Decision | Status |
|------|----------|--------|
| 2025-02-01 | SQLite as storage backend | ✅ Decided |
| 2025-02-01 | Go for MCP server | ✅ Decided |
| 2025-02-01 | FTS5 first, vectors Phase 2 | ✅ Decided |
| 2025-02-01 | Ollama for embeddings | ✅ Decided |
| 2025-02-01 | Hybrid search strategy | ✅ Decided |
| 2025-02-01 | Drop-in API compatibility | ✅ Decided |
| TBD | Embedding timing (eager vs lazy) | 🔄 Open |
| TBD | Memory decay strategy | 🔄 Open |
| TBD | Context injection budget | 🔄 Open |

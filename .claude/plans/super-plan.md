# Claude Memory Super Plan - Phase 2 & Beyond

## Research Sources

| Project | Stars | Key Learnings |
|---------|-------|---------------|
| **supermemory** | 16k | Cloud-first memory, static/dynamic facts, memory versioning, decay/TTL |
| **docker/cagent** | - | Local RAG system, BM25+embedding hybrid, RRF fusion, chunking, reranking |

---

## Phase 2: Semantic Search & Smart Context

### 2.1 Hybrid Search (from cagent RAG)

**Goal**: Combine FTS5 keyword search with vector similarity search.

**Implementation** (port from `cagent/pkg/rag/`):

1. **Add sqlite-vec extension** for vector storage
   - Store embeddings in `observations` table
   - Add `embedding BLOB` column for F32 vectors

2. **Implement RRF Fusion** (from `cagent/pkg/rag/fusion/rrf.go`)
   ```
   score(d) = Σ(1 / (k + rank(d)))  where k=60
   ```
   - FTS5 provides keyword results (ranked by BM25)
   - sqlite-vec provides semantic results (ranked by cosine similarity)
   - RRF combines both into final ranking

3. **Embedding Generation**
   - Use Ollama locally (`nomic-embed-text` or `mxbai-embed-large`)
   - Batch embedding support (from `cagent/pkg/rag/embed/`)
   - Store embeddings lazily (generate on first search if missing)

**Files to create**:
- `internal/storage/embedding.go` - Ollama integration
- `internal/storage/vector.go` - sqlite-vec operations
- `internal/storage/fusion.go` - RRF hybrid search

---

### 2.2 Static vs Dynamic Facts (from supermemory)

**Goal**: Distinguish between persistent preferences and session-bound context.

**Schema changes**:
```sql
ALTER TABLE observations ADD COLUMN fact_type TEXT DEFAULT 'dynamic';
-- Values: 'static' (permanent), 'dynamic' (session), 'session_turn'
```

**Context injection format** (from supermemory's `format-context.js`):
```
<mark42-context>
## User Profile (Persistent)
- Prefers TypeScript over JavaScript
- Uses Bun as package manager

## Recent Context
- Working on authentication flow
- Debugging OAuth callback issue
</mark42-context>
```

**Files to modify**:
- `internal/storage/store.go` - Add fact_type column
- `hooks/session-start.py` - Format context sections

---

### 2.3 Memory Versioning (from supermemory)

**Goal**: Track when facts are updated rather than creating duplicates.

**Schema changes**:
```sql
ALTER TABLE entities ADD COLUMN supersedes_id INTEGER REFERENCES entities(id);
ALTER TABLE entities ADD COLUMN is_latest BOOLEAN DEFAULT TRUE;
ALTER TABLE entities ADD COLUMN version INTEGER DEFAULT 1;
```

**Behavior**:
- When creating entity with existing name, create new version
- Mark previous as `is_latest = FALSE`
- Set `supersedes_id` to previous entity ID
- Queries filter by `is_latest = TRUE` by default

**Files to modify**:
- `internal/storage/entity.go` - Version chain logic
- `internal/mcp/handlers.go` - Handle versioning in create_entities

---

### 2.4 Search Result Improvements

**Goal**: Show similarity scores and improve formatting.

**Changes**:
- Return BM25 score from FTS5 queries
- Display percentage match in results
- Deduplicate results before returning

**Files to modify**:
- `internal/storage/search.go` - Add score to results
- `cmd/memory/main.go` - Format score display

---

## Phase 3: Intelligence

### 3.1 Memory Decay/TTL (from supermemory)

**Goal**: Let temporary context expire automatically.

**Schema changes**:
```sql
ALTER TABLE observations ADD COLUMN importance REAL DEFAULT 1.0;
ALTER TABLE observations ADD COLUMN forget_after TIMESTAMP;
ALTER TABLE observations ADD COLUMN last_accessed TIMESTAMP;
```

**Decay algorithm**:
```
importance = initial_importance * decay_factor ^ days_since_access
```
- When `importance < threshold`, mark as `is_forgotten = TRUE`
- Or use explicit `forget_after` for hard TTL

**Files to create**:
- `internal/storage/decay.go` - Decay calculations
- `cmd/memory/gc.go` - Garbage collection command

---

### 3.2 Transcript Capture (from supermemory)

**Goal**: Auto-extract memories from session transcripts.

**Implementation** (from supermemory's `transcript-formatter.js`):
1. Parse session transcript on Stop hook
2. Track last captured UUID to avoid duplicates
3. Format user/assistant turns
4. Extract actionable items and decisions
5. Store as `session_turn` fact type

**Files to modify**:
- `hooks/stop.py` - Add transcript parsing
- Create tracker file: `~/.claude/memory/trackers/{session_id}.txt`

---

### 3.3 Codebase Indexing (from supermemory)

**Goal**: Index project structure into memory.

**New skill**: `/mark42:index`
- Walk project directory
- Extract: file structure, conventions, architecture
- Chunk code files using treesitter (from cagent)
- Store as searchable memories

**Files to create**:
- `skills/index/SKILL.md` - Index skill definition
- `internal/indexer/indexer.go` - Code indexing logic

---

### 3.4 Document Chunking (from cagent)

**Goal**: Properly chunk large documents for search.

**Implementation** (from `cagent/pkg/rag/chunk/`):
- Chunk by tokens (not characters)
- Preserve sentence boundaries
- Track chunk index within document
- File hash for change detection

**Files to create**:
- `internal/chunk/chunk.go` - Chunking logic
- `internal/storage/document.go` - Document/chunk storage

---

## Phase 4: Advanced Features

### 4.1 Reranking (from cagent)

**Goal**: Re-score search results for better relevance.

**Implementation** (from `cagent/pkg/rag/rerank/`):
- LLM-based reranking option
- Cross-encoder scoring
- Threshold filtering

### 4.2 Memory Graph Visualization (from supermemory)

**Goal**: Visualize knowledge graph.

**Implementation**:
- Export graph as JSON/DOT format
- Generate D3-force layout data
- Create simple HTML viewer

### 4.3 Multi-Project Memory Scoping (from supermemory container tags)

**Goal**: Scope memories by project.

**Schema changes**:
```sql
ALTER TABLE entities ADD COLUMN container_tag TEXT;
-- Format: "cwd:{path}" or "project:{name}"
```

---

## Implementation Order

**Sprint 1 (Week 1-2): Semantic Search Foundation**
1. Add sqlite-vec extension
2. Implement Ollama embedding integration
3. Implement RRF fusion for hybrid search
4. Add similarity scores to search results

**Sprint 2 (Week 3-4): Smart Context**
1. Add static/dynamic fact types
2. Improve context injection formatting
3. Add memory versioning
4. Implement deduplication

**Sprint 3 (Week 5-6): Auto-Capture**
1. Implement transcript parsing in Stop hook
2. Add decay/TTL support
3. Create `/mark42:index` skill
4. Add chunking for large documents

**Sprint 4 (Week 7-8): Polish**
1. Add reranking option
2. Create graph visualization
3. Add multi-project scoping
4. Documentation and testing

---

## Critical Files Summary

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 2.1 | `embedding.go`, `vector.go`, `fusion.go` | `search.go`, `store.go` |
| 2.2 | - | `store.go`, `session-start.py` |
| 2.3 | - | `entity.go`, `handlers.go` |
| 3.1 | `decay.go`, `gc.go` | `store.go` |
| 3.2 | `trackers/` | `stop.py` |
| 3.3 | `skills/index/`, `indexer.go` | - |

---

## Verification Plan

1. **Unit tests**: Each new file gets `*_test.go`
2. **Integration test**: Full search flow with hybrid results
3. **E2E test**: Session start → work → stop → restart → context restored
4. **Benchmark**: Compare search speed vs JSON Memory MCP

---

## Dependencies to Add

```go
// go.mod additions
require (
    github.com/asg017/sqlite-vec-go-bindings v0.1.0  // Vector search
    github.com/ollama/ollama v0.1.0                  // Embedding client
)
```

---

## Decisions Made

1. **Scope**: Full Phase 2 - Hybrid Search + Static/Dynamic Facts + Memory Versioning
2. **RRF k parameter**: Use cagent's default of 60
3. **Embedding model**: Start with `nomic-embed-text` (768 dims, fast)
4. **Fact types**: `static`, `dynamic`, `session_turn`

## Immediate Next Steps (Phase 2 Sprint 1)

1. **Add sqlite-vec extension** to go.mod
2. **Port cagent's RRF fusion** from `pkg/rag/fusion/rrf.go`
3. **Create embedding.go** for Ollama integration
4. **Modify schema** for fact_type and versioning columns
5. **Update search.go** for hybrid search with scores
6. **Update session-start.py** for sectioned context injection

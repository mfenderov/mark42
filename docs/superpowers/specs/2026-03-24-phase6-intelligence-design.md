# Phase 6: Core Intelligence Design

## Overview

Phase 6 transforms mark42 from "storage with search" into an actual memory engine. It adds four complementary capabilities that make the knowledge graph self-correcting, temporally aware, and context-sensitive.

**Goal**: Every observation gets a real importance score (H2.6), stale facts expire automatically (H2.3), near-duplicate observations are detected by meaning not just substring (H2.4), and context injection is query-aware rather than always returning the same top-N by importance (H2.5).

**Approach**: Schema-first ordering. H2.6 and H2.3 add new columns to `observations` first; H2.4 and H2.5 build on that foundation. This avoids retrofitting validity filters onto features built before the schema stabilises.

**Implementation order**: H2.6 → H2.3 → H2.4 → H2.5

---

## H2.6: Fix Phantom Access Tracking

### Problem

`internal/storage/importance.go:185` passes hardcoded `0` for `access_count` to `RecalculateImportance`. The frequency component (30% of the importance formula) has been architecturally wired in since Phase 3 but has never contributed to actual scores.

### Schema Change

**Migration 009** (`internal/storage/migrations/009_add_access_count.go`):
```sql
ALTER TABLE observations ADD COLUMN access_count INTEGER DEFAULT 0
```
Idempotent — check `pragma table_info(observations)` before altering.

### Implementation

**`internal/storage/importance.go`**:
- `UpdateLastAccessed` gains `access_count = access_count + 1` in the UPDATE statement
- `RecalculateImportance` reads actual `access_count` via a SELECT per entity before scoring, replacing the hardcoded `0`

**Call sites that track access** (add `UpdateLastAccessed` call after retrieval):
- `internal/mcp/handlers.go` — `searchNodes` (after loading entity observations)
- `internal/mcp/handlers.go` — `getContext` / `getRecentContext` (after building result set)
- `internal/mcp/handlers.go` — `openNodes` (after loading entities)

### Test Coverage
- `TestCalculateFrequencyScore` — already exists, covers the pure function
- `TestUpdateLastAccessed_IncrementsAccessCount` — new test verifying the counter increments
- `TestRecalculateImportance_UsesRealAccessCount` — new test verifying frequency contributes to score

---

## H2.3: Temporal Fact Validity

### Problem

Old and new facts about the same topic coexist silently. "Uses Go 1.22" and "Uses Go 1.25" are both returned as valid memories.

### Schema Change

**Migration 010** (`internal/storage/migrations/010_add_temporal_columns.go`):
```sql
ALTER TABLE observations ADD COLUMN valid_from  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
ALTER TABLE observations ADD COLUMN valid_until TIMESTAMP DEFAULT NULL
```
All existing observations are implicitly valid (`valid_until IS NULL`). Idempotent — check before altering.

### Validity Filter

All read queries that return observations gain the filter:
```sql
AND (o.valid_until IS NULL)
```

**Affected files** (explicit list):
- `internal/storage/context.go` — `GetContextForInjection`, `GetRecentContext`
- `internal/storage/search.go` — `SearchWithLimit`, `ftsSearch`
- `internal/storage/hybrid.go` — inline SQL in `HybridSearch`
- `internal/storage/entity.go` — `GetEntity` (observations loaded for an entity)
- `internal/storage/importance.go` — `RecalculateImportance` candidate query

**Exempt**: Session observations (`fact_type IN ('session_event', 'session_summary')`) — these are append-only event logs. Temporal validity does not apply; they are never invalidated.

Session exemption applies only to **cross-entity queries** (context injection, search, hybrid search) that join across all entity types. These queries use `AND (o.valid_until IS NULL OR e.entity_type = 'session')`. Single-entity queries that already scope to a non-session entity (e.g., `RecalculateImportance`, which operates on one entity at a time) simply use `AND (o.valid_until IS NULL)` — the session exemption clause is unnecessary there.

### Auto-Detection on Write

When `AddObservationWithType` is called with a non-session fact type and an embedder is registered on the Store:

1. Generate embedding for the new observation
2. Load existing valid observations for the entity that have stored embeddings
3. For each existing observation, compute cosine similarity against the new one
4. If similarity > 0.85 (configurable threshold), expire the old observation: `UPDATE observations SET valid_until = CURRENT_TIMESTAMP WHERE id = ?`
5. The new observation is inserted normally

**Fallback**: If no embedder, or if no embeddings exist for existing observations, skip auto-detection — observations accumulate (manual invalidation via `invalidate_observation` tool).

**Threshold**: 0.85 default, exposed in `Store` config but not as an MCP param (YAGNI).

### New Storage Functions (`internal/storage/temporal.go`)

```go
// InvalidateObservation expires a specific observation by content.
func (s *Store) InvalidateObservation(entityName, content string) error

// GetEntityHistory returns all observations including expired ones, ordered by valid_from DESC.
func (s *Store) GetEntityHistory(entityName string) ([]ObservationWithValidity, error)

// ObservationWithValidity adds temporal fields to an observation.
type ObservationWithValidity struct {
    Content    string
    FactType   string
    ValidFrom  time.Time
    ValidUntil sql.NullTime  // Valid=false means still valid (valid_until IS NULL in DB)
}
```

Auto-detection logic lives in `temporal.go` as `detectAndExpireSuperseded(tx, entityID int64, newContent string, embedder Embedder) error`, called from `AddObservationWithType`.

### New MCP Tools

**`invalidate_observation`** — LLM explicitly expires a fact:
```json
{ "entityName": "string", "content": "string" }
```

**`get_entity_history`** — shows temporal evolution:
```json
{ "entityName": "string" }
```
Returns all observations (valid and expired) with timestamps.

### Test Coverage
- Migration idempotency test (extend existing `ExpectedMigrationCount`)
- `TestInvalidateObservation` — verifies observation is no longer returned by `GetEntity`
- `TestGetEntityHistory` — verifies expired observations appear in history
- `TestAutoDetectSuperseded_WithEmbedder` — verifies high-similarity write triggers invalidation
- `TestAutoDetectSuperseded_NoEmbedder` — verifies fallback accumulates without crashing
- `TestValidityFilter_ExemptsSessions` — verifies session observations are unaffected

---

## H2.4: Semantic Consolidation

### Problem

Current `ConsolidateObservations` only catches substring containment. "Uses Go for backend" and "Backend written in Go" are both kept because neither contains the other.

### Design

Extend `consolidate_memories` MCP tool with an optional `mode` parameter:

| Mode | Behaviour |
|------|-----------|
| `"substring"` (default) | Current logic — backward compatible |
| `"semantic"` | Embedding-based pairwise similarity |

**Semantic mode algorithm** (`internal/storage/consolidate.go`):

`ConsolidateWithSimilarity` is a standalone function — it does not call `ConsolidateObservations` as a subroutine. It handles all observations itself:

1. Load all valid observations for the entity (respects H2.3's `valid_until IS NULL`)
2. For each observation, fetch its stored embedding from the `embeddings` table
3. For pairs **with embeddings**: compute cosine similarity; pairs above threshold are duplicates
4. For pairs **without embeddings** (one or both missing): fall back to substring containment check (same logic as `ConsolidateObservations`)
5. For each duplicate pair: keep the longer observation (or the more recently accessed if equal length); expire the other via `InvalidateObservation` (H2.3's temporal machinery — no hard delete)

**No LLM merge step** — YAGNI. Expiry + keep-longest is sufficient.

### New Storage Function

```go
// ConsolidateWithSimilarity consolidates observations using embedding cosine similarity.
// Falls back to substring matching for observations without stored embeddings.
func (s *Store) ConsolidateWithSimilarity(entityName string, threshold float64) (string, error)
```

### MCP Changes

`consolidate_memories` input schema gains two optional fields:
```json
{
  "entityName": "string",
  "mode": "substring | semantic",      // default: "substring"
  "threshold": 0.85                     // optional, default: 0.85, semantic mode only
}
```

### Test Coverage
- `TestConsolidateWithSimilarity_SemanticMode` — with fake embedder, verifies near-duplicate pair triggers expiry
- `TestConsolidateWithSimilarity_NoEmbeddings` — verifies fallback to substring when no embeddings stored
- `TestConsolidateWithSimilarity_KeepsLonger` — verifies the correct observation is retained

---

## H2.5: Query-Aware Context Injection

### Problem

`get_context` always returns the same memories (top-N by importance × recency) regardless of what the user is working on. A user debugging Kubernetes gets the same context as when they're writing Go tests.

### Design

Add optional `query` string to `get_context`. Two code paths:

**With query (new path)**:
1. Run `HybridSearchWithEmbedder` if embedder available, else `SearchWithLimit` (FTS only)
2. Collect up to 3× token budget worth of candidates from search results
3. Re-rank by `importance × recency_boost` (same formula as current flat path)
4. Apply token budget to re-ranked list
5. Only valid observations are candidates (H2.3's filter applies via hybrid search)

**Without query (existing path)**:
- Unchanged — `GetContextForInjection` with flat importance ranking, no regression

**No MMR** (Maximal Marginal Relevance) in this phase — YAGNI. Simple relevance × importance is sufficient to validate the concept; diversity can be added in Phase 8.

### Storage Changes (`internal/storage/context.go`)

`GetContextForInjection` gains a `query string` parameter. When non-empty, the function:
1. Delegates candidate selection to `HybridSearch` or `Search`
2. Re-scores candidates with the existing importance × recency formula
3. Returns results within token budget

### MCP Changes (`internal/mcp/handlers.go`)

`get_context` input schema gains one optional field:
```json
{
  "projectName": "string",
  "tokenBudget": 2000,
  "minImportance": 0.3,
  "query": "string"   // new, optional
}
```

### Hook Change (`cmd/memory/hook_session_start.go`)

The hook calls `store.GetContextForInjection` directly (not via MCP JSON-RPC). The change is: pass `projectName` as the new `query` parameter in the existing `GetContextForInjection` call at line ~98. No JSON marshalling or MCP client code is needed.

### Test Coverage
- `TestGetContextForInjection_WithQuery` — verifies query-matching observations score higher than non-matching high-importance observations
- `TestGetContextForInjection_NoQuery_Unchanged` — verifies existing behavior is unaffected
- `TestGetContextForInjection_NoEmbedder_FallsBackToFTS` — verifies FTS path when no embedder

---

## File Map

| File | Change |
|------|--------|
| `internal/storage/migrations/009_add_access_count.go` | New migration |
| `internal/storage/migrations/010_add_temporal_columns.go` | New migration |
| `internal/storage/importance.go` | Increment access_count, read real count in RecalculateImportance |
| `internal/storage/temporal.go` | New: InvalidateObservation, GetEntityHistory, auto-detection |
| `internal/storage/context.go` | Add validity filter, add query param to GetContextForInjection |
| `internal/storage/search.go` | Add validity filter |
| `internal/storage/hybrid.go` | Add validity filter |
| `internal/storage/entity.go` | Add validity filter to observation loading |
| `internal/storage/consolidate.go` | Add ConsolidateWithSimilarity |
| `internal/mcp/handlers.go` | UpdateLastAccessed call sites, new tools, new params |
| `cmd/memory/hook_session_start.go` | Pass project name as query |

**New files**: `temporal.go`, `009_add_access_count.go`, `010_add_temporal_columns.go`

---

## Migration Sequence

```
008_add_session_fact_types  (existing, Phase 4)
009_add_access_count        (H2.6)
010_add_temporal_columns    (H2.3)
```

`ExpectedMigrationCount` in `store_test.go` must be updated to 10.

---

## Error Handling

- Auto-detection failure (embedding error) → log warning, skip invalidation, insert observation normally. Never block writes.
- `InvalidateObservation` with unknown entity or content → return `ErrNotFound`
- Semantic consolidation with no embedder → return error with clear message: "semantic mode requires embedder"

---

## Non-Goals (Phase 6)

- LLM merge step for consolidated observations (Phase 7)
- MMR diversity in context injection (Phase 8)
- Temporal reasoning across multiple entities (Phase 7/8)
- `valid_from` query parameter (future — expose only `valid_until IS NULL` path for now)

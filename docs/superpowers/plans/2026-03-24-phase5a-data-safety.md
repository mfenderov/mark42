# Phase 5A: Data Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate all data-corruption risks in mark42 by wrapping multi-step writes in transactions, preventing SQL injection in context queries, and fixing embedder lifecycle so unavailable Ollama doesn't silently degrade search performance.

**Architecture:** SQLite WAL mode with sqlx. All multi-step write operations (archive, consolidate, complete-session, recalculate-importance) need `db.Beginx()` / `defer tx.Rollback()` / `tx.Commit()` wrappers. FTS5 triggers fire within transactions automatically — no special handling needed. SQL injection fix uses an allowlist guard before string interpolation into ORDER BY.

**Tech Stack:** Go, `github.com/jmoiron/sqlx` (`sqlx.Tx` for transactions, `db.Beginx()`), `modernc.org/sqlite` (pure Go SQLite), `go test -race ./...`

**Note on TDD for Tasks 1–4:** These are hardening fixes — crash-scenario atomicity cannot be unit-tested without process kill signals. Tasks 1–4 add/verify state-consistency tests (both invariants of the operation hold) that serve as regression guards. Task 6 (SQL injection) is the only proper RED→GREEN test in this plan.

---

## File Map

| File | Changes |
|------|---------|
| `internal/storage/decay.go:84-121` | Wrap INSERT+DELETE in transaction |
| `internal/storage/consolidate.go:51-56` | Replace per-call DeleteObservation loop with tx-batched DELETEs |
| `internal/storage/session.go:73-98` | Inline SQL, wrap INSERT+UPDATE in transaction |
| `internal/storage/importance.go:128-203` | Load rows into memory first, then tx-batch UPDATEs |
| `internal/storage/context.go:43` | Replace `formatInt(i+1)` with `strconv.Itoa(i+1)`, add `validFactTypes` guard |
| `cmd/server/main.go:48-57` | Skip `WithEmbedder` on connectivity failure, add `defer cancel()` |
| `internal/storage/decay_test.go` | Add atomic archive state-consistency test |
| `internal/storage/consolidate_test.go` | Add all-duplicates-removed test |
| `internal/storage/importance_test.go` | Add RecalculateImportance consistency test |
| `internal/storage/context_test.go` | Add SQL injection prevention test (RED before fix) |

---

## Task 1: Atomic ArchiveOldMemories

**Problem:** `internal/storage/decay.go:84-121` — `INSERT INTO archived_observations` then `DELETE FROM observations` as two separate statements. A crash between them leaves the observation in both tables (duplicated data). Wrapping in a transaction makes the pair atomic.

**Files:**
- Modify: `internal/storage/decay.go:84-121`
- Test: `internal/storage/decay_test.go`

- [ ] **Step 1: Write the state-consistency test**

Add to `internal/storage/decay_test.go`. This test verifies the correct final state: observation must be in the archive AND removed from the main table after a successful archive operation.

```go
func TestStore_ArchiveOldMemories_AtomicArchiveAndDelete(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	store.CreateEntity("OldFact", "test", []string{"This is stale"})
	store.SetObservationImportance("OldFact", "This is stale", 0.05)

	// Backdate the observation so it qualifies for archival
	store.DB().Exec(`
		UPDATE observations
		SET created_at = datetime('now', '-100 days'),
		    last_accessed = datetime('now', '-100 days')
		WHERE content = 'This is stale'
	`)

	cfg := storage.DecayConfig{
		ArchiveAfterDays:    90,
		MinImportanceToKeep: 0.1,
	}

	archived, err := store.ArchiveOldMemories(cfg)
	if err != nil {
		t.Fatalf("ArchiveOldMemories failed: %v", err)
	}
	if archived != 1 {
		t.Fatalf("expected 1 archived, got %d", archived)
	}

	// Invariant 1: observation removed from main observations table
	entity, _ := store.GetEntity("OldFact")
	if len(entity.Observations) != 0 {
		t.Errorf("observation should be removed from main table, got: %v", entity.Observations)
	}

	// Invariant 2: observation present in archive table
	archiveCount, _ := store.GetArchiveCount()
	if archiveCount != 1 {
		t.Errorf("observation should be in archive table, got count: %d", archiveCount)
	}
}
```

- [ ] **Step 2: Run test to confirm expected state (baseline)**

```bash
cd /Users/mark.fenderov/dev/private/claude-memory
go test ./internal/storage/... -run TestStore_ArchiveOldMemories_AtomicArchiveAndDelete -v -race
```

Expected: PASS (documents correct behavior; becomes regression guard after wrapping)

- [ ] **Step 3: Wrap ArchiveOldMemories in a transaction**

Replace `internal/storage/decay.go:84-121` with:

```go
func (s *Store) ArchiveOldMemories(cfg DecayConfig) (int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -cfg.ArchiveAfterDays)
	cutoffStr := cutoffDate.Format("2006-01-02 15:04:05")

	tx, err := s.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf("beginning archive transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO archived_observations (original_entity_id, entity_name, content, fact_type, importance, archived_at)
		SELECT o.entity_id, e.name, o.content, o.fact_type, o.importance, datetime('now')
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
		AND o.importance < ?
		AND COALESCE(o.last_accessed, o.created_at) < ?
		AND o.fact_type != 'static'
	`, cfg.MinImportanceToKeep, cutoffStr)
	if err != nil {
		return 0, err
	}

	archived, _ := result.RowsAffected()
	if archived == 0 {
		return 0, nil
	}

	if _, err = tx.Exec(`
		DELETE FROM observations
		WHERE id IN (
			SELECT o.id FROM observations o
			JOIN entities e ON e.id = o.entity_id
			WHERE e.is_latest = 1
			AND o.importance < ?
			AND COALESCE(o.last_accessed, o.created_at) < ?
			AND o.fact_type != 'static'
		)
	`, cfg.MinImportanceToKeep, cutoffStr); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing archive transaction: %w", err)
	}

	return int(archived), nil
}
```

Note: `cutoffStr` is computed once and reused in both SQL statements so the cutoff is identical regardless of clock ticks between calls.

Also ensure `decay.go` imports `"fmt"`. Check the existing import block — add `"fmt"` if not present.

- [ ] **Step 4: Run full storage test suite**

```bash
go test ./internal/storage/... -race -v 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/decay.go internal/storage/decay_test.go
git commit -m "fix: wrap ArchiveOldMemories INSERT+DELETE in transaction"
```

---

## Task 2: Atomic ConsolidateObservations

**Problem:** `internal/storage/consolidate.go:51-56` — calls `DeleteObservation(entityName, obs)` in a loop. Each deletion is a separate autocommit transaction. A failure midway leaves the entity with some duplicates removed and others still present.

**Files:**
- Modify: `internal/storage/consolidate.go:51-57`
- Test: `internal/storage/consolidate_test.go`

- [ ] **Step 1: Write the state-consistency test**

Add to `internal/storage/consolidate_test.go` (package `storage_test`):

```go
func TestStore_ConsolidateObservations_AllDuplicatesRemoved(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// "Go" is a substring of all three "Go is X" observations — all 3 shorter ones qualify
	// but only "Go" itself should be removed (it's contained in the others, not the reverse)
	store.CreateEntity("Lang", "language", []string{
		"Go",
		"Go is fast",
		"Go is safe",
		"Go is concurrent",
		"Has generics",
	})

	_, err := store.ConsolidateObservations("Lang")
	if err != nil {
		t.Fatalf("ConsolidateObservations failed: %v", err)
	}

	entity, _ := store.GetEntity("Lang")

	// "Go" is a substring of all three "Go is X" obs — must be removed
	for _, obs := range entity.Observations {
		if obs == "Go" {
			t.Error(`"Go" is contained in the longer observations and should be removed`)
		}
	}

	// The 3 "Go is X" observations + "Has generics" must remain
	if len(entity.Observations) != 4 {
		t.Errorf("expected 4 observations after consolidation, got %d: %v",
			len(entity.Observations), entity.Observations)
	}
}
```

- [ ] **Step 2: Run test to confirm baseline passes**

```bash
go test ./internal/storage/... -run TestStore_ConsolidateObservations_AllDuplicatesRemoved -v -race
```

Expected: PASS

- [ ] **Step 3: Replace DeleteObservation loop with transaction**

Replace `internal/storage/consolidate.go:51-57` (the deletion loop and return statement) with:

```go
	// Delete duplicate observations atomically.
	// FTS5 triggers fire inside the transaction — observations_fts stays in sync.
	tx, err := s.db.Beginx()
	if err != nil {
		return "", fmt.Errorf("beginning consolidation transaction: %w", err)
	}
	defer tx.Rollback()

	deleted := 0
	for _, obs := range uniqueDeletes {
		result, execErr := tx.Exec(
			"DELETE FROM observations WHERE entity_id = ? AND content = ?",
			entity.ID, obs,
		)
		if execErr == nil {
			if n, _ := result.RowsAffected(); n > 0 {
				deleted++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing consolidation transaction: %w", err)
	}

	return fmt.Sprintf("%s: consolidated %d redundant observations (kept %d)",
		entityName, deleted, len(observations)-deleted), nil
```

Key points:
- Uses `entity.ID` (already loaded by `GetEntity` at line 13 — no extra lookup needed)
- FTS5 `DELETE` triggers on `observations` fire within the transaction automatically
- Replaces the `s.DeleteObservation()` loop (each of which does its own SELECT+DELETE pair) with direct SQL within the transaction

- [ ] **Step 4: Run full storage test suite**

```bash
go test ./internal/storage/... -race -v 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/consolidate.go internal/storage/consolidate_test.go
git commit -m "fix: wrap ConsolidateObservations deletions in single transaction"
```

---

## Task 3: Atomic CompleteSession

**Problem:** `internal/storage/session.go:73-98` — `AddObservationWithType` (INSERT) and `SetContainerTag` (UPDATE) are separate autocommit operations. A failure between them leaves the session with a summary observation but status still "active".

**Files:**
- Modify: `internal/storage/session.go:73-98`
- Test: `internal/storage/session_test.go` (existing test covers this)

- [ ] **Step 1: Run existing test as baseline**

`TestCompleteSession` at `session_test.go:100-133` already verifies both invariants:
- `s.Status == "completed"`
- `s.Summary` matches what was passed

Run it to confirm it passes before the refactor:

```bash
go test ./internal/storage/... -run TestCompleteSession -v -race
```

Expected: PASS — this is the regression guard for this task

- [ ] **Step 2: Rewrite CompleteSession with explicit transaction**

Replace `internal/storage/session.go:73-98` entirely with:

```go
func (s *Store) CompleteSession(sessionName, summary string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var entityID int64
	if err := tx.QueryRow(
		"SELECT id FROM entities WHERE name = ? AND is_latest = 1",
		sessionName,
	).Scan(&entityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("getting session entity: %w", err)
	}

	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)",
		entityID, summary, string(FactTypeSessionSummary),
	); err != nil {
		return fmt.Errorf("storing session summary: %w", err)
	}

	var tag string
	if err := tx.QueryRow(
		"SELECT COALESCE(container_tag, '') FROM entities WHERE id = ?",
		entityID,
	).Scan(&tag); err != nil {
		return fmt.Errorf("reading session metadata: %w", err)
	}

	var meta SessionMetadata
	if tag != "" {
		if err := json.Unmarshal([]byte(tag), &meta); err != nil {
			return fmt.Errorf("parsing session metadata: %w", err)
		}
	}
	meta.Status = "completed"
	meta.EndedAt = time.Now().Format(time.RFC3339)

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling session metadata: %w", err)
	}

	if _, err := tx.Exec(
		"UPDATE entities SET container_tag = ? WHERE id = ?",
		string(metaJSON), entityID,
	); err != nil {
		return fmt.Errorf("updating session metadata: %w", err)
	}

	return tx.Commit()
}
```

Add two imports to `session.go` import block (current imports: `"encoding/json"`, `"fmt"`, `"time"`):
- `"database/sql"` — for `sql.ErrNoRows`
- `"errors"` — for `errors.Is`

- [ ] **Step 3: Run existing test to confirm refactor didn't break behavior**

```bash
go test ./internal/storage/... -run TestCompleteSession -v -race
```

Expected: PASS

- [ ] **Step 4: Run full storage test suite**

```bash
go test ./internal/storage/... -race -v 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/session.go
git commit -m "fix: wrap CompleteSession summary+status writes in transaction"
```

---

## Task 4: Atomic RecalculateImportance

**Problem:** `internal/storage/importance.go:128-203` — iterates observations and calls individual `db.Exec("UPDATE observations SET importance = ?")` outside any transaction. A partial failure leaves inconsistent importance scores across the graph. Additionally, holding an open `db.Query` rows cursor while running writes in the same connection can cause locking issues in SQLite.

**Files:**
- Modify: `internal/storage/importance.go:128-203`
- Test: `internal/storage/importance_test.go`

- [ ] **Step 1: Write the state-consistency test**

Add to `internal/storage/importance_test.go` (package `storage_test`):

```go
func TestStore_RecalculateImportance_UpdatesAllObservations(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	store.CreateEntity("Entity1", "test", []string{"Observation one"})
	store.CreateEntity("Entity2", "test", []string{"Observation two"})
	store.CreateEntity("Entity3", "test", []string{"Observation three"})

	store.SetObservationImportance("Entity1", "Observation one", 1.0)
	store.SetObservationImportance("Entity2", "Observation two", 1.0)
	store.SetObservationImportance("Entity3", "Observation three", 1.0)

	// Backdate all observations so recency decay will reduce importance by >0.01
	store.DB().Exec(`
		UPDATE observations
		SET created_at = datetime('now', '-60 days'),
		    last_accessed = datetime('now', '-60 days')
	`)

	updated, err := store.RecalculateImportance()
	if err != nil {
		t.Fatalf("RecalculateImportance failed: %v", err)
	}

	// All 3 observations must have been updated (60-day decay causes >0.01 delta)
	if updated != 3 {
		t.Errorf("expected 3 observations updated, got %d", updated)
	}
}
```

- [ ] **Step 2: Run test as baseline**

```bash
go test ./internal/storage/... -run TestStore_RecalculateImportance_UpdatesAllObservations -v -race
```

Expected: PASS

- [ ] **Step 3: Restructure RecalculateImportance to load-then-transact**

Replace `internal/storage/importance.go:128-203` with:

```go
func (s *Store) RecalculateImportance() (int, error) {
	cfg := DefaultImportanceConfig()

	var maxRelations int
	if err := s.db.Get(&maxRelations, `
		SELECT COALESCE(MAX(rel_count), 0)
		FROM (
			SELECT COUNT(*) as rel_count
			FROM relations
			GROUP BY from_entity_id
		)
	`); err != nil {
		maxRelations = 1
	}
	if maxRelations == 0 {
		maxRelations = 1
	}

	// Load all observation rows into memory first, then apply updates in a single transaction.
	// This avoids holding an open read cursor while writing — SQLite cursor+tx interplay.
	type obsRow struct {
		ID             int64   `db:"id"`
		BaseImportance float64 `db:"importance"`
		FactType       string  `db:"fact_type"`
		DaysSince      float64 `db:"days_since"`
		RelationCount  int     `db:"relation_count"`
	}

	var rows []obsRow
	if err := s.db.Select(&rows, `
		SELECT o.id, o.importance, o.fact_type,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since,
		       (SELECT COUNT(*) FROM relations WHERE from_entity_id = o.entity_id OR to_entity_id = o.entity_id) as relation_count
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
	`); err != nil {
		return 0, err
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf("beginning importance transaction: %w", err)
	}
	defer tx.Rollback()

	updated := 0
	for _, row := range rows {
		baseScore := row.BaseImportance
		if row.FactType == string(FactTypeStatic) {
			baseScore = math.Max(baseScore, 0.8)
		}

		newImportance := CalculateImportance(
			baseScore,
			row.DaysSince,
			0, // access_count not yet tracked in schema — Phase 6 work
			row.RelationCount,
			maxRelations,
			cfg,
		)

		if math.Abs(newImportance-row.BaseImportance) > 0.01 {
			if _, err := tx.Exec(
				"UPDATE observations SET importance = ? WHERE id = ?",
				newImportance, row.ID,
			); err == nil {
				updated++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing importance transaction: %w", err)
	}

	return updated, nil
}
```

**Required import change for `importance.go`:** The current imports are `"math"` and `"time"`. Add `"fmt"`:
```go
import (
	"fmt"
	"math"
	"time"
)
```

- [ ] **Step 4: Run full storage test suite**

```bash
go test ./internal/storage/... -race -v 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/importance.go internal/storage/importance_test.go
git commit -m "fix: load-then-transact pattern for RecalculateImportance"
```

---

## Task 5: Fix Embedder Lifecycle in MCP Server

**Problem:** `cmd/server/main.go:48-57` — `handler.WithEmbedder(embedder)` is called BEFORE testing connectivity. If Ollama is down, the embedder is attached but broken — every `search_nodes` call attempts embedding generation (~5s timeout), fails, then falls back to FTS. Silent performance degradation per search. Additionally, `cancel()` is called without `defer` — a panic in `CreateEmbedding` would leak the context.

**Files:**
- Modify: `cmd/server/main.go:48-57`

Note: No new unit test — `package main` is not easily unit-tested, and the no-embedder path is exercised by existing handler tests that run with a nil embedder.

- [ ] **Step 1: Fix the embedder lifecycle**

Replace `cmd/server/main.go:48-57` with:

```go
	embedderURL := os.Getenv("CLAUDE_MEMORY_EMBEDDER_URL")
	if embedderURL == "" {
		embedderURL = storage.DefaultOllamaBaseURL()
	}
	if embedderURL != "disabled" {
		embedder := storage.NewEmbeddingClient(embedderURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := embedder.CreateEmbedding(ctx, "test"); err != nil {
			logError("embedder unavailable at %s — semantic search disabled", embedderURL)
		} else {
			handler.WithEmbedder(embedder)
		}
	}
```

Two changes from original:
1. `defer cancel()` — prevents context leak if `CreateEmbedding` panics
2. `handler.WithEmbedder(embedder)` is in the `else` branch — only attaches embedder when the connectivity test succeeds

- [ ] **Step 2: Build to verify compilation**

```bash
go build ./cmd/server/...
```

Expected: no errors

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -race 2>&1 | tail -10
```

Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "fix: only attach embedder when Ollama connectivity test passes, add defer cancel"
```

---

## Task 6: Prevent SQL Injection in GetContextForInjection

**Problem:** `internal/storage/context.go:43` — `FactTypePriority` values are concatenated directly into the ORDER BY clause without validation. `ContextConfig` is an exported struct — any caller can inject SQL via `FactTypePriority = []string{"'; DROP TABLE observations; --"}`.

This is the only task in this plan that follows traditional TDD RED→GREEN: the test fails before the fix because no validation exists yet.

**Files:**
- Modify: `internal/storage/context.go:43`
- Test: `internal/storage/context_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/storage/context_test.go` (package `storage_test`):

```go
func TestGetContextForInjection_RejectsInvalidFactType(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	cfg := storage.DefaultContextConfig()
	cfg.FactTypePriority = []string{"'; DROP TABLE observations; --"}

	_, err := store.GetContextForInjection(cfg, "")
	if err == nil {
		t.Error("expected error for invalid fact type in FactTypePriority, got nil")
	}
}

func TestGetContextForInjection_AcceptsAllValidFactTypes(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	cfg := storage.DefaultContextConfig()
	cfg.FactTypePriority = []string{
		"static", "dynamic", "session_turn", "session_event", "session_summary",
	}

	_, err := store.GetContextForInjection(cfg, "")
	if err != nil {
		t.Errorf("all known fact types should be accepted, got error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to confirm first test FAILS (RED)**

```bash
go test ./internal/storage/... -run TestGetContextForInjection_RejectsInvalidFactType -v -race
```

Expected: **FAIL** — "expected error for invalid fact type in FactTypePriority, got nil"

- [ ] **Step 3: Add allowlist and guard in context.go**

At the top of `internal/storage/context.go` (before `GetContextForInjection`), add the allowlist variable:

```go
// validFactTypes is the complete set of known fact type strings.
// Used to validate FactTypePriority before string interpolation into SQL.
var validFactTypes = map[string]bool{
	"static":          true,
	"dynamic":         true,
	"session_turn":    true,
	"session_event":   true,
	"session_summary": true,
}
```

Then modify `GetContextForInjection` — replace lines 41-44 (the loop that builds `factTypeCases`) with:

```go
	// Validate all fact types before string interpolation into ORDER BY clause
	for _, ft := range cfg.FactTypePriority {
		if !validFactTypes[ft] {
			return nil, fmt.Errorf("invalid fact type in FactTypePriority: %q", ft)
		}
	}

	var factTypeCases []string
	for i, ft := range cfg.FactTypePriority {
		factTypeCases = append(factTypeCases, "WHEN '"+ft+"' THEN "+strconv.Itoa(i+1))
	}
```

Add `"strconv"` and `"fmt"` to the import block in `context.go`. Current imports are only `"math"` and `"strings"` — both new imports must be added explicitly. Do NOT delete `formatInt` — it is still used at `context.go:124` (`GetRecentContext`) and in `session.go:189` (`GetRecentSessionSummaries`). Only the single usage on line 43 is being replaced with `strconv.Itoa`.

- [ ] **Step 4: Run both tests to confirm GREEN**

```bash
go test ./internal/storage/... -run TestGetContextForInjection -v -race
```

Expected: both PASS

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -race 2>&1 | tail -10
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/storage/context.go internal/storage/context_test.go
git commit -m "fix: validate FactTypePriority against allowlist before SQL interpolation"
```

---

## Final Verification

- [ ] **Full test suite with race detector**

```bash
go test -race ./... 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: all `ok`, no `FAIL`

- [ ] **Build all binaries**

```bash
make build-all
```

Expected: both `mark42` and `mark42-server` compile cleanly

- [ ] **Lint**

```bash
make lint
```

Expected: no errors

---

## What's Next

After this plan is complete:

- **Phase 5B: Performance & Cleanup** — N+1 query elimination, dead code removal, server signal handling, CLI decomposition (`main.go` 1783 lines → domain files)
- **Phase 6: Core Intelligence** — Fix phantom access tracking (30% of importance formula is always 0), temporal fact validity, semantic consolidation with embedding similarity, query-aware context injection
- **Phase 7: LLM Integration** — Intelligent session summaries, LLM-powered knowledge extraction (ADD/UPDATE/DELETE/NOOP cycle), GraphRAG-lite graph-expanded search

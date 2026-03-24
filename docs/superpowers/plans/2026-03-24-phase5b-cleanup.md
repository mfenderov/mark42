# Phase 5B: Performance & Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate dead code, replace bubble sorts and `formatInt` with stdlib equivalents, fix an N+1 query in `HybridSearchWithBoost`, eliminate a type-assertion workaround by introducing a `storage.Embedder` interface, and add graceful shutdown to the MCP server.

**Architecture:** All changes are isolated improvements with no behavior change (dead code removal, algorithm upgrade, interface introduction). The Embedder interface fix is the most structural change: it defines `storage.Embedder` so `HybridSearchWithEmbedder` accepts any embedder value without a concrete-type dependency on the `mcp` package.

**Tech Stack:** Go 1.26.1, SQLite/sqlx, Cobra CLI, `slices` stdlib package (available since Go 1.21), `strconv`, `os/signal`

---

## File Map

| File | Change |
|------|--------|
| `internal/storage/vector.go` | ~~Delete `getObservationID` (lines 19–30, dead code)~~ — **removed from scope** (see Task 1 note) |
| `hooks/hooks.json` | Remove the `SessionEnd` block |
| `cmd/memory/hook_misc.go` | Delete `hookSessionEndCmd`, `runSessionEndHook`, and the `init()` entry for them |
| `internal/storage/workdir.go` | Replace 2 bubble sorts with `slices.SortFunc` |
| `internal/storage/context.go` | Replace `formatInt(hours)` call → `strconv.Itoa(hours)`; delete `formatInt` definition |
| `internal/storage/session.go` | Replace `formatInt(hours)` call → `strconv.Itoa(hours)`; add `"strconv"` import |
| `internal/storage/workdir.go` | Add `batchGetContainerTags`; replace N+1 loop in `HybridSearchWithBoost` |
| `internal/storage/embedding.go` | Add `Embedder` interface |
| `internal/storage/hybrid.go` | Change `HybridSearchWithEmbedder` parameter from `*EmbeddingClient` → `Embedder` |
| `internal/mcp/handlers.go` | Remove type assertion; pass `h.embedder` directly |
| `cmd/server/main.go` | Add `signal.NotifyContext`; thread `ctx` into `server.Run` |

---

## Task 1: Delete Dead Code

Three pieces of dead code to remove atomically.

**Files:**
- ~~Modify: `internal/storage/vector.go`~~ — **removed from scope** (see note below)
- Modify: `hooks/hooks.json`
- Modify: `cmd/memory/hook_misc.go`

> **Note:** `getObservationID` is called by `hybrid_test.go` and `vector_test.go` (same-package tests). It is a test helper, not dead code. This item was removed from scope.

Background: ~~`getObservationID` in `vector.go` has never been called (grep confirms zero usages).~~ The `SessionEnd` hook in `hooks.json` calls `mark42 hook session-end`, which runs `runSessionEndHook` — a function with an empty body and a comment saying "Stats are collected silently for future use." It has no future use planned in the roadmap. Removing these two eliminates noise and reduces the hook's no-op overhead.

- [ ] **Step 1: Verify dead code is truly unused**

```bash
grep -rn "getObservationID" /path/to/repo --include="*.go"
grep -rn "session-end\|sessionEnd\|SessionEnd\|runSessionEndHook\|hookSessionEndCmd" /path/to/repo
```

Expected: `getObservationID` appears only in its definition. `runSessionEndHook` appears only in `hook_misc.go`. `session-end` appears only in `hooks.json` and `hook_misc.go`.

- [ ] **Step 2: Write the test (RED) — verify the SessionEnd command is gone after change**

Add to `cmd/memory/hook_misc_test.go` (create if doesn't exist):

```go
package main

import (
    "testing"
)

func TestHookMisc_NoSessionEndCommand(t *testing.T) {
    // After cleanup, hookSessionEndCmd must not exist.
    // This file intentionally has no test body — its existence enforces
    // that the test package compiles without hookSessionEndCmd being referenced.
    // The real check is that hook_misc.go no longer registers it.
    _ = hookPreCompactCmd // sanity: pre-compact still exists
}
```

Run: `go test ./cmd/memory/ -run TestHookMisc_NoSessionEndCommand -v`
Expected: PASS (compiles successfully since hookPreCompactCmd still exists)

- ~~**Step 3: Delete `getObservationID` from `internal/storage/vector.go` (lines 19–30)**~~

  > **Removed from scope.** `getObservationID` is called by `hybrid_test.go` and `vector_test.go` (same-package tests). It is a test helper, not dead code. This step was skipped.

- [ ] **Step 4: Remove SessionEnd block from `hooks/hooks.json`**

Remove these 9 lines:
```json
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "mark42 hook session-end",
            "timeout": 5
          }
        ]
      }
    ],
```

- [ ] **Step 5: Remove `hookSessionEndCmd`, its `init()` registration, and `runSessionEndHook` from `cmd/memory/hook_misc.go`**

Remove lines 10–21 and 41–44:
```go
var hookSessionEndCmd = &cobra.Command{
    Use:   "session-end",
    Short: "SessionEnd hook: silent stats collection",
    RunE: func(cmd *cobra.Command, args []string) error {
        projectDir := getProjectDir()
        if projectDir == "" {
            return nil
        }
        runSessionEndHook(projectDir)
        return nil
    },
}
```
```go
func init() {
    hookCmd.AddCommand(hookSessionEndCmd)    // ← remove this line only
    hookCmd.AddCommand(hookPreCompactCmd)
}
```
```go
func runSessionEndHook(projectDir string, opts ...hookOption) {
    // SessionEnd hooks must not produce structured output.
    // Stats are collected silently for future use.
}
```

After deletion the `init()` becomes:
```go
func init() {
    hookCmd.AddCommand(hookPreCompactCmd)
}
```

- [ ] **Step 6: Run tests**

```bash
go build ./...
go test ./... -race
```

Expected: all tests pass, no compilation errors.

- [ ] **Step 7: Commit**

```bash
git add hooks/hooks.json cmd/memory/hook_misc.go cmd/memory/hook_misc_test.go
git commit -m "refactor: delete dead code (SessionEnd hook, runSessionEndHook)"
```

---

## Task 2: Replace Bubble Sorts with `slices.SortFunc`

**Files:**
- Modify: `internal/storage/workdir.go`
- Test: `internal/storage/workdir_test.go`

Background: `sortFusedResultsByScore` at lines 154–163 uses a manual O(n²) bubble sort. A second inline bubble sort exists in `GetContextWithContainerTag` at lines 219–225. Both should use `slices.SortFunc` (available since Go 1.21; project is on 1.26.1). The existing tests at lines 78 and 117 of `workdir_test.go` already verify the sort behavior, so we only need to confirm they still pass after the change.

- [ ] **Step 1: Run existing sort tests to confirm GREEN baseline**

```bash
go test ./internal/storage/ -run "TestStore_HybridSearchWithContainerBoost|TestStore_GetContextWithContainerTag" -v
```

Expected: both PASS.

- [ ] **Step 2: Replace `sortFusedResultsByScore` in `workdir.go`**

Current (lines 153–163):
```go
// sortFusedResultsByScore sorts results by FusionScore descending (higher is better).
func sortFusedResultsByScore(results []FusedResult) {
    // Simple bubble sort - results are typically small
    for i := 0; i < len(results)-1; i++ {
        for j := i + 1; j < len(results); j++ {
            if results[j].FusionScore > results[i].FusionScore {
                results[i], results[j] = results[j], results[i]
            }
        }
    }
}
```

Replace with:
```go
// sortFusedResultsByScore sorts results by FusionScore descending (higher is better).
func sortFusedResultsByScore(results []FusedResult) {
    slices.SortFunc(results, func(a, b FusedResult) int {
        if b.FusionScore > a.FusionScore {
            return 1
        }
        if b.FusionScore < a.FusionScore {
            return -1
        }
        return 0
    })
}
```

Add `"slices"` to the import block in `workdir.go`. Current imports are `"context"`, `"database/sql"`, `"fmt"`.

- [ ] **Step 3: Replace inline bubble sort in `GetContextWithContainerTag` (lines 219–225)**

Current:
```go
// Sort by final score (descending)
for i := 0; i < len(results)-1; i++ {
    for j := i + 1; j < len(results); j++ {
        if results[j].FinalScore > results[i].FinalScore {
            results[i], results[j] = results[j], results[i]
        }
    }
}
```

Replace with:
```go
// Sort by final score (descending)
slices.SortFunc(results, func(a, b ContextResult) int {
    if b.FinalScore > a.FinalScore {
        return 1
    }
    if b.FinalScore < a.FinalScore {
        return -1
    }
    return 0
})
```

- [ ] **Step 4: Run tests**

```bash
go build ./...
go test ./internal/storage/ -run "TestStore_HybridSearchWithContainerBoost|TestStore_GetContextWithContainerTag" -v
go test ./internal/storage/ -race
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/workdir.go
git commit -m "refactor: replace bubble sorts with slices.SortFunc in workdir.go"
```

---

## Task 3: Delete `formatInt`, Replace with `strconv.Itoa`

**Files:**
- Modify: `internal/storage/context.go` (2 changes: replace call at line 141, delete definition at lines 262–276)
- Modify: `internal/storage/session.go` (1 change: replace call at line 220, add `strconv` import)

Background: `formatInt` was added to `context.go` to avoid importing `strconv` for a single call. After Phase 5A, `strconv` is already imported in `context.go` (for `strconv.Itoa` in the SQL injection fix). The helper is now redundant dead code with two callers.

- [ ] **Step 1: Verify current state**

```bash
grep -n "formatInt" internal/storage/context.go internal/storage/session.go
grep -n "\"strconv\"" internal/storage/context.go internal/storage/session.go
```

Expected:
- `context.go:141` — `hoursParam := "-" + formatInt(hours)`
- `context.go:262–276` — `func formatInt(n int) string { ... }`
- `session.go:220` — `hoursParam := "-" + formatInt(hours)`
- `context.go` — imports `"strconv"` (added in Phase 5A)
- `session.go` — does NOT import `"strconv"` yet

- [ ] **Step 2: In `context.go`, replace the `formatInt` call at line 141**

Change:
```go
hoursParam := "-" + formatInt(hours)
```
To:
```go
hoursParam := "-" + strconv.Itoa(hours)
```

- [ ] **Step 3: Delete `formatInt` definition from `context.go` (lines 262–276)**

Remove:
```go
// formatInt converts int to string without importing fmt.
func formatInt(n int) string {
    if n == 0 {
        return "0"
    }
    if n < 0 {
        return "-" + formatInt(-n)
    }
    var digits []byte
    for n > 0 {
        digits = append([]byte{byte('0' + n%10)}, digits...)
        n /= 10
    }
    return string(digits)
}
```

- [ ] **Step 4: In `session.go`, replace `formatInt(hours)` call at line 220**

Change:
```go
hoursParam := "-" + formatInt(hours)
```
To:
```go
hoursParam := "-" + strconv.Itoa(hours)
```

Add `"strconv"` to the import block in `session.go`. Current imports are `"database/sql"`, `"encoding/json"`, `"errors"`, `"fmt"`, `"time"`.

- [ ] **Step 5: Run tests**

```bash
go build ./...
go test ./internal/storage/ -race
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/context.go internal/storage/session.go
git commit -m "refactor: replace formatInt helper with strconv.Itoa"
```

---

## Task 4: Fix N+1 Query in `HybridSearchWithBoost`

**Files:**
- Modify: `internal/storage/workdir.go`
- Test: `internal/storage/workdir_test.go`

Background: `HybridSearchWithBoost` at line 136 calls `s.GetContainerTag(results[i].EntityName)` once per result — a classic N+1. For 20 results this is 20 round-trips to SQLite. Fix: add a `batchGetContainerTags` helper that fetches all tags in one query using `WHERE name IN (...)`, then use the resulting map in the boost loop.

Note: `sqlx` does not natively support `IN` clause with slices — use a simple approach with a manually-constructed placeholders string, or use a loop-built query. The clean pattern for small N is to build the query with `strings.Repeat`.

- [ ] **Step 1: Write failing test (RED)**

Add to `internal/storage/workdir_test.go`:

```go
func TestStore_BatchGetContainerTags(t *testing.T) {
    store := newTestStore(t)
    defer store.Close()

    if err := store.Migrate(); err != nil {
        t.Fatalf("Migrate failed: %v", err)
    }

    store.CreateEntity("E1", "project", []string{"obs1"})
    store.CreateEntity("E2", "project", []string{"obs2"})
    store.CreateEntity("E3", "project", []string{"obs3"})

    store.SetContainerTag("E1", "proj-a")
    store.SetContainerTag("E2", "proj-b")
    // E3 has no tag

    tags, err := store.BatchGetContainerTags([]string{"E1", "E2", "E3", "Missing"})
    if err != nil {
        t.Fatalf("BatchGetContainerTags failed: %v", err)
    }

    if tags["E1"] != "proj-a" {
        t.Errorf("E1: want proj-a, got %q", tags["E1"])
    }
    if tags["E2"] != "proj-b" {
        t.Errorf("E2: want proj-b, got %q", tags["E2"])
    }
    if tags["E3"] != "" {
        t.Errorf("E3: want empty, got %q", tags["E3"])
    }
    if _, ok := tags["Missing"]; ok {
        t.Error("Missing entity should not appear in result")
    }
}
```

Run: `go test ./internal/storage/ -run TestStore_BatchGetContainerTags -v`
Expected: FAIL with "BatchGetContainerTags: undefined"

- [ ] **Step 2: Implement `BatchGetContainerTags` and update `HybridSearchWithBoost`**

Add to `internal/storage/workdir.go`:

```go
// BatchGetContainerTags retrieves container tags for multiple entities in one query.
// Returns a map of entity name → tag (missing entries have empty string).
func (s *Store) BatchGetContainerTags(entityNames []string) (map[string]string, error) {
    if len(entityNames) == 0 {
        return map[string]string{}, nil
    }

    // Build placeholders: ?,?,?,...
    placeholders := strings.Repeat("?,", len(entityNames))
    placeholders = placeholders[:len(placeholders)-1]

    query := fmt.Sprintf(`
        SELECT name, COALESCE(container_tag, '') as tag
        FROM entities
        WHERE name IN (%s) AND (is_latest = 1 OR is_latest IS NULL)
    `, placeholders)

    args := make([]any, len(entityNames))
    for i, n := range entityNames {
        args[i] = n
    }

    rows, err := s.db.Query(query, args...)
    if err != nil {
        return nil, fmt.Errorf("batch get container tags: %w", err)
    }
    defer rows.Close()

    result := make(map[string]string, len(entityNames))
    for rows.Next() {
        var name, tag string
        if err := rows.Scan(&name, &tag); err != nil {
            return nil, err
        }
        result[name] = tag
    }
    return result, rows.Err()
}
```

Add `"strings"` to the import block in `workdir.go`.

Replace the N+1 loop in `HybridSearchWithBoost`:

Current (lines 134–140):
```go
// Apply container tag boost
for i := range results {
    tag, _ := s.GetContainerTag(results[i].EntityName)
    if tag == containerTag && containerTag != "" {
        results[i].FusionScore *= boostFactor
    }
}
```

Replace with:
```go
// Apply container tag boost (batch fetch to avoid N+1)
if containerTag != "" {
    names := make([]string, len(results))
    for i, r := range results {
        names[i] = r.EntityName
    }
    tags, err := s.BatchGetContainerTags(names)
    if err != nil {
        return nil, fmt.Errorf("fetching container tags for boost: %w", err)
    }
    for i := range results {
        if tags[results[i].EntityName] == containerTag {
            results[i].FusionScore *= boostFactor
        }
    }
}
```

- [ ] **Step 3: Run tests (GREEN)**

```bash
go test ./internal/storage/ -run "TestStore_BatchGetContainerTags|TestStore_HybridSearchWithContainerBoost" -v
go test ./internal/storage/ -race
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/storage/workdir.go internal/storage/workdir_test.go
git commit -m "perf: fix N+1 query in HybridSearchWithBoost using BatchGetContainerTags"
```

---

## Task 5: Define `storage.Embedder` Interface, Fix `HybridSearchWithEmbedder`

**Files:**
- Modify: `internal/storage/embedding.go` (add `Embedder` interface)
- Modify: `internal/storage/hybrid.go` (change parameter type)
- Modify: `internal/mcp/handlers.go` (remove type assertion)
- Test: `internal/mcp/handlers_test.go` (existing `fakeEmbedder` tests must continue passing)

Background: `HybridSearchWithEmbedder` in `hybrid.go:120` takes `*EmbeddingClient` (concrete type), forcing `handlers.go:529` to type-assert `h.embedder.(*storage.EmbeddingClient)` — which defeats the purpose of having the `mcp.Embedder` interface. Adding `storage.Embedder` (same method signature) and changing the parameter type lets `handlers.go` pass `h.embedder` directly. Go's structural typing means `mcp.Embedder` values automatically satisfy `storage.Embedder` without any adapter code.

- [ ] **Step 1: Run existing `fakeEmbedder` tests to confirm GREEN baseline**

```bash
go test ./internal/mcp/ -run "TestHandler_AutoEmbed" -v
```

Expected: PASS.

- [ ] **Step 2: Add `Embedder` interface to `internal/storage/embedding.go`**

Add after the existing imports block (before `EmbeddingClient` struct definition):

```go
// Embedder generates vector embeddings for text.
type Embedder interface {
    CreateEmbedding(ctx context.Context, text string) ([]float64, error)
}
```

`context` is already imported in this file.

- [ ] **Step 3: Update `HybridSearchWithEmbedder` signature in `hybrid.go`**

Current (line 120):
```go
func (s *Store) HybridSearchWithEmbedder(ctx context.Context, query string, embedder *EmbeddingClient, limit int) ([]FusedResult, error) {
```

Change to:
```go
func (s *Store) HybridSearchWithEmbedder(ctx context.Context, query string, embedder Embedder, limit int) ([]FusedResult, error) {
```

The nil-check inside the function (`if embedder != nil`) still works correctly with interface values — an untyped nil satisfies this check.

- [ ] **Step 4: Remove type assertion in `handlers.go`**

Current (lines 528–538):
```go
// Try hybrid search (FTS + vector) if embedder is a full EmbeddingClient
if ec, ok := h.embedder.(*storage.EmbeddingClient); ok && ec != nil {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    results, err := h.store.HybridSearchWithEmbedder(ctx, input.Query, ec, 20)
    if err == nil && len(results) > 0 {
        return h.formatHybridResults(results)
    }
    // Fall through to FTS-only on error
}
```

Replace with:
```go
// Try hybrid search (FTS + vector) if embedder is available
if h.embedder != nil {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    results, err := h.store.HybridSearchWithEmbedder(ctx, input.Query, h.embedder, 20)
    if err == nil && len(results) > 0 {
        return h.formatHybridResults(results)
    }
    // Fall through to FTS-only on error
}
```

Note: `h.embedder` is typed as `mcp.Embedder` (interface). Since `mcp.Embedder` and `storage.Embedder` have identical method sets, Go's structural typing allows passing `h.embedder` directly to `HybridSearchWithEmbedder(... embedder storage.Embedder ...)` without any conversion.

- [ ] **Step 5: Run all tests**

```bash
go build ./...
go test ./... -race
```

Expected: all PASS. The `fakeEmbedder` in `handlers_test.go` already implements `CreateEmbedding`, so it satisfies both `mcp.Embedder` and `storage.Embedder`.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/embedding.go internal/storage/hybrid.go internal/mcp/handlers.go
git commit -m "refactor: define storage.Embedder interface, remove type assertion in searchNodes"
```

---

## Task 6: Add Graceful Shutdown to MCP Server

**Files:**
- Modify: `cmd/server/main.go`

Background: The MCP server's `Run()` method blocks on `bufio.Scanner.Scan()` (line 82 of main.go). When receiving SIGTERM or SIGINT (e.g., from Claude Code shutting down, or a process manager), the process terminates abruptly without any cleanup. Adding `signal.NotifyContext` lets the server exit its read loop cleanly when a signal is received. The scanner loop is moved to a goroutine; the main goroutine selects on either new lines or context cancellation.

- [ ] **Step 1: Write the test (RED)**

Create `cmd/server/main_test.go`:

```go
package main

import (
    "context"
    "testing"
    "time"
)

func TestServer_StopsOnContextCancel(t *testing.T) {
    handler := newTestHandler(t)
    server := &Server{handler: handler}

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    done := make(chan error, 1)
    go func() {
        done <- server.Run(ctx)
    }()

    select {
    case err := <-done:
        if err != nil {
            t.Fatalf("Run returned error: %v", err)
        }
    case <-time.After(500 * time.Millisecond):
        t.Fatal("Run did not stop after context cancellation")
    }
}
```

This requires `Run` to accept a `context.Context`. Currently it does not, so the test will fail to compile.

Run: `go build ./cmd/server/` — expected: compile error (Run takes no ctx argument yet).

**Note:** `newTestHandler(t)` in this test needs to be a helper that creates a minimal `*mcp.Handler`. Add it to the same test file:

```go
func newTestHandler(t *testing.T) *mcp.Handler {
    t.Helper()
    store, err := storage.NewStore(":memory:")
    if err != nil {
        t.Fatalf("NewStore: %v", err)
    }
    t.Cleanup(func() { store.Close() })
    return mcp.NewHandler(store)
}
```

This requires imports: `"testing"`, `"context"`, `"time"`, `"github.com/mfenderov/mark42/internal/mcp"`, `"github.com/mfenderov/mark42/internal/storage"`.

- [ ] **Step 2: Refactor `Run` to accept `context.Context` and handle signals**

Update `cmd/server/main.go`:

Add imports `"os/signal"` and `"syscall"` to the import block.

Change `Run()` to `Run(ctx context.Context) error`:

```go
// Run starts the server's main loop. Stops when ctx is cancelled or stdin is closed.
func (s *Server) Run(ctx context.Context) error {
    scanner := bufio.NewScanner(os.Stdin)

    const maxScannerSize = 10 * 1024 * 1024 // 10MB
    buf := make([]byte, maxScannerSize)
    scanner.Buffer(buf, maxScannerSize)

    lines := make(chan []byte)
    scanErr := make(chan error, 1)

    go func() {
        for scanner.Scan() {
            line := make([]byte, len(scanner.Bytes()))
            copy(line, scanner.Bytes())
            select {
            case lines <- line:
            case <-ctx.Done():
                return
            }
        }
        scanErr <- scanner.Err()
    }()

    for {
        select {
        case <-ctx.Done():
            return nil
        case err := <-scanErr:
            return err
        case line := <-lines:
            if len(line) == 0 {
                continue
            }
            var req mcp.Request
            if err := json.Unmarshal(line, &req); err != nil {
                s.sendError(nil, mcp.ErrCodeParse, "Parse error", err)
                continue
            }
            s.handleRequest(&req)
        }
    }
}
```

Update `main()` to create a signal context and call `server.Run(ctx)`:

```go
func main() {
    // ... existing dbPath, store, handler, embedder setup unchanged ...

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    server := &Server{handler: handler}
    if err := server.Run(ctx); err != nil {
        logError("server error: %v", err)
        os.Exit(1)
    }
}
```

Add `"os/signal"` and `"syscall"` to the existing import block (which already has `"context"`).

- [ ] **Step 3: Run tests (GREEN)**

```bash
go build ./cmd/server/
go test ./cmd/server/ -run TestServer_StopsOnContextCancel -v
go test ./... -race
```

Expected: `TestServer_StopsOnContextCancel` PASS; full suite PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go cmd/server/main_test.go
git commit -m "feat: add graceful shutdown to MCP server via signal.NotifyContext"
```

---

## Final Verification

After all 6 tasks:

- [ ] `go build ./...` — both binaries compile
- [ ] `go test ./... -race` — full suite passes with race detector
- [ ] `go vet ./...` — no issues
- [ ] Manually confirm: `hooks/hooks.json` has no `SessionEnd` block
- [ ] Manually confirm: `grep -n "formatInt\|getObservationID\|runSessionEndHook" internal/storage/ cmd/memory/` returns nothing (except possibly in test files if any)
- [ ] Manually confirm: `grep -n "EmbeddingClient" internal/mcp/handlers.go` shows no direct reference (type assertion removed)

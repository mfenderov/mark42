# PR #10 Bug Fixes Design

**Date:** 2026-04-03
**Branch:** fix/pr10-review-issues (based on feat/npm-distribution)
**Scope:** Fix all critical and important issues identified in PR #10 review

---

## Background

PR #10 adds npm-based plugin distribution and CC memory sync. Three parallel review agents (code-reviewer, silent-failure-hunter, pr-test-analyzer) identified 3 critical bugs, 4 important issues, and several test gaps. This spec covers all of them.

---

## Section 1 — Go: `hook_memory_sync.go`

### 1a. Abort early on unexpected project entity failure

**Problem:** If `store.CreateEntity(projectEntityName)` fails with any error other than `ErrEntityExists`, the code logs a warning and continues into the file loop. All subsequent `CreateRelation` calls silently return `ErrNotFound` (discarded with `_`), producing orphaned entities with no graph connection.

**Fix:** Return early on unexpected errors, aborting the entire sync run.

```go
_, projErr := store.CreateEntity(projectEntityName, "project", nil)
if projErr != nil && !errors.Is(projErr, storage.ErrEntityExists) {
    logger.Warn("failed to create project entity, aborting cc memory sync",
        "entity", projectEntityName, "err", projErr)
    return
}
```

### 1b. Check observation write errors; skip checksum on failure

**Problem:** Both `AddObservationWithType` calls discard errors with `_ =`. The checksum update (`checksums[name] = sum`) is unconditional. If observation storage fails, the file is permanently marked as synced — observations are never retried.

**Fix:** Check both errors. On failure, log a warning and `continue` (skip the checksum update so the file is retried next session).

```go
if mem.Description != "" {
    if err := store.AddObservationWithType(entityName, mem.Description, storage.FactTypeStatic); err != nil {
        logger.Warn("failed to add description observation", "entity", entityName, "err", err)
        continue
    }
}
if mem.Body != "" {
    if err := store.AddObservationWithType(entityName, mem.Body, storage.FactTypeDynamic); err != nil {
        logger.Warn("failed to add body observation", "entity", entityName, "err", err)
        continue
    }
}
```

### 1c. Log `CreateRelation` failure

**Problem:** `_ = store.CreateRelation(...)` silently discards errors.

**Fix:** Check the error and log a warning (not a fatal — orphaned relations are not critical).

```go
if err := store.CreateRelation(entityName, projectEntityName, "belongs_to"); err != nil {
    logger.Warn("failed to create belongs_to relation", "from", entityName, "to", projectEntityName, "err", err)
}
```

---

## Section 2 — Go: `hook_stop.go`

### 2a. Use `defer syncStore.Close()`

**Problem:** `syncStore.Close()` is called directly. If `syncCCMemory` panics, the DB connection leaks. Every other store open in the file uses `defer store.Close()`.

**Fix:** 
```go
if syncStore, err := getStore(); err == nil {
    defer syncStore.Close()  // was: syncStore.Close() at end of block
    syncCCMemory(...)
} else {
    logger.Warn(...)
}
```

---

## Section 3 — Tests: `hook_memory_sync_test.go`

### 3a. Verify body observation in DB

**Problem:** `TestSyncCCMemory/syncs_memory_files_into_store` checks the description observation but not the body (`FactTypeDynamic`). If `AddObservationWithType(body, ...)` were removed, no test would fail.

**Fix:** Add assertion that checks `entity.Observations` contains the body text.

### 3b. Strengthen stop hook integration test

**Problem:** `TestStopHookTriggersCCMemorySync` only checks that the checksum file exists on disk. It does not verify the entity was created in the DB.

**Fix:** After `runStopHook`, open the test DB and call `store.GetEntity("cc-memory/<slug>/Integration test memory")`. Verify it exists and has observations.

---

## Section 4 — Config: `hooks/hooks.json`

### 4a. Reduce `post-tool-use` timeout

**Problem:** All four hook timeouts are 120s. `post-tool-use` fires after every `Edit`/`Write`/`Bash`. On first install, `hook.sh` triggers `npx install-binary` (30–60s download), freezing the UI after every tool call.

**Fix:** `post-tool-use`: 120 → 15 seconds. `session-start` and `stop` keep 120s.

---

## Section 5 — npm: `npm/install.js`

### 5a. WriteStream error path — clean up partial tmpFile

**Problem:** On write-stream error, `file.on('error', cb)` calls back without closing or unlinking the partial file. The file descriptor stays open; the partial file remains on disk.

**Fix:**
```js
file.on('error', (writeErr) => {
    file.close(() => {
        try { fs.unlinkSync(tmpFile); } catch (_) {}
        cb(writeErr);
    });
});
```

### 5b. tar catch block — unlink tmpFile before exit

**Problem:** When `execFileSync('tar', ...)` throws, the catch block logs and exits without cleaning up `tmpFile`. Repeated install failures accumulate stale tarballs in `os.tmpdir()`.

**Fix:** Add `try { fs.unlinkSync(tmpFile); } catch (_) {}` before `process.exit(1)`.

### 5c. Separate tar/chmod try-catch

**Problem:** Both `execFileSync('tar', ...)` and the two `fs.chmodSync(...)` calls are in the same try block. If chmod fails after tar succeeds, the error message says "Extraction failed" (misleading), and non-executable binaries are left in `nativeDir`, poisoning subsequent installs.

**Fix:** Split into two try blocks:
- Block 1: tar extraction → on error: unlink tmpFile, print "Extraction failed", exit 1
- `fs.unlinkSync(tmpFile)` unconditionally after successful tar
- Block 2: chmod → on error: print "Failed to set binary permissions", exit 1

### 5d. SHA256 checksum verification

**Problem:** The binary is downloaded from GitHub releases without verifying against the goreleaser-generated `checksums.txt`. mark42 runs as a persistent hook with broad filesystem access — a compromised binary has significant blast radius.

**Fix:** Before extraction:
1. Download `checksums.txt` from the same GitHub release
2. Find the line matching `<filename>`
3. Compute SHA256 of the downloaded tarball
4. Compare — exit 1 if mismatch

The download function is reused for `checksums.txt`. Verification happens between download and extraction.

---

## Testing Strategy

- All Go fixes follow TDD: failing test first, then fix
- Section 3 tests are written before the Go fixes (they'll fail until the bugs are corrected)
- npm/install.js has no test harness — changes are structural (separating try blocks, adding cleanup) and verified manually via `npm install --dry-run`
- Full test suite (`go test -race ./...`) must pass before commit

---

## Files Changed

| File | Change |
|------|--------|
| `cmd/memory/hook_memory_sync.go` | Sections 1a, 1b, 1c |
| `cmd/memory/hook_stop.go` | Section 2a |
| `cmd/memory/hook_memory_sync_test.go` | Sections 3a, 3b |
| `hooks/hooks.json` | Section 4a |
| `npm/install.js` | Sections 5a, 5b, 5c, 5d |

# CC Memory → mark42 Sync Design

## Context

Claude Code has a built-in file-based auto-memory system (`~/.claude/projects/<slug>/memory/`) that stores user preferences, project facts, and feedback as individual Markdown files with YAML frontmatter. This data is valuable but has no search capability beyond MEMORY.md line scanning.

mark42 already has FTS5 + vector hybrid search, recency scoring, and session tracking. Syncing CC memory into mark42 unifies the two systems, making CC memories searchable and cross-referenceable with the knowledge graph.

## Scope

- **In scope**: Individual `.md` files with YAML frontmatter (name, description, type) in the current project's `memory/` directory
- **Out of scope**: Inline MEMORY.md content (Format B), agent memory (`agent-memory/`), bidirectional sync (mark42→CC)
- **Sync direction**: CC → mark42 (one-way)
- **Trigger**: Stop hook (end of session)

## Design

### 1. Memory File Discovery

The sync function derives the CC memory path from `CLAUDE_PROJECT_DIR`:

```
CLAUDE_PROJECT_DIR=/Users/mark/dev/private/deutsch
  → slug: -Users-mark-dev-private-deutsch
  → path: ~/.claude/projects/-Users-mark-dev-private-deutsch/memory/
```

**Slug derivation**: Replace `/` with `-` in the absolute path (CC's convention).

**File selection**: Glob `memory/*.md`, skip `MEMORY.md`. Only process files with YAML frontmatter (opening `---` on line 1).

**Checksum-based skip**: Store SHA256 hashes in `.claude/mark42/memory-checksums.json`:

```json
{
  "feedback_hook_debugging.md": "a1b2c3...",
  "project_phase6_complete.md": "d4e5f6..."
}
```

Compare current hash to stored hash. Skip unchanged files. Typical session overhead: ~2ms (directory listing + hash comparison, no DB writes).

### 2. Parsing

Split file at `---` delimiters to extract:
- **Frontmatter**: `name`, `description`, `type` (YAML)
- **Body**: Everything after the closing `---`

No external YAML library needed — the frontmatter is simple enough for line-based parsing (3 key-value pairs, no nesting).

### 3. Entity Mapping

| CC Memory Field | mark42 Field |
|----------------|-------------|
| `name` | Entity name: `cc-memory/<project-slug>/<name>` |
| `type` | Entity type: `feedback`, `project`, `user`, `reference` |
| `description` | Observation with fact_type `static` |
| Body content | Observation with fact_type `dynamic` |

**Entity naming**: Prefixed with `cc-memory/<project-slug>/` to namespace from native mark42 entities.

Example: `cc-memory/claude-memory/Hook debugging with debug mode`

**Relations**: Each memory entity gets a `belongs_to` relation to a project entity (e.g., `project:claude-memory`).

### 4. Upsert Strategy

Use `CreateOrUpdateEntity()`:
- **First sync**: Creates entity with version 1
- **Content changed**: Creates new version (old version kept, marked `is_latest = false`)
- **Content unchanged**: Checksum skip (no DB write)

Observations are added via `AddObservationWithType()`. The `UNIQUE(entity_id, content)` constraint handles deduplication — identical observations are silently skipped.

### 5. Stop Hook Integration

New call added at the end of `runStopHook()`:

```
runStopHook():
  1. Flag guard (existing)
  2. Read session events/dirty files (existing)
  3. Build session digest (existing)
  4. Capture session to SQLite (existing)
  5. Cleanup buffers (existing)
  6. syncCCMemory(projectDir, store)  ← NEW
```

**Error handling**: Silent — log error, don't block stop hook. Session capture is higher priority.

**Timeout**: Well within the 30s budget. Typical: <20ms (nothing changed), worst case: <50ms (3 files to sync).

### 6. New File

`cmd/memory/hook_memory_sync.go`:
- `syncCCMemory(projectDir string, store *storage.Store, opts ...hookOption)` — main entry point
- `projectSlug(projectDir string) string` — path → slug conversion
- `parseCCMemoryFile(path string) (*ccMemory, error)` — frontmatter + body parser
- `loadChecksums(path string) map[string]string` — read checksums JSON
- `saveChecksums(path string, checksums map[string]string)` — write checksums JSON

### 7. Data Types

```go
type ccMemory struct {
    Name        string
    Description string
    Type        string
    Body        string
    FileName    string
}
```

## Files to Create/Modify

| File | Action |
|------|--------|
| `cmd/memory/hook_memory_sync.go` | Create — sync logic, parsing, checksums |
| `cmd/memory/hook_memory_sync_test.go` | Create — unit tests with temp dirs and test DB |
| `cmd/memory/hook_stop.go` | Modify — add `syncCCMemory()` call at end of `runStopHook()` |

## Verification

1. **Unit tests**: Create temp directory with mock CC memory files, run `syncCCMemory`, verify entities created in test SQLite DB
2. **Checksum test**: Run sync twice, verify second run skips all files (no new DB writes)
3. **Version test**: Modify a memory file, run sync, verify new entity version created
4. **Integration**: Run `mark42 hook stop` manually, verify CC memories appear in `mark42 search` results
5. **Full cycle**: Start a CC session, let auto-memory write a file, stop session, verify it appears in mark42

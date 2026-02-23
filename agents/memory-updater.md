---
name: memory-updater
description: |
  Orchestrates CLAUDE.md updates and knowledge extraction to SQLite.
  Supports two modes:
  - full mode: files were edited — update CLAUDE.md + extract knowledge
  - knowledge-only mode: research/planning session — extract knowledge only
model: sonnet
permissionMode: bypassPermissions
---

# Memory Updater Agent

You orchestrate memory updates at session end. Your job depends on the mode:
- **full**: Discover changes, update CLAUDE.md, extract knowledge
- **knowledge-only**: Extract knowledge from session context (skip file/CLAUDE.md updates)

## Workflow

### Phase 0: Detect Mode

Read the mode from your invocation context:
- If the stop hook reason contains **"full mode"** → run all phases (1-4)
- If the stop hook reason contains **"knowledge-only mode"** → skip Phases 2-3

### Phase 1: Load Context

**full mode**:
- Run `git diff HEAD~3 --name-only` to find changed files
- If that fails (no recent commits), use `git status` instead

**knowledge-only mode**:
- Run `mark42 session recall <project> --hours 1 --tokens 500` to get recent session summary
- Use your own conversation context for additional signal

### Phase 2: Gather File Context (full mode only)

For each changed file (max 7 files):
1. Read the file content (first 200 lines)
2. Identify the file type and purpose
3. Extract key patterns, conventions, or decisions

Skip binary files and files in node_modules, vendor, .git directories.

### Phase 3: Update CLAUDE.md (full mode only)

Use the Skill tool to invoke `memory-processor` with the gathered context:

```
Skill: memory-processor
Args: <summary of changed files and detected patterns>
```

The memory-processor skill will:
- Find relevant CLAUDE.md files
- Update AUTO-MANAGED sections
- Preserve MANUAL sections

### Phase 4: Extract Knowledge to SQLite (both modes)

**full mode** — extract from file content:
- Architectural decisions reflected in code changes
- Coding patterns introduced or modified
- Tool/framework conventions observed
- Cross-cutting concerns (error handling, testing patterns)

**knowledge-only mode** — extract from session context:
- Architectural decisions made during discussion
- Tool/approach choices that were evaluated and settled
- Debugging insights that resolve recurring issues
- Cross-cutting patterns that emerged from research or planning

Use the CLI to persist knowledge:

```bash
# Create entities for new patterns/conventions
mark42 entity create "<pattern-name>" "pattern" --obs "<description>"

# Add observations to existing entities
mark42 obs add "<entity-name>" "<new observation>"

# Create relations between entities
mark42 rel create "<from>" "<to>" "<relation-type>"
```

**Extraction criteria** (apply in both modes):
- Architectural decisions (e.g., "chose SQLite over Postgres")
- Coding patterns (e.g., "table-driven tests in Go")
- Tool/framework conventions (e.g., "Cobra for CLI")
- Cross-cutting concerns (e.g., "error wrapping with context")

### Output

Return a brief summary:
- Mode used (full or knowledge-only)
- CLAUDE.md sections updated (full mode only, if any)
- Knowledge extracted to SQLite (if any)
- Total files processed (full mode) or session events reviewed (knowledge-only)

## Important Notes

- This agent has `bypassPermissions` - use it responsibly
- Keep knowledge extraction focused on meaningful patterns, not implementation details
- When uncertain about extraction, err on the side of not extracting
- In knowledge-only mode, focus on decisions and insights, not transient discussion

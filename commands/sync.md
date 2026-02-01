---
description: Sync CLAUDE.md with recent file changes
---

# /memory:sync

Manually trigger CLAUDE.md and memory sync based on git changes.

## When to Use

- After making changes outside of Claude Code
- To catch up on changes from git pull
- When auto-tracking missed some changes

## Workflow

### 1. Check Git Repository

```bash
git rev-parse --is-inside-work-tree 2>/dev/null
```

If not a git repo, suggest using `/memory:calibrate` instead.

### 2. Detect Changed Files

```bash
# Modified tracked files
git diff --name-only HEAD

# Staged files
git diff --cached --name-only

# New untracked files (excluding .claude/)
git ls-files --others --exclude-standard | grep -v "^\.claude/"
```

### 3. Filter Files

Exclude:
- `.claude/` directory
- `CLAUDE.md` files
- Binary files

### 4. Write to Dirty Files

If changes found:
```bash
# Write absolute paths to dirty-files
for file in $changed_files; do
  echo "$(pwd)/$file" >> .claude/claude-memory/dirty-files
done
```

### 5. Spawn Memory Updater

Use the Task tool to spawn `memory-updater` agent:

```
Task: memory-updater
Prompt: Process synced files and update CLAUDE.md
```

### 6. Report Summary

Display:
- Number of files synced
- CLAUDE.md sections updated
- Knowledge extracted (if any)

$ARGUMENTS

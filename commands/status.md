---
description: Show memory system status
---

# /memory:status

Display the current state of the mark42 system.

## Checks to Perform

### 1. Database Status

```bash
mark42 stats
```

Display:
- Database path
- Entity count
- Observation count
- Relation count

### 2. CLAUDE.md Files

Find all CLAUDE.md files:
```bash
find . -name "CLAUDE.md" -type f
```

For each file, show:
- Path
- Last modified time
- AUTO-MANAGED sections present

### 3. Pending Changes

Check dirty files:
```bash
wc -l < .claude/mark42/dirty-files 2>/dev/null || echo "0"
```

If pending changes exist, offer to run `/memory:sync`.

### 4. Configuration

Show trigger mode from `.claude/mark42/config.json`.

## Output Format

```
mark42 Status
====================

Database: ~/.claude/memory.db
  Entities:     32
  Observations: 273
  Relations:    38

CLAUDE.md Files:
  ./CLAUDE.md (last updated: 2 hours ago)
    Sections: build-commands, architecture, conventions

Pending Changes: 0 files

Trigger Mode: default
```

$ARGUMENTS

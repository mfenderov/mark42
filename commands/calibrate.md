---
description: Force recalibration of CLAUDE.md sections
---

# /memory:calibrate

Force a complete recalibration of all CLAUDE.md AUTO-MANAGED sections.

## When to Use

- After major refactoring
- When AUTO-MANAGED sections are stale
- To remove outdated patterns
- After initial setup to fine-tune content

## Workflow

### 1. Find All CLAUDE.md Files

```bash
find . -name "CLAUDE.md" -type f -not -path "./.git/*"
```

### 2. Analyze Each Section

For each CLAUDE.md, read and analyze:
- `build-commands`: Compare against actual config files
- `architecture`: Compare against current directory structure
- `conventions`: Verify patterns still exist in codebase

### 3. Verification Pass

For each documented item, verify it still exists:

```bash
# Example: Check if documented pattern exists
grep -r "<pattern>" . --include="*.go" --exclude-dir=.git
```

### 4. Interactive Review (Optional)

Ask user about questionable items:
- Keep: Item is still relevant
- Remove: Item is outdated
- Update: Item needs modification

### 5. Update Sections

Invoke `memory-processor` skill with calibration context.

### 6. SQLite Cleanup (Optional)

Review memory entities:
```bash
mark42 entity list
```

Offer to remove stale entities that no longer apply.

## Output

Report:
- Sections recalibrated
- Items removed (with reason)
- Items added (with source)
- Entities cleaned up (if any)

$ARGUMENTS

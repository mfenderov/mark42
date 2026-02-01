---
description: Initialize claude-memory for this project
argument-hint: [--gitmode]
---

# /memory:init

Initialize the claude-memory system for this project.

## Steps

### 1. Configure Trigger Mode

Ask the user how auto-memory should trigger:

**Options:**
- `default` (Recommended): Track Edit/Write/Bash operations in real-time
- `gitmode`: Only trigger on git commits (best for frequent committers)

Save to `.claude/claude-memory/config.json`:
```json
{
  "triggerMode": "default"
}
```

### 2. Update .gitignore

Add to `.gitignore`:
```
# Claude Memory
.claude/claude-memory/dirty-files
```

### 3. Initialize Database

Ensure the database exists:
```bash
claude-memory init
```

### 4. Analyze Codebase

Invoke the `codebase-analyzer` skill to:
- Detect project type
- Find build/test commands
- Identify patterns and conventions
- Generate CLAUDE.md with AUTO-MANAGED sections

### 5. Confirm Setup

Display summary:
- Trigger mode configured
- Database location
- CLAUDE.md created/updated
- Initial entities in memory (if any)

$ARGUMENTS

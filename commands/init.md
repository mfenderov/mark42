---
description: Initialize mark42 for this project
argument-hint: [--gitmode]
---

# /memory:init

Initialize the mark42 system for this project.

## Steps

### 1. Configure Trigger Mode

Ask the user how auto-memory should trigger:

**Options:**
- `default` (Recommended): Track Edit/Write/Bash operations in real-time
- `gitmode`: Only trigger on git commits (best for frequent committers)

Save to `.claude/mark42/config.json`:
```json
{
  "triggerMode": "default"
}
```

### 2. Update .gitignore

Add to `.gitignore`:
```
# mark42
.claude/mark42/dirty-files
```

### 3. Initialize Database

Ensure the database exists:
```bash
mark42 init
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

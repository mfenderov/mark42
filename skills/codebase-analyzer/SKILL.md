---
name: codebase-analyzer
description: |
  Analyze codebase and initialize CLAUDE.md structure.
  Use when running /memory:init to create initial CLAUDE.md files
  with AUTO-MANAGED sections.
---

# Codebase Analyzer

Analyze codebase structure and generate initial CLAUDE.md files.

## Workflow

### Step 1: Scan for Existing CLAUDE.md

Check if CLAUDE.md already exists:

```bash
find . -name "CLAUDE.md" -type f 2>/dev/null
```

If found, ask user:
- **Migrate**: Convert to auto-managed format (add markers)
- **Backup**: Create .backup and generate fresh
- **Merge**: Keep manual sections, add auto-managed
- **Cancel**: Abort

### Step 2: Detect Project Type

Scan for framework indicators:

| File | Project Type |
|------|--------------|
| `package.json` | Node.js/JavaScript |
| `pyproject.toml`, `setup.py` | Python |
| `Cargo.toml` | Rust |
| `go.mod` | Go |
| `Makefile` | Make-based |
| `Dockerfile` | Containerized |

Extract build/test/lint commands from config files.

### Step 3: Identify Subtrees

Look for module directories that should have their own CLAUDE.md:
- `src/` with 10+ source files
- `lib/` directory
- `packages/*` (monorepo)
- `apps/*` (monorepo)

### Step 4: Detect Patterns

Analyze code for:
- Naming conventions (PascalCase, camelCase, snake_case)
- Import patterns (ES6, CommonJS, Go imports)
- Architecture (feature-based, layered, MVC)
- Testing patterns (table-driven, BDD, etc.)

### Step 5: Generate CLAUDE.md

Use this template structure:

```markdown
# Project Name

## Project Overview

<!-- AUTO-MANAGED: project-description -->
[Detected description from README or package.json]
<!-- END AUTO-MANAGED -->

## Quick Reference

<!-- AUTO-MANAGED: build-commands -->
| Command | Purpose |
|---------|---------|
[Detected commands]
<!-- END AUTO-MANAGED -->

## Architecture

<!-- AUTO-MANAGED: architecture -->
[Detected directory structure]
<!-- END AUTO-MANAGED -->

## Conventions

<!-- AUTO-MANAGED: conventions -->
[Detected patterns and conventions]
<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
[User notes - never modified by automation]
<!-- END MANUAL -->
```

### Step 6: Initialize SQLite Memory

Optionally seed the memory database with detected patterns:

```bash
mark42 entity create "Project-Name" "project" \
  --obs "Type: [detected-type]" \
  --obs "Framework: [detected-framework]"
```

## Output

- List of CLAUDE.md files created
- Detected project type and frameworks
- Suggested subtree locations
- Initial memory entities created

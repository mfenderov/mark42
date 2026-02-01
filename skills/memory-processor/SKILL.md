---
name: memory-processor
description: |
  Process changed files and update relevant CLAUDE.md sections.
  Use when the memory-updater agent needs to update AUTO-MANAGED sections
  based on file changes.
---

# Memory Processor

Process file changes and update CLAUDE.md AUTO-MANAGED sections.

## Guidelines

**Content Rules:**
- Be specific: "Use 2-space indentation" not "Format code properly"
- Include commands: Build, test, lint, dev commands
- Document patterns: Code style, naming conventions, architectural decisions
- Keep concise: Target < 500 lines
- Use structure: Bullet points under descriptive headings
- Stay current: Remove outdated information when updating
- Avoid generic: No "follow best practices" or "write clean code"
- Exclude moving targets: Never include version numbers, test counts, timestamps

## Marker Format

```markdown
<!-- AUTO-MANAGED: section-name -->
Content automatically updated
<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
Content never touched
<!-- END MANUAL -->
```

**CRITICAL**: Use exact format. No variations like `BEGIN AUTO-MANAGED` or `END AUTO-MANAGED: name`.

## Section Mapping

**Root CLAUDE.md:**

| Section | Purpose | Update Triggers |
|---------|---------|-----------------|
| `project-description` | Project overview | README changes |
| `build-commands` | Build, test, lint | package.json, Makefile, pyproject.toml |
| `architecture` | Directory structure | New dirs, structural changes |
| `conventions` | Naming, imports | Pattern changes in source |
| `cli-commands` | CLI usage | CLI changes |

**Subtree CLAUDE.md:**

| Section | Purpose | Update Triggers |
|---------|---------|-----------------|
| `module-description` | Module purpose | Module README changes |
| `architecture` | Module structure | File changes |
| `conventions` | Module conventions | Pattern changes |

## Algorithm

1. **Parse context**: Read file changes from memory-updater agent
2. **Categorize changes**: Map files to CLAUDE.md sections
3. **Analyze impact**: What needs updating?
4. **Verify before removal**: Use Grep to check if pattern still exists elsewhere
5. **Update CLAUDE.md**: Modify sections, preserve markers
6. **Validate**: Ensure specific, concise, structured content

## Removal Verification

Before removing documented items:

```bash
# Check if pattern still exists in codebase
grep -r "<pattern>" src/ --include="*.go" --exclude-dir=.git
```

If found elsewhere → keep documented
If not found anywhere → remove from CLAUDE.md

## Output

Return brief summary:
- "Updated [sections] in [path] based on changes to [files]"
- "Removed [item] - no longer used in codebase"
- "No updates needed"

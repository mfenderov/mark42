# ADR 0003: Consumer-Driven Contracts and Neutral State Layout

## Status
Accepted

## Context
`mark42` serves as a cross-harness persistent memory layer accessed by diverse AI harnesses:
- **Claude Code** (via native Go hook adapters)
- **OpenCode** (via Node.js plugin over CLI stdin)
- **Pi** (via standard MCP recall over stdio)

Historically:
1. `session capture` CLI accepted JSON on standard input without an explicit schema specification or validation, leading to silent capture failures when fields were missing or malformed.
2. The project slugging algorithm was duplicated between Go (`internal/state/state.go`) and JavaScript (`adapters/opencode/mark42.js`), creating a risk of divergence.
3. State directories were split between neutral paths (`~/.mark42/state/<slug>`) and legacy project-local paths (`.claude/mark42/`).

## Decision
1. **JSON Schema Specification**: Formulate and publish `schemas/session-capture.v1.json` as the canonical contract for session capture payloads.
2. **Strict CLI Validation**: Validate all session capture payloads on stdin in `mark42 session capture` with informative error messages.
3. **Single Source of Truth for Slugs**: Provide `mark42 path slug <dir>` and `mark42 path state-dir <dir>` subcommands so external adapters can query the canonical slug without reimplementing string-replacement algorithms.
4. **Strangler Fig State Migration**: Implement `state.MigrateLegacyState(projectDir)` to transparently promote legacy scratch files into neutral state storage.

## Consequences
- **Positive**: Contract mismatches between external harness plugins and `mark42` fail with descriptive validation errors rather than corrupting or dropping session state.
- **Positive**: Single canonical source of truth for slug calculation across all languages.
- **Positive**: Seamless upgrade path for existing developer repositories.

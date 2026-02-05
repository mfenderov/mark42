# Claude Memory Plugin Improvement Plan

> Generated: 2026-02-05
> Status: Ready for Implementation
> Last Updated: 2026-02-05 (Corrected hook format documentation)

## Overview

This plan outlines improvements to the claude-memory plugin (mark42) based on:
- Claude Code hook format updates and new hook types
- Underutilized CLI features already implemented
- Phase 3 roadmap alignment (Intelligence features)

---

## Priority 1: Hook Enhancements

### 1.1 Add SessionEnd Hook

**Purpose**: Clean shutdown, final context save, session statistics.

**New File**: `hooks/session-end.py`

**Responsibilities**:
- Flush any pending memory writes
- Record session duration and statistics
- Trigger final knowledge extraction if dirty files exist
- Log session summary to SQLite

**Output Format** (SessionEnd uses standard format):
```python
result = {
    "hookSpecificOutput": {
        "message": "Session ended, memories synced",
        "stats": {"entities_updated": N, "duration_seconds": M}
    }
}
```

**Effort**: ~2 hours

### 1.2 Add PreCompact Hook

**Purpose**: Preserve critical memories before context compaction.

**New File**: `hooks/pre-compact.py`

**Responsibilities**:
- Extract key decisions from current context
- Save important observations before they're lost
- Update entity importance scores based on conversation
- Mark session-turn facts for potential promotion to dynamic

**Output Format** (PreCompact uses hookSpecificOutput):
```python
result = {
    "hookSpecificOutput": {
        "memoriesPreserved": count,
        "contextSummary": "brief summary"
    }
}
```

**Effort**: ~3 hours

### 1.3 Enhance Existing Stop Hook

**File**: `hooks/stop.py`

**Note**: The current `decision`/`reason` format is CORRECT for Stop hooks. No format change needed.

**Enhancement**: Add more structured context gathering:
- Include entity counts modified during session
- Add timing information
- Better dirty file tracking

**Effort**: ~1 hour

---

## Priority 2: New Skills/Commands

### 2.1 `/memory:context` Skill

**Purpose**: Inject relevant memories into current conversation.

**New File**: `skills/memory-context/SKILL.md`

**Implementation**:
```bash
# Via MCP tool (preferred):
mcp__mark42__get_context with projectName and tokenBudget

# Via CLI (for testing):
claude-memory search "<project>" --format context --limit 20
```

**Skill Behavior**:
1. Detect current project name from cwd
2. Call `get_context` MCP tool with project boost
3. Format memories for context injection
4. Present memories grouped by importance/fact-type

**Effort**: ~1 hour

### 2.2 `/memory:search` Skill

**Purpose**: Interactive memory search during session.

**New File**: `skills/memory-search/SKILL.md`

**Implementation**:
```bash
# Via MCP tool (uses hybrid search automatically):
mcp__mark42__search_nodes with query

# Via CLI:
claude-memory search "<query>" --limit 10 --format json
```

**Note**: Hybrid search (FTS5 + vector) is automatic in the MCP tool when an embedder is configured. The CLI currently uses FTS5-only search.

**Skill Behavior**:
1. Accept natural language query
2. Use `search_nodes` MCP tool (hybrid when available)
3. Display results with entity types and observations
4. Allow drilling into specific entities via `open_nodes`

**Effort**: ~1 hour

### 2.3 `/memory:importance` Skill

**Purpose**: Manually boost/demote memory importance.

**New File**: `skills/memory-importance/SKILL.md`

**Implementation**:
- Leverage existing `SetObservationImportance` storage method
- Allow user to mark memories as critical (1.0) or deprecated (0.0)
- Integrate with `get_context` importance filtering

**Effort**: ~1 hour

---

## Priority 3: Phase 3 Intelligence Features

### 3.1 Auto-Context Injection at SessionStart

**Current**: SessionStart hook loads context but doesn't inject it.

**Improvement**:
- Enhance `session-start.py` to call `get_context`
- Inject top N memories into session context
- Use project name for relevance boosting

**Effort**: ~2 hours

### 3.2 Importance Scoring Algorithm

**Current**: Static importance (0-1 float).

**Improvement**:
- Track access frequency per entity
- Boost importance when memories are referenced
- Decay importance for unused memories
- Formula: `importance = base_importance * recency_factor * access_factor`

**Implementation Location**: `internal/storage/importance.go`

**Effort**: ~4 hours

### 3.3 Memory Decay/Consolidation

**Current**: Memories persist indefinitely.

**Improvement**:
- Implement TTL for session_turn facts
- Auto-promote frequently accessed session_turn → dynamic
- Auto-archive stale dynamic facts
- Consolidation: merge similar observations

**Implementation Location**: `internal/storage/decay.go`

**Effort**: ~6 hours

### 3.4 Cross-Session Continuity

**Purpose**: Track what was discussed across sessions.

**Implementation**:
- Store session summaries as entities
- Link entities to sessions where they were modified
- Enable "what did we discuss last time?" queries

**Effort**: ~4 hours

---

## Implementation Sequence

### Sprint 1: Hook Enhancements (1 day)
1. [ ] Add SessionEnd hook
2. [ ] Add PreCompact hook
3. [ ] Enhance Stop hook with better context
4. [ ] Update hooks.json with new hooks
5. [ ] Test hook chain execution

### Sprint 2: Skills/Commands (1 day)
1. [ ] Create `/memory:context` skill
2. [ ] Create `/memory:search` skill
3. [ ] Create `/memory:importance` skill
4. [ ] Add CLI `--hybrid` flag (optional enhancement)

### Sprint 3: Intelligence Features (1 week)
1. [ ] Auto-context injection
2. [ ] Importance scoring algorithm
3. [ ] Memory decay mechanism
4. [ ] Cross-session continuity

---

## Testing Strategy

### Unit Tests
- Hook output format validation
- Importance calculation correctness
- Decay algorithm behavior

### Integration Tests
- Full hook chain: SessionStart → PostToolUse → PreCompact → Stop → SessionEnd
- Memory injection and retrieval cycle
- Cross-session entity tracking

### Manual Testing
- Run Claude Code session with all hooks enabled
- Verify memories persist correctly
- Test context injection relevance

---

## File Changes Summary

| File | Action | Priority |
|------|--------|----------|
| `hooks/session-end.py` | Create | P1 |
| `hooks/pre-compact.py` | Create | P1 |
| `hooks/stop.py` | Enhance (keep format) | P1 |
| `hooks.json` | Add new hooks | P1 |
| `skills/memory-context/SKILL.md` | Create | P2 |
| `skills/memory-search/SKILL.md` | Create | P2 |
| `skills/memory-importance/SKILL.md` | Create | P2 |
| `internal/storage/importance.go` | Create | P3 |
| `internal/storage/decay.go` | Create | P3 |
| `internal/storage/session.go` | Create | P3 |

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Hook execution time | < 500ms each |
| Context injection relevance | > 80% useful memories |
| Memory decay accuracy | No premature deletions |
| Cross-session recall | Find prior discussions > 90% |

---

## Dependencies

- Claude Code 2.1.31+ (for SessionEnd, PreCompact hooks)
- Ollama running locally (for hybrid search embeddings)
- SQLite with FTS5 (already in place)

---

## Hook Format Reference

Different hooks use different output formats:

| Hook Type | Format |
|-----------|--------|
| Stop, SubagentStop, PostToolUse, UserPromptSubmit | `{"decision": "...", "reason": "..."}` |
| PreToolUse | `{"hookSpecificOutput": {"permissionDecision": "..."}}` |
| PermissionRequest | `{"hookSpecificOutput": {"decision": {"behavior": "..."}}}` |
| SessionEnd, PreCompact | `{"hookSpecificOutput": {...}}` |
| SessionStart | No output expected |

---

## Notes

- All changes should maintain backward compatibility with existing memories
- Consider migration path for existing session_turn facts when decay is implemented
- Hook async execution could improve performance but adds complexity
- The Stop hook format (`decision`/`reason`) is NOT deprecated - it's the correct format

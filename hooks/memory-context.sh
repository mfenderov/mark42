#!/bin/bash
# Memory context injection - loads relevant memories at session start
#
# This hook queries the memory database and injects relevant context

MEMORY_DB="${CLAUDE_MEMORY_DB:-$HOME/.claude/memory.db}"
MEMORY_BIN="${CLAUDE_MEMORY_BIN:-$HOME/bin/claude-memory}"

# Exit silently if database doesn't exist
if [[ ! -f "$MEMORY_DB" ]]; then
    exit 0
fi

# Exit silently if binary doesn't exist
if [[ ! -x "$MEMORY_BIN" ]]; then
    exit 0
fi

# Get project name for relevant search
PROJECT_NAME=$(basename "${CLAUDE_PROJECT_DIR:-$(pwd)}")

# Search for relevant memories (project name + recent)
CONTEXT=$("$MEMORY_BIN" --db "$MEMORY_DB" search "$PROJECT_NAME" --limit 5 --format context 2>/dev/null)

if [[ -n "$CONTEXT" ]]; then
    echo "Relevant memories for this session:"
    echo ""
    echo "$CONTEXT"
fi

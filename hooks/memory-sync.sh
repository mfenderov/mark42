#!/bin/bash
# Memory sync - prompts for knowledge persistence at session end
#
# Only blocks on FIRST stop of session (uses a flag file to prevent loops)

FLAG_FILE="${CLAUDE_PROJECT_DIR:-.}/.claude/memory-prompted"

# If already prompted this session, pass through
if [[ -f "$FLAG_FILE" ]]; then
    exit 0
fi

# Create flag to prevent re-prompting
mkdir -p "$(dirname "$FLAG_FILE")" 2>/dev/null
touch "$FLAG_FILE"

cat << 'EOF'
{
  "decision": "block",
  "reason": "Memory sync: Review if any learnings, patterns, decisions, or knowledge emerged this session worth persisting. If so, use Bash to call:\n\n  claude-memory entity create \"<name>\" \"<type>\" --obs \"<observation>\"\n  claude-memory obs add \"<entity>\" \"<observation>\"\n  claude-memory rel create \"<from>\" \"<to>\" \"<relation-type>\"\n\nIf nothing significant, say 'nothing to persist' and proceed."
}
EOF

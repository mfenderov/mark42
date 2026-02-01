# Rollout Guide

## Prerequisites

- [ ] Claude Memory CLI built: `make build`
- [ ] Tests passing: `make test`
- [ ] Docker MCP has data to migrate (optional)

## Step 1: Install Binary

```bash
# Option A: Install to ~/bin (recommended)
make install

# Option B: Symlink to project
ln -sf $(pwd)/claude-memory ~/bin/claude-memory

# Verify
claude-memory version
```

## Step 2: Initialize Database

```bash
claude-memory init
# Output: INFO Database initialized path=~/.claude/memory.db
```

## Step 3: Migrate Existing Data (Optional)

If you have data in the Docker Memory MCP:

```bash
# 1. Export from Docker volume
docker run --rm -v claude-memory:/data alpine cat /data/memory.json > /tmp/memory.json

# 2. Import to SQLite
claude-memory migrate --from /tmp/memory.json

# 3. Verify
claude-memory stats
```

## Step 4: Update Stop Hook

Edit `~/.claude/hooks/memory-mcp-sync.sh`:

```bash
#!/bin/bash
# Memory sync - prompts for knowledge persistence at session end

FLAG_FILE="${CLAUDE_PROJECT_DIR:-.}/.claude/memory-prompted"

if [[ -f "$FLAG_FILE" ]]; then
    exit 0
fi

mkdir -p "$(dirname "$FLAG_FILE")" 2>/dev/null
touch "$FLAG_FILE"

cat << 'EOF'
{
  "decision": "block",
  "reason": "Memory sync: Review if any learnings, patterns, decisions, or knowledge emerged this session worth persisting. If so, use Bash to call:\n\n  claude-memory entity create \"<name>\" \"<type>\" --obs \"<observation>\"\n  claude-memory obs add \"<entity>\" \"<observation>\"\n  claude-memory rel create \"<from>\" \"<to>\" \"<relation-type>\"\n\nIf nothing significant, say 'nothing to persist' and proceed."
}
EOF
```

## Step 5: Add SessionStart Hook (Optional)

Create `~/.claude/hooks/memory-context.sh`:

```bash
#!/bin/bash
# Memory context injection - loads relevant memories at session start

MEMORY_DB="${HOME}/.claude/memory.db"
MEMORY_BIN="${HOME}/bin/claude-memory"

if [[ ! -f "$MEMORY_DB" ]] || [[ ! -x "$MEMORY_BIN" ]]; then
    exit 0
fi

PROJECT_NAME=$(basename "${CLAUDE_PROJECT_DIR:-$(pwd)}")
CONTEXT=$("$MEMORY_BIN" --db "$MEMORY_DB" search "$PROJECT_NAME" --limit 5 --format context 2>/dev/null)

if [[ -n "$CONTEXT" ]]; then
    echo "Relevant memories for this session:"
    echo ""
    echo "$CONTEXT"
fi
```

Make executable:
```bash
chmod +x ~/.claude/hooks/memory-context.sh
```

Add to `~/.claude/settings.json`:
```json
"SessionStart": [
  {
    "matcher": "startup",
    "hooks": [
      {
        "type": "command",
        "command": "~/.claude/hooks/memory-context.sh",
        "timeout": 5
      }
    ]
  }
]
```

## Step 6: Disable Docker MCP (Optional)

Once satisfied with the new system, comment out or remove the Docker MCP from `~/.claude/.mcp.json`:

```json
{
  "mcpServers": {
    // "MCP_DOCKER": { ... }  // Commented out
  }
}
```

## Step 7: Update CLAUDE.md

Update the Memory MCP reference in `~/.claude/CLAUDE.md`:

```markdown
## Non-Negotiable Rules

6. **YOU MUST** check memory (`claude-memory search "query"`) for relevant patterns, decisions, or context before starting complex tasks
```

## Verification

### Test Persistence (Stop Hook)
1. Start a new session
2. Do some work
3. End the session
4. When prompted, try: `claude-memory entity create "test" "verification"`
5. Verify: `claude-memory entity get test`

### Test Context Injection (SessionStart Hook)
1. Create some test data: `claude-memory entity create "my-project" "project" --obs "Test data"`
2. Start a new session in a directory named `my-project`
3. Verify the memory context appears at session start

### Test Search
```bash
claude-memory search "pattern"
claude-memory search "TDD"
```

## Rollback

If issues occur:

1. Re-enable Docker MCP in `~/.claude/.mcp.json`
2. Revert hook changes
3. Data remains in `~/.claude/memory.db` for debugging

## CLI Reference

```bash
# Entity operations
claude-memory entity create <name> <type> [--obs "observation"]...
claude-memory entity get <name>
claude-memory entity list [--type <type>]
claude-memory entity delete <name>

# Observation operations
claude-memory obs add <entity> <content>
claude-memory obs delete <entity> <content>

# Relation operations
claude-memory rel create <from> <to> <type>
claude-memory rel list <entity>
claude-memory rel delete <from> <to> <type>

# Search
claude-memory search <query> [--limit N] [--format default|json|context]

# Graph
claude-memory graph [--format json|dot]

# Database
claude-memory init
claude-memory stats
claude-memory migrate --from <json-file>
```

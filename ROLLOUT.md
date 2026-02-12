# Rollout Guide

## Prerequisites

- [ ] mark42 CLI built: `make build`
- [ ] Tests passing: `make test`
- [ ] Docker MCP has data to migrate (optional)

## Step 1: Install Binary

```bash
# Option A: Install to ~/bin (recommended)
make install

# Option B: Symlink to project
ln -sf $(pwd)/mark42 ~/bin/mark42

# Verify
mark42 version
```

## Step 2: Initialize Database

```bash
mark42 init
# Output: INFO Database initialized path=~/.claude/memory.db
```

## Step 3: Migrate Existing Data (Optional)

If you have data in the Docker Memory MCP:

```bash
# 1. Export from Docker volume
docker run --rm -v mark42:/data alpine cat /data/memory.json > /tmp/memory.json

# 2. Import to SQLite
mark42 migrate --from /tmp/memory.json

# 3. Verify
mark42 stats
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
  "reason": "Memory sync: Review if any learnings, patterns, decisions, or knowledge emerged this session worth persisting. If so, use Bash to call:\n\n  mark42 entity create \"<name>\" \"<type>\" --obs \"<observation>\"\n  mark42 obs add \"<entity>\" \"<observation>\"\n  mark42 rel create \"<from>\" \"<to>\" \"<relation-type>\"\n\nIf nothing significant, say 'nothing to persist' and proceed."
}
EOF
```

## Step 5: Add SessionStart Hook (Optional)

Create `~/.claude/hooks/memory-context.sh`:

```bash
#!/bin/bash
# Memory context injection - loads relevant memories at session start

MEMORY_DB="${HOME}/.claude/memory.db"
MEMORY_BIN="${HOME}/bin/mark42"

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

6. **YOU MUST** check memory (`mark42 search "query"`) for relevant patterns, decisions, or context before starting complex tasks
```

## Verification

### Test Persistence (Stop Hook)
1. Start a new session
2. Do some work
3. End the session
4. When prompted, try: `mark42 entity create "test" "verification"`
5. Verify: `mark42 entity get test`

### Test Context Injection (SessionStart Hook)
1. Create some test data: `mark42 entity create "my-project" "project" --obs "Test data"`
2. Start a new session in a directory named `my-project`
3. Verify the memory context appears at session start

### Test Search
```bash
mark42 search "pattern"
mark42 search "TDD"
```

## Rollback

If issues occur:

1. Re-enable Docker MCP in `~/.claude/.mcp.json`
2. Revert hook changes
3. Data remains in `~/.claude/memory.db` for debugging

## CLI Reference

```bash
# Entity operations
mark42 entity create <name> <type> [--obs "observation"]...
mark42 entity get <name>
mark42 entity list [--type <type>]
mark42 entity delete <name>

# Observation operations
mark42 obs add <entity> <content>
mark42 obs delete <entity> <content>

# Relation operations
mark42 rel create <from> <to> <type>
mark42 rel list <entity>
mark42 rel delete <from> <to> <type>

# Search
mark42 search <query> [--limit N] [--format default|json|context]

# Graph
mark42 graph [--format json|dot]

# Database
mark42 init
mark42 stats
mark42 migrate --from <json-file>
```

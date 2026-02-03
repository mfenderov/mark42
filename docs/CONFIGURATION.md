# Configuration Guide

## Database Location

Default: `~/.claude/memory.db`

Override with `--db` flag or environment variable:
```bash
export CLAUDE_MEMORY_DB=/path/to/custom/memory.db
claude-memory --db /path/to/custom/memory.db
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CLAUDE_MEMORY_DB` | `~/.claude/memory.db` | Database file path |
| `CLAUDE_MEMORY_TOKEN_BUDGET` | `2000` | Max tokens for context injection |
| `CLAUDE_MEMORY_MIN_IMPORTANCE` | `0.3` | Minimum importance score for context |
| `CLAUDE_MEMORY_BOOST` | `1.5` | Score boost for project-matching memories |
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama API URL |

## Ollama Configuration

### Base URL

```bash
# Default
claude-memory embed test --url http://localhost:11434

# Custom URL
claude-memory embed test --url http://my-server:11434
```

### Embedding Model

```bash
# Default: nomic-embed-text
claude-memory embed generate --model nomic-embed-text

# Alternative models
claude-memory embed generate --model mxbai-embed-large
claude-memory embed generate --model all-minilm
```

## Context Injection

### Token Budget

Controls how many tokens of memory context are injected at session start.

```bash
# Default: 2000 tokens
claude-memory context --token-budget 2000

# Smaller for fast sessions
claude-memory context --token-budget 1000

# Larger for context-heavy work
claude-memory context --token-budget 3000
```

### Importance Threshold

Minimum importance score to include in context.

```bash
# Default: 0.3
claude-memory context --min-importance 0.3

# Only high-importance memories
claude-memory context --min-importance 0.7

# Include more memories
claude-memory context --min-importance 0.1
```

### Project Boosting

Memories matching the current project get score boost.

```bash
# Default: 1.5x boost
claude-memory workdir search "query" --tag "my-project" --boost 1.5

# Higher boost for project focus
claude-memory workdir search "query" --tag "my-project" --boost 2.0
```

## Memory Decay Configuration

### Archive Settings

```bash
# Archive memories older than 90 days with importance < 0.1
claude-memory decay archive --days 90 --min-importance 0.1

# Preview what would be archived
claude-memory decay archive --days 90 --min-importance 0.1 --dry-run
```

### Forget Settings

```bash
# Delete memories past their forget_after date
claude-memory decay forget --expired

# Delete archived memories older than 180 days
claude-memory decay forget --archive-days 180
```

## Fact Types

| Type | Description | Use Case |
|------|-------------|----------|
| `static` | Permanent facts, preferences | User preferences, coding standards |
| `dynamic` | Contextual, session-relevant | Recent decisions, temporary context |
| `session_turn` | Single-conversation scope | Conversation-specific context |

Static facts receive a 1.2x boost in context scoring.

## Importance Scoring

The importance formula:

```
importance = base_score × recency_decay × frequency_score × centrality_score
```

Where:
- `base_score`: Initial observation importance (default: 1.0)
- `recency_decay`: e^(-days_since_access / 30)
- `frequency_score`: 1 + log(access_count + 1)
- `centrality_score`: 1 + (relation_count / max_relations) × 0.5

### Recalculation

```bash
# Recalculate all importance scores
claude-memory importance recalculate

# View importance distribution
claude-memory importance stats
```

## Plugin Configuration

### Hook Environment

Hooks receive these environment variables:
- `CLAUDE_PROJECT_DIR`: Current working directory
- `CLAUDE_PLUGIN_ROOT`: Plugin installation directory

### Customizing Session Start

Edit `.claude-plugin/hooks/session-start.py`:

```python
# Adjust token budget
token_budget = int(os.environ.get("CLAUDE_MEMORY_TOKEN_BUDGET", "2000"))

# Adjust boost factor
boost_factor = os.environ.get("CLAUDE_MEMORY_BOOST", "1.5")
```

## MCP Server Configuration

For `.mcp.json`:

```json
{
  "mcpServers": {
    "claude-memory": {
      "command": "claude-memory-server",
      "args": ["--db", "~/.claude/memory.db"]
    }
  }
}
```

## Performance Tuning

### For Large Databases

```bash
# Increase WAL checkpoint frequency
sqlite3 ~/.claude/memory.db "PRAGMA wal_checkpoint(TRUNCATE);"

# Analyze for query optimization
sqlite3 ~/.claude/memory.db "ANALYZE;"

# Vacuum to reclaim space
sqlite3 ~/.claude/memory.db "VACUUM;"
```

### For Slow Searches

1. Reduce search limits: `--limit 10`
2. Increase importance threshold: `--min-importance 0.5`
3. Ensure embeddings are generated: `claude-memory embed stats`

## Backup and Restore

### Backup

```bash
# Simple copy (while not in use)
cp ~/.claude/memory.db ~/.claude/memory.db.backup

# Safe backup (handles active connections)
sqlite3 ~/.claude/memory.db ".backup ~/.claude/memory.db.backup"
```

### Restore

```bash
mv ~/.claude/memory.db.backup ~/.claude/memory.db
claude-memory upgrade  # Ensure schema is current
```

## Security Considerations

1. **File Permissions**: Database should be readable only by owner
   ```bash
   chmod 600 ~/.claude/memory.db
   ```

2. **Sensitive Data**: Avoid storing secrets in observations
   - Use environment variables instead
   - Mark sensitive entities for early decay

3. **Backup Encryption**: Encrypt backups if they contain sensitive context
   ```bash
   sqlite3 ~/.claude/memory.db ".backup /dev/stdout" | gpg -c > memory.db.gpg
   ```

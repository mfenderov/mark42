# Troubleshooting Guide

## Common Issues

### Database Errors

#### "database is locked"

**Symptoms**: Operations fail with "database is locked" error.

**Cause**: Multiple processes accessing the same database file.

**Solution**:
1. Check for running claude-memory processes: `pgrep -f claude-memory`
2. Wait for current operations to complete
3. If stuck, restart Claude Code session
4. Enable WAL mode (default): `PRAGMA journal_mode=WAL`

#### "no such table: entities"

**Symptoms**: First-time operations fail with table not found.

**Solution**:
```bash
# Run schema migrations
claude-memory upgrade
```

#### "database disk image is malformed"

**Symptoms**: All operations fail, database appears corrupted.

**Solution**:
1. Back up the corrupted file: `cp ~/.claude/memory.db ~/.claude/memory.db.bak`
2. Attempt repair:
   ```bash
   sqlite3 ~/.claude/memory.db ".recover" | sqlite3 ~/.claude/memory_new.db
   mv ~/.claude/memory_new.db ~/.claude/memory.db
   ```
3. If repair fails, delete and reinitialize:
   ```bash
   rm ~/.claude/memory.db
   claude-memory init
   ```

### Search Issues

#### "no results found" for existing content

**Cause**: FTS5 index may be out of sync.

**Solution**:
```sql
-- Rebuild FTS indexes
INSERT INTO observations_fts(observations_fts) VALUES('rebuild');
INSERT INTO entities_fts(entities_fts) VALUES('rebuild');
```

Or via sqlite3:
```bash
sqlite3 ~/.claude/memory.db "INSERT INTO observations_fts(observations_fts) VALUES('rebuild');"
```

#### Hybrid search not finding semantic matches

**Cause**: Embeddings not generated or Ollama unavailable.

**Solution**:
1. Check Ollama is running: `curl http://localhost:11434/api/tags`
2. Generate embeddings: `claude-memory embed generate`
3. Check embedding coverage: `claude-memory embed stats`

### Embedding Issues

#### "Ollama connection refused"

**Solution**:
1. Start Ollama: `ollama serve`
2. Verify it's running: `ollama list`
3. Pull embedding model: `ollama pull nomic-embed-text`

#### Embedding generation is slow

**Cause**: Large number of observations, slow model, or network issues.

**Solution**:
1. Use smaller batch sizes: `claude-memory embed generate --batch 5`
2. Use local Ollama instead of remote
3. Consider pruning old/unimportant observations first

### Performance Issues

#### Slow searches on large databases

**Solution**:
1. Run importance recalculation to prune low-value memories:
   ```bash
   claude-memory importance recalculate
   ```
2. Archive old memories:
   ```bash
   claude-memory decay archive --days 90 --min-importance 0.1
   ```
3. Check database size:
   ```bash
   ls -lh ~/.claude/memory.db
   ```

#### High memory usage

**Cause**: Large result sets or inefficient queries.

**Solution**:
1. Use smaller limits: `--limit 10`
2. Increase minimum importance threshold: `--min-importance 0.5`

### Plugin/Hook Issues

#### SessionStart hook not loading context

**Symptoms**: No memory context injected at session start.

**Checklist**:
1. Database exists: `ls ~/.claude/memory.db`
2. Binary is in PATH: `which claude-memory`
3. Hook is executable: `ls -l ~/.claude-plugin/hooks/session-start.py`
4. Check hook output manually:
   ```bash
   CLAUDE_PROJECT_DIR=$(pwd) python3 ~/.claude-plugin/hooks/session-start.py
   ```

#### "command not found: claude-memory"

**Solution**:
1. Install binary: `make install`
2. Add to PATH:
   ```bash
   export PATH="$HOME/bin:$PATH"
   ```
3. Or specify full path in hooks

### Migration Issues

#### Import from JSON Memory MCP fails

**Symptoms**: Entities/relations not imported.

**Solution**:
1. Verify JSON format:
   ```bash
   cat ~/.config/claude-memory/memory.json | jq .
   ```
2. Try with verbose output:
   ```bash
   claude-memory migrate --from ~/.config/claude-memory/memory.json 2>&1
   ```
3. Check for duplicate entities (already exist in database)

### Context Injection Issues

#### Too much/too little context injected

**Solution**: Adjust token budget and importance thresholds:
```bash
# Smaller context
claude-memory context --token-budget 1000 --min-importance 0.5

# Larger context
claude-memory context --token-budget 3000 --min-importance 0.2
```

#### Wrong project memories shown

**Cause**: Container tags not set for entities.

**Solution**:
1. Set container tags for project-specific entities:
   ```bash
   claude-memory workdir set "MyEntity" "my-project"
   ```
2. List entities by project:
   ```bash
   claude-memory workdir list "my-project"
   ```

## Diagnostic Commands

```bash
# Check database status
claude-memory stats

# Check schema version
sqlite3 ~/.claude/memory.db "SELECT * FROM goose_db_version;"

# Check embedding coverage
claude-memory embed stats

# Check decay statistics
claude-memory decay stats

# Check importance distribution
claude-memory importance stats

# List all tables
sqlite3 ~/.claude/memory.db ".tables"
```

## Getting Help

1. Check CLI help: `claude-memory --help`
2. Check subcommand help: `claude-memory <command> --help`
3. Review logs in Claude Code output
4. Open an issue on GitHub with:
   - Error message
   - Steps to reproduce
   - Output of `claude-memory stats`
   - Database schema version

# Troubleshooting Guide

## Common Issues

### Database Errors

#### "database is locked"

**Symptoms**: Operations fail with "database is locked" error.

**Cause**: Multiple processes accessing the same database file.

**Solution**:
1. Check for running mark42 processes: `pgrep -f mark42`
2. Wait for current operations to complete
3. If stuck, restart Claude Code session
4. Enable WAL mode (default): `PRAGMA journal_mode=WAL`

#### "no such table: entities"

**Symptoms**: First-time operations fail with table not found.

**Solution**:
```bash
# Run schema migrations
mark42 upgrade
```

#### "database disk image is malformed"

**Symptoms**: All operations fail, database appears corrupted.

**Solution**:
1. Back up the corrupted file: `cp ~/.mark42/memory.db ~/.mark42/memory.db.bak`
2. Attempt repair:
   ```bash
   sqlite3 ~/.mark42/memory.db ".recover" | sqlite3 ~/.mark42/memory_new.db
   mv ~/.mark42/memory_new.db ~/.mark42/memory.db
   ```
3. If repair fails, delete and reinitialize:
   ```bash
   rm ~/.mark42/memory.db
   mark42 init
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
sqlite3 ~/.mark42/memory.db "INSERT INTO observations_fts(observations_fts) VALUES('rebuild');"
```

#### Hybrid search not finding semantic matches

**Cause**: Embeddings not generated or Ollama unavailable.

**Solution**:
1. Check Ollama is running: `curl http://localhost:11434/api/tags`
2. Generate embeddings: `mark42 embed generate`
3. Check embedding coverage: `mark42 embed stats`

### Embedding Issues

#### "Ollama connection refused"

**Solution**:
1. Start Ollama: `ollama serve`
2. Verify it's running: `ollama list`
3. Pull embedding model: `ollama pull nomic-embed-text`

#### Embedding generation is slow

**Cause**: Large number of observations, slow model, or network issues.

**Solution**:
1. Use smaller batch sizes: `mark42 embed generate --batch 5`
2. Use local Ollama instead of remote
3. Consider pruning old/unimportant observations first

### Performance Issues

#### Slow searches on large databases

**Solution**:
1. Run importance recalculation to prune low-value memories:
   ```bash
   mark42 importance recalculate
   ```
2. Archive old memories:
   ```bash
   mark42 decay archive --days 90 --min-importance 0.1
   ```
3. Check database size:
   ```bash
   ls -lh ~/.mark42/memory.db
   ```

#### High memory usage

**Cause**: Large result sets or inefficient queries.

**Solution**:
1. Use smaller limits: `--limit 10`
2. Increase minimum importance threshold: `--min-importance 0.5`

### MCP Server Issues

#### Server fails to start or connect

**Symptoms**: Harness reports MCP connection failed or tool calls time out.

**Checklist**:
1. Binary is installed and executable: `which mark42-server`
2. Check standalone execution: `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | mark42-server`
3. Verify database permissions: `ls -la ~/.mark42/memory.db`
4. Inspect configuration in `.mcp.json` or your IDE settings.

#### "command not found: mark42-server"

**Solution**:
1. Install binaries: `make install-all`
2. Add to PATH:
   ```bash
   export PATH="$HOME/bin:$PATH"
   ```
3. Or specify absolute path in `.mcp.json`:
   ```json
   {
     "mcpServers": {
       "mark42": {
         "command": "/absolute/path/to/bin/mark42-server"
       }
     }
   }
   ```

### Migration Issues

#### Import from JSON Memory MCP fails

**Symptoms**: Entities/relations not imported.

**Solution**:
1. Verify JSON format:
   ```bash
   cat ~/.config/mark42/memory.json | jq .
   ```
2. Try with verbose output:
   ```bash
   mark42 migrate --from ~/.config/mark42/memory.json 2>&1
   ```
3. Check for duplicate entities (already exist in database)

### Context Injection Issues

#### Too much/too little context injected

**Solution**: Adjust token budget and importance thresholds:
```bash
# Smaller context
mark42 context --token-budget 1000 --min-importance 0.5

# Larger context
mark42 context --token-budget 3000 --min-importance 0.2
```

#### Wrong project memories shown

**Cause**: Container tags not set for entities.

**Solution**:
1. Set container tags for project-specific entities:
   ```bash
   mark42 workdir set "MyEntity" "my-project"
   ```
2. List entities by project:
   ```bash
   mark42 workdir list "my-project"
   ```

## Diagnostic Commands

```bash
# Check database status
mark42 stats

# Check schema version
sqlite3 ~/.mark42/memory.db "SELECT * FROM goose_db_version;"

# Check embedding coverage
mark42 embed stats

# Check decay statistics
mark42 decay stats

# Check importance distribution
mark42 importance stats

# List all tables
sqlite3 ~/.mark42/memory.db ".tables"
```

## Getting Help

1. Check CLI help: `mark42 --help`
2. Check subcommand help: `mark42 <command> --help`
3. Review logs in Claude Code output
4. Open an issue on GitHub with:
   - Error message
   - Steps to reproduce
   - Output of `mark42 stats`
   - Database schema version

# Migration Guide

## From JSON Memory MCP

The `@modelcontextprotocol/server-memory` stores data in JSON format. This guide explains how to migrate to claude-memory's SQLite format.

### Automatic Migration

```bash
# From default JSON Memory MCP location
claude-memory migrate --from ~/.config/claude-memory/memory.json

# From custom location
claude-memory migrate --from /path/to/memory.json
```

### Supported Formats

#### Standard JSON Format

```json
{
  "entities": [
    {
      "name": "EntityName",
      "entityType": "type",
      "observations": ["obs1", "obs2"]
    }
  ],
  "relations": [
    {
      "from": "Entity1",
      "to": "Entity2",
      "relationType": "relates_to"
    }
  ]
}
```

#### NDJSON Format (Docker MCP)

```json
{"type":"entity","name":"EntityName","entityType":"type","observations":["obs1"]}
{"type":"relation","from":"Entity1","to":"Entity2","relationType":"relates_to"}
```

### Migration Steps

1. **Backup existing data**
   ```bash
   cp ~/.config/claude-memory/memory.json ~/.config/claude-memory/memory.json.bak
   ```

2. **Initialize new database**
   ```bash
   claude-memory init
   ```

3. **Run migration**
   ```bash
   claude-memory migrate --from ~/.config/claude-memory/memory.json
   ```

4. **Verify migration**
   ```bash
   claude-memory stats
   ```

5. **Generate embeddings** (optional but recommended)
   ```bash
   claude-memory embed generate
   ```

6. **Update MCP configuration**

   Replace in `.mcp.json`:
   ```json
   {
     "mcpServers": {
       "memory": {
         "command": "claude-memory-server",
         "args": ["--db", "~/.claude/memory.db"]
       }
     }
   }
   ```

### Handling Duplicates

If entities already exist in the target database:
- New observations are added to existing entities
- Duplicate relations are skipped
- Entity types are preserved from existing entries

### Post-Migration Tasks

1. **Recalculate importance scores**
   ```bash
   claude-memory importance recalculate
   ```

2. **Set container tags for project-specific entities**
   ```bash
   claude-memory workdir set "EntityName" "project-name"
   ```

3. **Configure fact types for important memories**
   ```sql
   -- Mark persistent facts as static
   UPDATE observations SET fact_type = 'static'
   WHERE content LIKE '%preference%' OR content LIKE '%convention%';
   ```

## Schema Upgrades

When upgrading to a new version with schema changes:

### Automatic Upgrade

```bash
claude-memory upgrade
```

### Manual Verification

```bash
# Check current schema version
sqlite3 ~/.claude/memory.db "SELECT * FROM goose_db_version ORDER BY id DESC LIMIT 1;"

# Expected: version_id = 7 (current)
```

### Schema Version History

| Version | Changes |
|---------|---------|
| 1 | Initial schema (entities, observations, relations) |
| 2 | FTS5 indexes and triggers |
| 3 | Embedding storage table |
| 4 | Fact type column |
| 5 | Versioning columns |
| 6 | Importance and decay columns |
| 7 | Archive table |

### Rolling Back

If you need to revert schema changes:

```bash
# Backup first
sqlite3 ~/.claude/memory.db ".backup memory_backup.db"

# Downgrade using goose (advanced)
goose -dir internal/storage/migrations sqlite3 ~/.claude/memory.db down
```

**Warning**: Downgrading may lose data in new columns/tables.

## From Other Memory Systems

### From Plain Text Notes

1. Create a JSON file with the format above
2. Run migration command
3. Clean up and categorize entities

### From Notion/Obsidian

1. Export as markdown/JSON
2. Transform to Memory MCP JSON format
3. Run migration
4. Establish relations between imported entities

### Custom Migration Script

For complex migrations, use the storage API directly:

```go
package main

import (
    "github.com/mfenderov/claude-memory/internal/storage"
)

func main() {
    store, _ := storage.NewStore("~/.claude/memory.db")
    defer store.Close()
    store.Migrate()

    // Custom import logic
    store.CreateEntity("Name", "type", []string{"obs"})
    store.CreateRelation("From", "To", "type")
}
```

## Verification Checklist

After migration, verify:

- [ ] Entity count matches source: `claude-memory stats`
- [ ] Observations preserved: `claude-memory entity get <name>`
- [ ] Relations intact: `claude-memory rel list <name>`
- [ ] Search works: `claude-memory search <term>`
- [ ] MCP tools respond: Test via Claude Code session

## Troubleshooting Migration

### "entity already exists"

This is normal - observations are added to existing entities.

### "relation not found"

Source or target entity doesn't exist. Check for typos in entity names.

### Empty migration result

Check JSON format matches expected schema. Validate with:
```bash
cat memory.json | jq '.entities | length'
```

### Partial migration

Some records may fail due to:
- Invalid UTF-8 characters
- Extremely long content
- Circular references in relations

Review warnings in migration output.

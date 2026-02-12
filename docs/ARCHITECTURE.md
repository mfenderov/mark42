# Architecture

## Overview

mark42 is an MCP (Model Context Protocol) server that provides persistent, searchable memory for Claude Code sessions. It replaces the default JSON-based Memory MCP with a SQLite-backed implementation offering superior search capabilities.

## System Components

### 1. MCP Server

The server implements the MCP protocol over stdio, handling JSON-RPC requests from Claude Code.

```
Claude Code ←──stdio──→ MCP Server ←──SQL──→ SQLite
```

**Responsibilities:**
- Parse MCP JSON-RPC requests
- Translate to SQL operations
- Return formatted responses
- Manage database connections

**Implementation choices:**
- Language: Go (for simplicity, single binary, good SQLite support)
- MCP transport: stdio (standard for local MCP servers)
- Concurrency: Single-writer with read-write mutex

### 2. SQLite Database

Single-file database storing the knowledge graph.

**Schema:**

```sql
-- Core entities (nodes in the graph)
CREATE TABLE entities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    entity_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_entities_name ON entities(name);
CREATE INDEX idx_entities_type ON entities(entity_type);

-- Observations (properties attached to entities)
CREATE TABLE observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(entity_id, content)  -- Prevent duplicate observations
);

CREATE INDEX idx_observations_entity ON observations(entity_id);

-- Relations (edges between entities)
CREATE TABLE relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    to_entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(from_entity_id, to_entity_id, relation_type)
);

CREATE INDEX idx_relations_from ON relations(from_entity_id);
CREATE INDEX idx_relations_to ON relations(to_entity_id);

-- Full-text search index on observations
CREATE VIRTUAL TABLE observations_fts USING fts5(
    content,
    content='observations',
    content_rowid='id',
    tokenize='porter unicode61'  -- Stemming + unicode support
);

-- Triggers to keep FTS index in sync
CREATE TRIGGER observations_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER observations_ad AFTER DELETE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, content)
    VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER observations_au AFTER UPDATE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, content)
    VALUES('delete', old.id, old.content);
    INSERT INTO observations_fts(rowid, content) VALUES (new.id, new.content);
END;

-- Entity names also searchable
CREATE VIRTUAL TABLE entities_fts USING fts5(
    name,
    entity_type,
    content='entities',
    content_rowid='id',
    tokenize='porter unicode61'
);

-- Phase 2: Vector embeddings table (sqlite-vec)
-- CREATE VIRTUAL TABLE observation_embeddings USING vec0(
--     observation_id INTEGER PRIMARY KEY,
--     embedding FLOAT[384]  -- nomic-embed-text dimension
-- );
```

### 3. Search Engine

Hybrid search combining multiple strategies:

```
┌─────────────────────────────────────────────────────────────┐
│                      Search Query                            │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
       ┌───────────┐   ┌───────────┐   ┌───────────┐
       │  Exact    │   │   FTS5    │   │  Vector   │
       │  Match    │   │  (BM25)   │   │  (cosine) │
       └───────────┘   └───────────┘   └───────────┘
              │               │               │
              └───────────────┼───────────────┘
                              ▼
                    ┌───────────────────┐
                    │   Score Fusion    │
                    │   (weighted avg)  │
                    └───────────────────┘
                              │
                              ▼
                    ┌───────────────────┐
                    │  Ranked Results   │
                    └───────────────────┘
```

**Phase 1 (FTS5 only):**
```sql
-- Search observations
SELECT e.*, o.content, bm25(observations_fts) as score
FROM observations_fts f
JOIN observations o ON o.id = f.rowid
JOIN entities e ON e.id = o.entity_id
WHERE observations_fts MATCH ?
ORDER BY score
LIMIT 20;

-- Search entity names
SELECT e.*, bm25(entities_fts) as score
FROM entities_fts f
JOIN entities e ON e.id = f.rowid
WHERE entities_fts MATCH ?
ORDER BY score
LIMIT 20;
```

**Phase 2 (Hybrid):**
```sql
WITH keyword_results AS (
    SELECT o.entity_id, MIN(bm25(observations_fts)) as keyword_score
    FROM observations_fts f
    JOIN observations o ON o.id = f.rowid
    WHERE observations_fts MATCH ?
    GROUP BY o.entity_id
),
vector_results AS (
    SELECT observation_id, distance as vector_score
    FROM observation_embeddings
    WHERE embedding MATCH ? AND k = 50
),
combined AS (
    SELECT
        COALESCE(k.entity_id, o.entity_id) as entity_id,
        COALESCE(k.keyword_score, 0) as keyword_score,
        COALESCE(v.vector_score, 1) as vector_score
    FROM keyword_results k
    FULL OUTER JOIN (
        SELECT o.entity_id, MIN(v.vector_score) as vector_score
        FROM vector_results v
        JOIN observations o ON o.id = v.observation_id
        GROUP BY o.entity_id
    ) v ON k.entity_id = v.entity_id
)
SELECT e.*, c.keyword_score, c.vector_score,
       (c.keyword_score * 0.3 + (1 - c.vector_score) * 0.7) as combined_score
FROM combined c
JOIN entities e ON e.id = c.entity_id
ORDER BY combined_score DESC
LIMIT 20;
```

### 4. Embedding Pipeline (Phase 2)

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Observation    │────▶│  Ollama API     │────▶│  sqlite-vec     │
│  (text)         │     │  (embedding)    │     │  (storage)      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

**Embedding model options:**

| Model | Dimensions | Speed | Quality | Memory |
|-------|------------|-------|---------|--------|
| `nomic-embed-text` | 768 | Fast | Good | 274MB |
| `mxbai-embed-large` | 1024 | Medium | Better | 670MB |
| `all-minilm` | 384 | Fastest | Adequate | 46MB |

**Recommendation**: Start with `nomic-embed-text` for balance of quality and speed.

## Data Flow

### Create Entity

```
1. Claude Code: mcp__memory__create_entities({entities: [...]})
2. MCP Server: Parse request
3. SQLite: INSERT INTO entities (name, entity_type) VALUES (?, ?)
4. SQLite: INSERT INTO observations (entity_id, content) VALUES (?, ?)
5. [Phase 2] Ollama: Generate embedding for each observation
6. [Phase 2] SQLite: INSERT INTO observation_embeddings VALUES (?, ?)
7. MCP Server: Return success response
```

### Search Nodes

```
1. Claude Code: mcp__memory__search_nodes({query: "..."})
2. MCP Server: Parse request
3. SQLite: FTS5 MATCH query on observations_fts
4. SQLite: FTS5 MATCH query on entities_fts
5. [Phase 2] Ollama: Generate query embedding
6. [Phase 2] SQLite: Vector similarity search
7. MCP Server: Merge and rank results
8. MCP Server: Return matched entities with observations
```

## File Structure

```
mark42/
├── README.md
├── docs/
│   ├── ARCHITECTURE.md        # This file
│   └── DESIGN_DECISIONS.md    # Rationale for choices
├── cmd/
│   └── server/
│       └── main.go            # Entry point
├── internal/
│   ├── mcp/
│   │   ├── server.go          # MCP protocol handling
│   │   ├── handlers.go        # Tool implementations
│   │   └── types.go           # Request/response types
│   ├── storage/
│   │   ├── sqlite.go          # Database operations
│   │   ├── schema.go          # Schema definitions
│   │   └── migrations.go      # Schema migrations
│   ├── search/
│   │   ├── fts.go             # Full-text search
│   │   ├── vector.go          # Vector search (Phase 2)
│   │   └── hybrid.go          # Score fusion
│   └── embedding/
│       └── ollama.go          # Ollama client (Phase 2)
├── go.mod
├── go.sum
└── Makefile
```

## Configuration

```yaml
# ~/.mark42/config.yaml (optional)
database:
  path: ~/.claude/memory.db

search:
  fts_weight: 0.3        # Weight for keyword matches
  vector_weight: 0.7     # Weight for semantic matches
  max_results: 20

embedding:
  enabled: false         # Set true for Phase 2
  model: nomic-embed-text
  ollama_url: http://localhost:11434
```

## Performance Considerations

### Database

- **WAL mode**: Enable Write-Ahead Logging for concurrent reads
- **Connection pooling**: Single writer, multiple readers
- **Prepared statements**: Cache frequently used queries
- **Index coverage**: Ensure queries use indexes

```sql
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=10000;
PRAGMA temp_store=MEMORY;
```

### Search

- **FTS5 tokenizer**: Use `porter` for English stemming
- **Query optimization**: Limit results early in the pipeline
- **Batch embedding**: Queue observations for batch processing

### Memory

- **Lazy loading**: Don't load entire graph into memory
- **Result streaming**: Stream large result sets
- **Connection reuse**: Keep database connection open

## Security

- **Local only**: No network exposure by default
- **File permissions**: Database file readable only by owner
- **No credentials**: No API keys or tokens stored
- **Sanitized queries**: All queries use parameterized statements

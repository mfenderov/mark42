# Architecture

## Overview

mark42 is a local memory layer for AI coding harnesses (Claude Code, pi, opencode). An MCP (Model Context Protocol) server and CLI provide persistent, searchable memory backed by SQLite, with hybrid FTS5 + vector search. It originated as a drop-in replacement for the JSON-based Memory MCP and remains API-compatible with it.

## System Components

### 1. MCP Server

The server implements the MCP protocol over stdio, handling JSON-RPC requests from the connected harness.

```
Harness (Claude Code / pi / opencode) ←──stdio──→ MCP Server ←──SQL──→ SQLite
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
-- Core entities (nodes in the graph, with versioning)
CREATE TABLE entities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    supersedes_id INTEGER REFERENCES entities(id),
    is_latest BOOLEAN DEFAULT 1,
    version INTEGER DEFAULT 1,
    container_tag TEXT
);

CREATE INDEX idx_entities_name ON entities(name);
CREATE INDEX idx_entities_type ON entities(entity_type);
CREATE INDEX idx_entities_latest ON entities(name, is_latest);
CREATE INDEX idx_entities_container ON entities(container_tag);

-- Observations (properties attached to entities, with fact types and temporal validity)
CREATE TABLE observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    fact_type TEXT DEFAULT 'dynamic',
    importance REAL DEFAULT 1.0,
    forget_after TIMESTAMP,
    last_accessed TIMESTAMP,
    valid_from TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    valid_until TIMESTAMP DEFAULT NULL
);

CREATE INDEX idx_observations_entity ON observations(entity_id);
CREATE INDEX idx_observations_fact_type ON observations(fact_type);

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

-- Observation vector embeddings (stored as little-endian IEEE 754 binary blobs)
CREATE TABLE observation_embeddings (
    observation_id INTEGER PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
    embedding BLOB NOT NULL,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_embeddings_model ON observation_embeddings(model);

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

### 3. Search Engine

Hybrid search combining multiple strategies:

```
┌─────────────────────────────────────────────────────────────┐
│                      Search Query                            │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
       ┌───────────┐                   ┌───────────┐
       │   FTS5    │                   │  Vector   │
       │  (BM25)   │                   │ (cosine)  │
       └───────────┘                   └───────────┘
              │                               │
              └───────────────┬───────────────┘
                              ▼
                    ┌───────────────────┐
                    │  RRF Score Fusion │
                    │      (k = 60)     │
                    └───────────────────┘
                              │
                              ▼
                    ┌───────────────────┐
                    │  Ranked Results   │
                    └───────────────────┘
```

**Keyword Search (FTS5 BM25):**
```sql
WITH observation_matches AS (
    SELECT DISTINCT o.entity_id, bm25(observations_fts) as score
    FROM observations_fts f
    JOIN observations o ON o.id = f.rowid
    WHERE observations_fts MATCH ?
    AND o.valid_until IS NULL
),
entity_matches AS (
    SELECT e.id as entity_id, bm25(entities_fts) as score
    FROM entities_fts f
    JOIN entities e ON e.id = f.rowid
    WHERE entities_fts MATCH ?
),
combined AS (
    SELECT entity_id, MIN(score) as score
    FROM (
        SELECT entity_id, score FROM observation_matches
        UNION ALL
        SELECT entity_id, score FROM entity_matches
    )
    GROUP BY entity_id
)
SELECT e.id, e.name, e.entity_type, e.created_at, c.score
FROM combined c
JOIN entities e ON e.id = c.entity_id
ORDER BY c.score
LIMIT ?;
```

**Vector Search (In-Process Pure Go):**
Vectors are stored as IEEE 754 Little-Endian binary blobs in SQLite, decoded into Go memory, and scored via cosine similarity filtered by active model and dimensions:

```sql
SELECT oe.observation_id, oe.embedding, o.content, e.name, e.entity_type
FROM observation_embeddings oe
JOIN observations o ON o.id = oe.observation_id
JOIN entities e ON e.id = o.entity_id
WHERE (o.valid_until IS NULL OR e.entity_type = 'session')
AND COALESCE(o.fact_type, 'dynamic') != 'session_event'
AND oe.model = ?
AND oe.dimensions = ?;
```

**Score Fusion (Reciprocal Rank Fusion):**
Results from keyword and semantic paths are combined using Reciprocal Rank Fusion ($k=60$) via typed `ResultKey{EntityName, Content}` identities to prevent cross-entity observation absorption:
$$RRF(d) = \sum_{m \in \text{strategies}} \frac{1}{k + \text{rank}_m(d)}$$

### 4. Embedding Pipeline

```
┌─────────────────┐     ┌───────────────────────┐     ┌────────────────────────┐
│  Observation    │────▶│ Ollama / DMR Local API│────▶│ SQLite Embeddings BLOB │
│  (text)         │     │ (nomic-embed-text)    │     │ (observation_embeddings│
└─────────────────┘     └───────────────────────┘     └────────────────────────┘
```

**Supported embedding endpoints:**
- Ollama: `http://localhost:11434/v1` (default)
- Docker Model Runner (DMR): `http://127.0.0.1:12434/engines/v1`
- Custom OpenAI-compatible endpoints configured via `CLAUDE_MEMORY_EMBEDDER_URL`

## Data Flow

### Create Entity / Observation

```
1. Harness: mcp__memory__create_entities({entities: [...]})
2. MCP Server: Parse request and validate schema
3. SQLite: INSERT INTO entities (name, entity_type) VALUES (?, ?)
4. SQLite: INSERT INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)
5. Triggers: SQLite triggers automatically update observations_fts and entities_fts
6. [Auto-Embed]: Local embedding client generates vector embedding
7. SQLite: INSERT INTO observation_embeddings VALUES (?, blob, model, dims)
8. MCP Server: Return structured success response with itemized diagnostics
```

### Search Nodes

```
1. Harness: mcp__memory__search_nodes({query: "..."})
2. MCP Server: Parse request
3. Storage: Execute FTS5 BM25 search on virtual tables
4. Storage: Generate query embedding and execute VectorSearchWithModel
5. Storage: Fuse candidates using Reciprocal Rank Fusion (k=60)
6. MCP Server: Return top-ranked entities with observation snippets
```

## File Structure

```
mark42/
├── README.md
├── docs/
│   ├── ARCHITECTURE.md        # Technical architecture and schema
│   ├── DESIGN_DECISIONS.md    # Design rationale
│   ├── CONFIGURATION.md       # Configuration options
│   ├── MIGRATION_GUIDE.md     # Migration from JSON Memory MCP
│   ├── TROUBLESHOOTING.md     # Diagnostic and recovery guide
│   └── adr/                   # Architecture Decision Records
│       ├── 0001-cancellation-and-timeouts.md
│       ├── 0002-query-time-decay-projection.md
│       └── 0003-consumer-driven-contracts-and-neutral-state.md
├── schemas/
│   └── session-capture.v1.json# Session capture JSON contract
├── cmd/
│   ├── memory/main.go         # CLI entry point
│   └── server/main.go         # MCP server entry point (stdio)
├── internal/
│   ├── storage/               # SQLite storage, FTS5, embeddings, RRF fusion
│   │   ├── store.go           # Lifecycle and schema
│   │   ├── search.go          # FTS5 full-text search
│   │   ├── vector.go          # Vector BLOB search & cosine similarity
│   │   ├── fusion.go          # Reciprocal Rank Fusion (RRF)
│   │   ├── embedding.go       # Ollama / DMR embedding client
│   │   ├── temporal.go        # Bitemporal validity & superseding
│   │   ├── importance.go      # Query-time decay & importance scoring
│   │   └── migrations/        # Versioned Goose Go migrations (001-011)
│   ├── mcp/                   # JSON-RPC 2.0 protocol and 20 tool handlers
│   ├── cli/                   # Cobra commands, formatters, and flags
│   ├── distill/               # Structural session distillation
│   ├── state/                 # State management and Strangler Fig migration
│   ├── paths/                 # Neutral path resolution (~/.mark42)
│   └── adapter/claude/        # Claude Code hook adapter
├── adapters/
│   ├── opencode/              # OpenCode JS plugin adapter
│   └── pi/                    # Pi MCP recall adapter
├── hooks/
│   └── hooks.json             # Hook configuration (session-start, stop, pre-compact)
├── go.mod
├── go.sum
└── Makefile
```

## Configuration

Configuration is managed through environment variables, CLI flags, and persisted settings:

- **Database Path**: `--db` flag, or `MARK42_DB` (primary) $\rightarrow$ `CLAUDE_MEMORY_DB` (legacy) $\rightarrow$ `~/.mark42/memory.db` (default).
- **Embedding Endpoint**: `CLAUDE_MEMORY_EMBEDDER_URL` (or default local Ollama at `http://localhost:11434/v1`). Set to `disabled` to disable semantic search.
- **Context Injection**: `CLAUDE_MEMORY_TOKEN_BUDGET` (default: 2000), `CLAUDE_MEMORY_BOOST` (default: 1.5).
- **Persisted Settings**: SQLite `settings` table maintains configuration dynamically tuned via `mark42 analytics tune --apply`.

## Performance Considerations

### Database

- **WAL mode & Concurrency**: Write-Ahead Logging with busy-wait tolerance
- **Connection pooling**: Controlled pool with serialized writers
- **Prepared statements**: Cache frequently used queries
- **Index coverage**: Ensure queries use indexes

```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
```

### Search & RRF Fusion

- **FTS5 tokenizer**: Use `porter` for English stemming
- **RRF Dedup Identity**: RRF fusion uses a typed `ResultKey{EntityName, Content}` to guarantee that distinct entities sharing identical observation text are never collapsed or lost during score combination
- **Vector Search Model Isolation**: Vector queries filter by active model (`WHERE oe.model = ?`) and dimensional parity to prevent cross-model vector space contamination
- **Stateless Query-Time Decay**: Importance decay is computed dynamically during scoring rather than via in-place database row mutations (see `docs/adr/0002-query-time-decay-projection.md`)

### Memory & Session Accretion

- **Immutable History**: Session distillation is a non-destructive projection; raw tool execution events are preserved in SQLite rather than deleted upon distillation
- **Single Source of Truth**: Database schema is unified via versioned Goose migrations from baseline version 0
- **Architectural Decision Records**: See `docs/adr/` for detailed architectural records

## Security

- **Local only**: No network exposure by default
- **File permissions**: Database file readable only by owner
- **No credentials**: No API keys or tokens stored
- **Sanitized queries**: All queries use parameterized statements

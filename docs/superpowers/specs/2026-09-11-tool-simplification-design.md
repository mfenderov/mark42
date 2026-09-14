# mark42 Tool Simplification Design (The 3-Verb Architecture)

**Date:** 2026-09-11
**Status:** Approved
**Topic:** MCP Tool Simplification & 3-Verb Architecture (`remember`, `recall`, `forget`)

---

## 1. Executive Summary & Problem Statement

### The Problem
`mark42` evolved through six phases into a feature-rich, local memory layer with 20 distinct Model Context Protocol (MCP) tools. While powerful, exposing 20 individual tools to AI coding agents causes critical operational issues:
1. **Context Window & Token Bloat:** Injecting 20 JSON schemas with detailed descriptions consumes ~2,500–3,000 tokens on **every single LLM turn** across all harnesses (Claude Code, Cursor, Copilot, etc.).
2. **Cognitive Load & Decision Paralysis:** LLMs frequently suffer from tool-selection confusion due to functional overlap:
   - Storing knowledge: `create_entities` vs `create_or_update_entities` vs `add_observations`
   - Retrieving knowledge: `search_nodes` vs `open_nodes` vs `get_context` vs `get_recent_context` vs `summarize_entity` vs `read_graph`
   - Removing knowledge: `delete_observations` vs `invalidate_observation` vs `delete_entities`
3. **Misplaced Admin Capabilities:** Tools such as `get_memory_analytics`, `get_tuning_recommendation`, `consolidate_memories`, and `read_graph` (which dumps the entire database into the context window) belong in developer CLI maintenance, not in the agent's real-time coding toolset.

### The Solution: Clean Break to 3 Verbs
A clean break from the legacy `@modelcontextprotocol/server-memory` specification to an ergonomic, unified **3-verb tool surface**:
- **`recall`**: Intelligent retrieval (hybrid search, topic inspection, or project-wide context injection).
- **`remember`**: Seamless knowledge capture (upserting entities, adding facts, managing fact types, linking relations, auto-embedding, and session logging).
- **`forget`**: Safe knowledge deprecation (soft-invalidation by default to preserve temporal history, or optional permanent deletion).

This architecture provides **100% parity across Go storage, MCP server, and the CLI**.

---

## 2. Architecture & Data Flow

```
                      +-----------------------------+
                      |   AI Coding Harness         |
                      |   (Claude / Cursor / Copilot|
                      +--------------+--------------+
                                     |
                                     | MCP JSON-RPC (stdio)
                                     v
                      +-----------------------------+
                      |       mark42-server         |
                      |   Exposes: remember, recall,|
                      |            forget           |
                      +--------------+--------------+
                                     |
                                     v
+-----------------------+     +-----------------------------+
|      mark42 CLI       |     |        Storage API          |
| mark42 remember/...   | --> | Store.Remember(...)         |
| (plus admin commands) |     | Store.Recall(...)           |
+-----------------------+     | Store.Forget(...)           |
                              +--------------+--------------+
                                             |
                                             v
                              +-----------------------------+
                              |        SQLite Layer         |
                              |  - entities & observations  |
                              |  - FTS5 & vector embeddings |
                              |  - relations & sessions     |
                              |  - temporal validity        |
                              +-----------------------------+
```

---

## 3. Detailed Component Design

### 3.1. MCP Tool Specifications

The MCP server registers exactly **3 tools**:

#### A. `remember`
Stores or updates facts, decisions, conventions, and relations under a named topic or entity.

- **Schema:**
  ```json
  {
    "name": "remember",
    "description": "Store or update knowledge in memory under a topic. Call this proactively whenever learning user preferences, personal facts, important decisions, rules, recurring patterns, or session milestones across any subject.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "topic": {
          "type": "string",
          "description": "The subject, entity, person, project, or concept name (e.g. 'user-preferences', 'alice', 'travel-plans', 'auth-system')"
        },
        "facts": {
          "type": "array",
          "items": { "type": "string" },
          "description": "One or more statements, rules, or observations to record"
        },
        "type": {
          "type": "string",
          "description": "Optional category or entity type (e.g. 'person', 'preference', 'decision', 'project', 'concept'). Default: 'concept'"
        },
        "fact_type": {
          "type": "string",
          "enum": ["static", "dynamic", "session"],
          "description": "Optional persistence level: 'static' (durable/evergreen fact or preference), 'dynamic' (active/changing state), or 'session' (conversation milestone/summary). Default: 'static'"
        },
        "relations": {
          "type": "array",
          "description": "Optional links to related topics",
          "items": {
            "type": "object",
            "properties": {
              "to": { "type": "string", "description": "Target topic name" },
              "type": { "type": "string", "description": "Relationship type (e.g. 'depends_on', 'implements', 'relates_to')" }
            },
            "required": ["to", "type"]
          }
        },
        "project": {
          "type": "string",
          "description": "Optional namespace or container tag to scope this memory (e.g. workspace, project, or domain)"
        }
      },
      "required": ["topic", "facts"]
    }
  }
  ```

#### B. `recall`
Retrieves relevant memories based on search query, specific topic, or project-wide context.

- **Schema:**
  ```json
  {
    "name": "recall",
    "description": "Retrieve memories from mark42. Call without arguments at the start of a conversation to load core preferences, durable facts, and recent context. Provide 'query' to search across memories using semantic and keyword search, or 'topic' to inspect a specific subject (takes precedence over query).",
    "inputSchema": {
      "type": "object",
      "properties": {
        "query": {
          "type": "string",
          "description": "Search query to find relevant memories, facts, preferences, or past discussions"
        },
        "topic": {
          "type": "string",
          "description": "Specific topic, entity, person, or concept name to inspect in detail"
        },
        "project": {
          "type": "string",
          "description": "Optional namespace, project, or container tag to scope the search or context"
        },
        "limit": {
          "type": "integer",
          "description": "Maximum number of results to return (default: 10)"
        }
      }
    }
  }
  ```

- **Execution Dispatch:**
  - `topic != ""`: Returns entity overview, active observations, inbound/outbound relations, and version history.
  - `query != ""`: Executes hybrid search (FTS5 BM25 + cosine vector similarity with RRF fusion) across observations and session entities.
  - `query == "" && topic == ""`: Functions as context injection, returning the project's top-importance static conventions plus recent session summaries.

#### C. `forget`
Soft-invalidates or permanently deletes facts or entire topics from memory.

- **Schema:**
  ```json
  {
    "name": "forget",
    "description": "Remove or invalidate knowledge when information changes, is superseded, or is no longer true. By default, soft-invalidates the fact (hiding it from future recall while preserving history). Set 'permanent: true' only to permanently delete. WARNING: Omitting 'fact' invalidates the ENTIRE topic.",
    "inputSchema": {
      "type": "object",
      "properties": {
        "topic": {
          "type": "string",
          "description": "Topic, entity, person, or concept name to forget from"
        },
        "fact": {
          "type": "string",
          "description": "Exact text or substring of the specific fact to forget. If omitted, invalidates all facts under the entire topic."
        },
        "permanent": {
          "type": "boolean",
          "description": "Whether to permanently delete from SQLite (true) or soft-invalidate with valid_until timestamp (false). Default: false"
        }
      },
      "required": ["topic"]
    }
  }
  ```
          "description": "Whether to permanently delete from SQLite (true) or soft-invalidate with valid_until timestamp (false). Default: false"
        }
      },
      "required": ["topic"]
    }
  }
  ```

---

### 3.2. Go Storage Core API (`internal/storage/verbs.go`)

Direct Go methods on `*storage.Store`:

```go
type RememberParams struct {
    Topic      string
    Facts      []string
    EntityType string
    FactType   FactType
    Relations  []RelationParam
    Project    string
}

type RelationParam struct {
    To   string
    Type string
}

type RememberResult struct {
    Topic             string
    ObservationsAdded int
    RelationsCreated   int
}

func (s *Store) Remember(ctx context.Context, params RememberParams) (*RememberResult, error)

type RecallParams struct {
    Query       string
    Topic       string
    Project     string
    Limit       int
    TokenBudget int
}

type RecallResult struct {
    Type          string
    Topic         *Entity
    Relations     []*Relation
    SearchResults []FusedResult
    Context       []ContextResult
    Sessions      []ContextResult
}

func (s *Store) Recall(ctx context.Context, params RecallParams) (*RecallResult, error)

type ForgetParams struct {
    Topic     string
    Fact      string
    Permanent bool
}

type ForgetResult struct {
    Topic     string
    Count     int
    Permanent bool
}

func (s *Store) Forget(ctx context.Context, params ForgetParams) (*ForgetResult, error)
```

---

### 3.3. CLI Parity (`internal/cli/verbs.go`)

Expose developer-friendly Cobra commands mirroring the 3 MCP verbs:

1. `mark42 remember <topic> <facts...> [--type T] [--rel "to:type"] [--fact-type static|dynamic|session] [--project P]`
2. `mark42 recall [query] [--topic T] [--project P] [--limit N]`
3. `mark42 forget <topic> [fact] [--permanent]`

---

## 4. Backward Compatibility & Data Safety

1. **Zero Database Schema Migrations:**
   All tables (`entities`, `observations`, `relations`, `embeddings`, `settings`) remain completely unchanged. Existing databases are 100% compatible.
2. **Session Compatibility:**
   Sessions captured as session entities in Phase 4 are immediately accessible via `recall`.
3. **Clean Break on MCP:**
   The MCP server advertises strictly `remember`, `recall`, and `forget`.

---

## 5. Testing & Verification Plan

1. **Storage Tests (`internal/storage/verbs_test.go`):**
   - `TestStore_Remember`: tests entity creation, upsert, fact addition, fact types, relation linking, and auto-embedding.
   - `TestStore_Recall`: tests topic retrieval, hybrid search retrieval, and default context + session recall.
   - `TestStore_Forget`: tests soft-invalidation vs permanent deletion for specific facts and entire topics.
2. **MCP Handler Tests (`internal/mcp/handlers_test.go`):**
   - Verifies `Tools()` returns exactly 3 tools.
   - Verifies JSON-RPC invocations of `remember`, `recall`, and `forget` with valid and invalid inputs.
3. **CLI Tests (`internal/cli/verbs_test.go`):**
   - Verifies Cobra flags, argument parsing, and output formatting for `remember`, `recall`, and `forget`.
4. **Full Test Suite & Linters:**
   - Run `make test` (full suite with race detector).
   - Run `make lint` and `make crap`.

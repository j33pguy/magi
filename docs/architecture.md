# Architecture

## Overview

magi is a single Go binary that runs four server interfaces concurrently, all backed by the same database and embedding engine.

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          magi (Go binary)                       │
│                                                                          │
│  ┌─────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  MCP    │  │   gRPC       │  │ grpc-gateway │  │  Legacy HTTP    │  │
│  │  stdio  │  │   :8300      │  │   :8301      │  │    :8302        │  │
│  └────┬────┘  └──────┬───────┘  └──────┬───────┘  └───────┬────────┘  │
│       │              │                  │                   │           │
│  ┌────┴──────────────┴──────────────────┴───────────────────┴────────┐  │
│  │                     Tool / Handler Layer                          │  │
│  │  remember · recall · forget · list · update · index_turn          │  │
│  │  store_conversation · recall_conversations · ingest · patterns    │  │
│  │  link_memories · get_related · check_contradictions               │  │
│  └──────────────────────────┬────────────────────────────────────────┘  │
│                             │                                           │
│  ┌──────────────────────────┴────────────────────────────────────────┐  │
│  │                       Core Services                               │  │
│  │                                                                    │  │
│  │  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐  │  │
│  │  │ Embeddings  │  │   Search     │  │     Classification      │  │  │
│  │  │ ONNX local  │  │ Hybrid RRF   │  │  55 regex rules         │  │  │
│  │  │ MiniLM-L6   │  │ BM25+vector  │  │  6 areas, auto-infer   │  │  │
│  │  └─────────────┘  └──────────────┘  └─────────────────────────┘  │  │
│  │                                                                    │  │
│  │  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐  │  │
│  │  │Contradiction│  │   Patterns   │  │      Ingestion          │  │  │
│  │  │ Detection   │  │  Behavioral  │  │  Grok/ChatGPT/text      │  │  │
│  │  │ Heuristics  │  │  Heuristics  │  │  format auto-detect     │  │  │
│  │  └─────────────┘  └──────────────┘  └─────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                             │                                           │
│  ┌──────────────────────────┴────────────────────────────────────────┐  │
│  │                     Database Layer (db/)                           │  │
│  │                                                                    │  │
│  │  Memory CRUD · Tags · Links · FTS5 · Vector Search · Migrations   │  │
│  │                                                                    │  │
│  │  ┌──────────────────────┐      ┌────────────────────────────┐     │  │
│  │  │  Local SQLite Replica │◀───▶│     Turso Cloud Database   │     │  │
│  │  │  ~/.magi.db  │ sync │  libsql + vector search   │     │  │
│  │  │  (fast offline reads) │      │  (distributed, durable)   │     │  │
│  │  └──────────────────────┘      └────────────────────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                       Web UI (:8080)                              │   │
│  │  HTMX · Dark theme · Memory browser · Graph viz · Import page    │   │
│  └──────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────┘
```

## Distributed Node Mesh

Located in `internal/node/`. The node mesh introduces a coordination layer between protocol handlers and the database, routing reads and writes through typed goroutine pools.

### Node Types

| Type | Interface | Responsibility |
|------|-----------|----------------|
| Writer | `node.Writer` | Persists memories: Save, Update, Archive, Delete |
| Reader | `node.Reader` | Retrieves memories: Get, List, Search, SearchBM25, HybridSearch |
| Index | `node.Index` | Manages search index updates (tag reindexing) |
| Coordinator | `node.Coordinator` | Routes requests to Writer/Reader pools, manages lifecycle |

### Architecture (Phase 1 — Embedded Mode)

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Protocol Layer                                   │
│         MCP · gRPC · REST · Web UI                                   │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│                     Coordinator                                      │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Router (session affinity)                   │   │
│  │  Tracks per-session write sequence numbers for                │   │
│  │  read-your-writes consistency                                 │   │
│  └───────────┬──────────────────────────────┬────────────────────┘   │
│              │                              │                        │
│  ┌───────────▼───────────┐    ┌─────────────▼─────────────────┐     │
│  │    Writer Pool (4)    │    │      Reader Pool (8)           │     │
│  │  ┌──┐ ┌──┐ ┌──┐ ┌──┐ │    │  ┌──┐ ┌──┐ ┌──┐ ┌──┐        │     │
│  │  │W1│ │W2│ │W3│ │W4│ │    │  │R1│ │R2│ │R3│ ... │R8│      │     │
│  │  └──┘ └──┘ └──┘ └──┘ │    │  └──┘ └──┘ └──┘     └──┘      │     │
│  │  channel buffer: 8    │    │  channel buffer: 16            │     │
│  └───────────┬───────────┘    └─────────────┬─────────────────┘     │
│              │                              │                        │
│  ┌───────────▼──────────────────────────────▼────────────────────┐  │
│  │                    Index Pool                                  │  │
│  │  Tag updates, FTS trigger pass-through (inline in embedded)   │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Registry                                    │   │
│  │  Tracks registered node capabilities and pool sizes            │   │
│  └──────────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│                     db.Store (any backend)                            │
└─────────────────────────────────────────────────────────────────────┘
```

### Session Affinity

The Router tracks a monotonic write sequence per session. After a write completes, subsequent reads from the same session are guaranteed to see that write. Session IDs propagate via `context.Context` from HTTP/gRPC handlers down to the pools.

### CoordinatedStore

`local.CoordinatedStore` implements the `db.Store` interface by delegating core operations (Save, Get, Update, Archive, Delete, List, Search) through the Coordinator pools. Operations not yet routed through pools (links, graph traversal, tags) pass through to the underlying store directly. This makes the node mesh a **drop-in replacement** — all existing API/gRPC/MCP endpoints work unchanged.

### Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_NODE_MODE` | `embedded` | Node communication mode (Phase 1: embedded only) |
| `MAGI_WRITER_POOL_SIZE` | `4` | Number of writer goroutines |
| `MAGI_READER_POOL_SIZE` | `8` | Number of reader goroutines |
| `MAGI_COORDINATOR_ENABLED` | `true` | Enable the coordinator (set `false` for direct store access) |

In embedded mode, all pools run as in-process goroutines communicating via Go channels. Zero serialization overhead — the same `*db.Memory` pointers pass through the pools.

## Data Flow

### Remember Service Layer

The `internal/remember` service layer centralizes write enrichment so every transport shares the same behavior. MCP, gRPC, REST, and the async pipeline all call into this layer to guarantee consistent classification, safety checks, and dedup logic.

Enrichment stages include:
- Secret detection and rejection
- Area/sub_area auto-classification
- Embedding generation
- Deduplication and soft-group linking
- Tag enrichment
- Contradiction detection (non-blocking)

### Write Path (remember)

When `MAGI_ASYNC_WRITES=true`, writes go through the async pipeline (see below) and return 202 Accepted in under 10ms. The synchronous path is:

```
1. Content received via MCP/gRPC/HTTP
2. Secret detection — reject if potential secrets found
3. Auto-classify area/sub_area (55 regex rules)
4. Generate 384-dim embedding (ONNX, all-MiniLM-L6-v2)
5. Deduplication check — cosine similarity against existing memories
   - >0.95 similarity: return existing memory ID (exact dedup)
   - >0.85 similarity: link as soft group (parent_id)
6. Save to database via configured backend
7. Set tags (taxonomy + user-provided)
8. Contradiction detection — search same area/sub_area, apply heuristics
   - Numeric change detection (e.g., "port 8080" vs "port 9090")
   - Boolean flip detection (e.g., "enabled" vs "disabled")
   - Replacement language detection (e.g., "now uses", "switched to")
9. If git versioning enabled, commit memory snapshot to git repo
10. Return memory ID + any contradiction warnings
```

### Read Path (recall)

```
1. Query received via MCP/gRPC/HTTP
2. Generate query embedding
3. Parallel search:
   a. Vector similarity (cosine distance against F32_BLOB embeddings)
   b. BM25 keyword search (FTS5 full-text index)
4. Reciprocal Rank Fusion (RRF) — merge ranked results
5. Apply filters (project, type, tags, speaker, area, time)
6. Apply recency decay weighting (optional exponential decay)
7. Apply min_relevance threshold
8. If no results pass threshold: rewrite query and retry once (adaptive search)
9. Return ranked results with scores
```

### Async Write Pipeline

Located in `internal/pipeline/`. When `MAGI_ASYNC_WRITES=true`, writes are dispatched to a worker pool instead of running synchronously.

```
1. Content submitted via MCP/gRPC/HTTP → returns 202 Accepted immediately (<10ms)
2. StatusTracker records state: pending → processing
3. Worker picks item from buffered channel
4. Per-worker pipeline:
   a. ID generation
   b. Embedding (ONNX)
   c. Classification (55 regex rules)
   d. Deduplication check
   e. Soft-group linking
   f. Tag assignment
   g. Contradiction check
5. BatchInserter collects completed writes
6. Flush to database on time (100ms) or size (50 items), whichever comes first
7. StatusTracker updates state: complete or failed (5-min TTL cleanup)
```

- API: `GET /memories/:id/status` returns write state (pending, processing, complete, failed)
- API: `GET /pipeline/stats` returns queue depth, batch pending, worker count, totals

### Sync Path

```
Local embedded replica ◀──── periodic sync (default 60s) ────▶ Turso cloud

- Reads: always from local replica (fast, offline-capable)
- Writes: to local replica, synced to cloud
- Multiple magi instances stay in sync via Turso
```

## Git-Backed Memory Versioning

Located in `internal/vcs/`. The VersionedStore middleware wraps `db.Client`, intercepting all mutations to maintain a git-backed history of memory changes.

- Writes to a git repo at `MAGI_GIT_PATH` (default `~/.magi/memories`)
- File layout: `memories/{id}.json` (embeddings excluded), `links/{fromId}.json`
- Commit modes: **immediate** (one commit per mutation) or **batch** (timer-based flush)
- Best-effort: git failures are logged as warnings; database operations always succeed
- Auto-rebuild: if the database is empty but an existing git repo is found, full rebuild runs on startup
- API: `GET /memories/:id/history` returns commit log, `GET /memories/:id/diff` returns unified diff between commits
- Enabled via `MAGI_GIT_ENABLED=true`

## Caching Layer

Located in `internal/cache/`. Three independent caches reduce latency for repeated operations:

| Cache | Key | Default Size | Default TTL |
|-------|-----|-------------|-------------|
| QueryCache | SHA256(query + filter + topK) | unbounded | 60s |
| MemoryCache | memory ID (LRU) | 1000 items | -- |
| EmbeddingCache | SHA256(content) (LRU) | 5000 items | -- |

- Conservative invalidation: the query cache is cleared on any write operation
- Memory and embedding caches use LRU eviction

## Observability

### Prometheus Metrics

Located in `internal/metrics/`. Exposed at `GET /metrics` in Prometheus exposition format.

| Metric | Type | Description |
|--------|------|-------------|
| `magi_write_latency_seconds` | Histogram | Latency of memory write operations |
| `magi_search_latency_seconds` | Histogram | Latency of memory search operations |
| `magi_embedding_duration_seconds` | Histogram | Duration of ONNX embedding generation |
| `magi_queue_depth` | Gauge | Current depth of async write pipeline |
| `magi_memory_count` | Gauge | Current number of memories in database |
| `magi_active_sessions` | Gauge | Number of active MCP sessions |
| `magi_cache_hits_total` | Counter | Cache hits (label: `cache` = query/memory/embedding) |
| `magi_cache_misses_total` | Counter | Cache misses (label: `cache` = query/memory/embedding) |
| `magi_git_commits_total` | Counter | Total git commits made for memory versioning |

### Health Probes

| Endpoint | Auth | Checks | Success | Failure |
|----------|------|--------|---------|---------|
| `GET /health` | No | DB query, git status | 200 with version, uptime, db_status, memory_count, git_status | 503 |
| `GET /readyz` | No | DB list query | 200 `{"ready": true}` | 503 with error |
| `GET /livez` | No | None (process alive) | 200 `{"alive": true}` | — |

`/readyz` is suitable for Kubernetes readiness probes. `/livez` is suitable for liveness probes.

### Write Tracking

Located in `internal/tracking/`. Convenience helpers for production dogfooding — MAGI recording its own operational state as memories.

| Helper | Memory Type | Tags | Content Format |
|--------|-------------|------|----------------|
| `TrackTask(id, state, metadata)` | `task` | `task`, `state:<state>`, `task:<id>` | `Task {id} → {state}` + metadata |
| `TrackDecision(summary, context)` | `decision` | `decision`, `architectural` | `Decision: {summary}\n\nContext: {context}` |
| `TrackConversation(summary, topics, decisions, items)` | `conversation` | `conversation`, `tracking`, `topic:<t>` | Structured summary with sections |

All tracking writes use `speaker: "system"` and `source: "tracking"`.

## Pluggable SQL Backends

A factory pattern in `internal/db/factory.go` selects the database backend at startup. All backends implement the `db.Store` interface.

| Backend | Selection | Vector Search | Full-Text Search | Notes |
|---------|-----------|---------------|------------------|-------|
| SQLite | `MEMORY_BACKEND=sqlite` | In-process cosine | FTS5 | Local, WAL mode, default |
| Turso | `MEMORY_BACKEND=turso` | libSQL native | FTS5 | Cloud libSQL + embedded replica |
| PostgreSQL | `MEMORY_BACKEND=postgres` | pgvector | tsvector | Native vector and FTS |
| MySQL | `MEMORY_BACKEND=mysql` | App-side reranking | App-side | Vector reranking in Go |
| SQL Server | `MEMORY_BACKEND=sqlserver` | App-side reranking | App-side | Vector reranking in Go |

Selected via the `MEMORY_BACKEND` environment variable. Each backend handles its own migrations.

## Database Schema

Seven incremental migrations:

| Version | Description |
|---------|-------------|
| V1 | Core tables: `memories` (with F32_BLOB(384) embedding), `memory_tags`, vector index |
| V2 | Visibility column: `private`, `internal`, `public` |
| V3 | FTS5 full-text search with auto-sync triggers (for BM25 in hybrid search) |
| V4 | Rich memory types: renamed `note` → `memory`, type index |
| V5 | Structured taxonomy: `speaker`, `area`, `sub_area` columns with indexes |
| V6 | Temporal index on `created_at` for time-range queries |
| V7 | Memory links table: `memory_links` with directed relationships and graph traversal |

### Key Tables

**memories** — Core storage
```sql
id TEXT PRIMARY KEY          -- random 16-byte hex
content TEXT NOT NULL         -- full content
summary TEXT                  -- optional one-liner
embedding F32_BLOB(384)       -- ONNX vector embedding
project TEXT NOT NULL          -- namespace
type TEXT DEFAULT 'memory'     -- decision, lesson, incident, etc.
visibility TEXT DEFAULT 'internal'
speaker TEXT                   -- user, assistant, agent, system
area TEXT                      -- work, infrastructure, development, personal, project, meta
sub_area TEXT                  -- power-platform, networking, magi, etc.
parent_id TEXT                 -- soft-grouping via dedup
created_at TEXT
updated_at TEXT
archived_at TEXT               -- non-null = soft-deleted
token_count INTEGER
```

**memory_tags** — Many-to-many tags
```sql
memory_id TEXT, tag TEXT       -- composite PK
```

**memory_links** — Directed graph relationships
```sql
from_id TEXT, to_id TEXT       -- directed edge
relation TEXT                  -- caused_by, led_to, related_to, supersedes, part_of, contradicts
weight REAL DEFAULT 1.0
auto INTEGER DEFAULT 0         -- 1 if system-generated
```

**memories_fts** — FTS5 virtual table for BM25 keyword search, auto-synced via triggers.

## Embedding Pipeline

All embeddings are generated locally — no cloud API calls, no data leaves the machine.

```
Content → BERT WordPiece tokenizer → all-MiniLM-L6-v2 (ONNX) → 384-dim float32 vector
```

- Model: [all-MiniLM-L6-v2](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2) via ONNX Runtime
- Dimensions: 384
- Storage: `F32_BLOB(384)` in Turso/libSQL
- Distance: Cosine similarity via `vector_distance_cos()`
- Tokenizer: Custom BERT WordPiece implementation (no Python, no external tokenizer service)

## Classification System

55 regex rules automatically infer `area` and `sub_area` from memory content:

| Area | Sub-areas |
|------|-----------|
| work | power-platform, fabric, power-bi, sharepoint, td-synnex, azure |
| infrastructure | iac, compute, dns, networking, security, ci-cd, monitoring, storage |
| project | magi, my-app, cli-tool, backup-service |
| home | lego, streaming, gaming |
| family | kids, spouse |
| meta | _(reserved)_ |

Classification runs on every `remember` and `index_turn` call when area/sub_area are not explicitly provided.

## Contradiction Detection

Three heuristics score potential contradictions (0.0–1.0):

| Heuristic | Score | Example |
|-----------|-------|---------|
| Numeric change | +0.7 | "port 8080" vs "port 9090" |
| Boolean flip | +0.6 | "TLS enabled" vs "TLS disabled" |
| Replacement language | +0.4 | "now uses Ansible" vs previous Terraform mention |

Scores are summed and capped at 1.0. Candidates scoring >0.5 are returned as warnings. Writes are never blocked.

## Behavioral Pattern Learning

Heuristic-based analysis (no LLM) detects four pattern types:

| Type | What it detects |
|------|-----------------|
| `preference` | Technology choices (13 tech stacks tracked), explicit prefer/avoid language |
| `decision_style` | Security-first, comparative evaluation, decisive |
| `work_pattern` | Weekend concentration by area, peak activity hours |
| `comms_style` | Concise (<100 chars), detailed (>500 chars), direct (low question ratio) |

Patterns are stored as `type=preference` memories with `pattern` tag, deduplicated at 0.9 similarity.

## Chaos Testing

Located in `internal/chaos/`. Build tag: `chaos`. Run with `go test -tags chaos ./internal/chaos/`.

| Test | What It Validates |
|------|-------------------|
| `TestConcurrentWrites` | 10 writers × 20 writes — no data corruption, all successful writes durable |
| `TestSearchDuringIngestion` | 3 writer + 3 search goroutines running concurrently — reads don't block writes in WAL mode |
| `TestKillMidWriteRecovery` | Cancel context mid-write — pre-existing data survives, DB remains functional |
| `TestCacheOverflow` | 200 sequential writes then 5 concurrent readers — cache pressure doesn't corrupt reads |

Some SQLite contention errors (`database is locked`) are expected under heavy concurrent writes. Tests verify that successful writes are durable and the system recovers.

## Project Structure

```
magi/
├── main.go                  # Entry point, CLI flags, server startup
├── cmd/mcp-config/          # MCP config generator subcommand
├── server/                  # MCP server setup, tool/resource registration
├── db/                      # Turso client, schema migrations, CRUD, tags, links
├── internal/
│   ├── api/                 # HTTP API server, health/readyz/livez handlers
│   ├── db/                  # Pluggable SQL backends (factory, store interface)
│   ├── node/                # Distributed node mesh (types, pool, router, registry)
│   │   └── local/           # Embedded-mode implementations (coordinator, writer, reader, index, store)
│   ├── metrics/             # Prometheus metrics (9 metrics, /metrics handler)
│   ├── tracking/            # Write tracking helpers (TrackTask, TrackDecision, TrackConversation)
│   ├── chaos/               # Chaos testing framework (concurrent writes, kill recovery, etc.)
│   ├── vcs/                 # Git-backed memory versioning (VersionedStore)
│   ├── pipeline/            # Async write pipeline (workers, batching, status)
│   ├── cache/               # Caching layer (query, memory, embedding caches)
│   └── server/              # Server initialization and lifecycle
├── tools/                   # MCP tool handlers (17 tools)
├── resources/               # MCP resource handlers (6 resources)
├── api/                     # Legacy HTTP API handlers
├── grpc/                    # gRPC service implementation + auth interceptor
├── web/                     # Web UI (HTMX templates, routes, static assets)
├── embeddings/              # ONNX embedding provider + BERT tokenizer
├── search/                  # Adaptive hybrid search (BM25 + vector + RRF)
├── classify/                # Rule-based area/sub_area classification
├── contradiction/           # Contradiction detection heuristics
├── patterns/                # Behavioral pattern analyzer + storage
├── ingest/                  # Multi-format conversation import pipeline
├── chunking/                # Markdown-aware text splitter
├── migrate/                 # Markdown file importer
├── cmd/import/              # CLI for importing existing memory files
├── proto/memory/v1/         # Protobuf definitions + generated code
├── .github/workflows/       # CI/CD (build, test, deploy)
└── Makefile                 # Build targets
```

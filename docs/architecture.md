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

## Data Flow

### Write Path (remember)

```
1. Content received via MCP/gRPC/HTTP
2. Secret detection — reject if potential secrets found
3. Auto-classify area/sub_area (55 regex rules)
4. Generate 384-dim embedding (ONNX, all-MiniLM-L6-v2)
5. Deduplication check — cosine similarity against existing memories
   - >0.95 similarity: return existing memory ID (exact dedup)
   - >0.85 similarity: link as soft group (parent_id)
6. Save to Turso via embedded replica
7. Set tags (taxonomy + user-provided)
8. Contradiction detection — search same area/sub_area, apply heuristics
   - Numeric change detection (e.g., "port 8080" vs "port 9090")
   - Boolean flip detection (e.g., "enabled" vs "disabled")
   - Replacement language detection (e.g., "now uses", "switched to")
9. Return memory ID + any contradiction warnings
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

### Sync Path

```
Local embedded replica ◀──── periodic sync (default 60s) ────▶ Turso cloud

- Reads: always from local replica (fast, offline-capable)
- Writes: to local replica, synced to cloud
- Multiple magi instances stay in sync via Turso
```

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
speaker TEXT                   -- j33p, gilfoyle, agent, system
area TEXT                      -- work, home, family, homelab, project, meta
sub_area TEXT                  -- power-platform, proxmox, magi, etc.
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
| homelab | iac, proxmox, dns, networking, vault, authentik, monitoring, lancache |
| project | magi, distify, labctl, vault-unsealer |
| home | lego, streaming, gaming |
| family | kids, spouse |
| meta | _(reserved)_ |

Classification runs on every `remember` and `index_turn` call when area/sub_area are not explicitly provided.

## Contradiction Detection

Three heuristics score potential contradictions (0.0–1.0):

| Heuristic | Score | Example |
|-----------|-------|---------|
| Numeric change | +0.7 | "VLAN 5" vs "VLAN 150" |
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

## Project Structure

```
magi/
├── main.go                  # Entry point, CLI flags, server startup
├── server/                  # MCP server setup, tool/resource registration
├── db/                      # Turso client, schema migrations, CRUD, tags, links
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

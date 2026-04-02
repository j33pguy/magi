# magi Roadmap

## Recently Shipped

### v0.3.0 — Security & Consistency

| Fix | PR | Notes |
|-----|-----|-------|
| Web UI auth + visibility enforcement | — | Web UI now requires Bearer auth and respects visibility |
| Unified remember enrichment | — | Consistent classify/secret detection/dedup/contradiction across MCP, gRPC, REST |
| Async write pipeline functional | — | `MAGI_ASYNC_WRITES=true` now works end-to-end |
| gRPC graph parity | — | `LinkMemories` and `GetRelated` implemented |
| PostgreSQL/MySQL factory wiring | — | Backends now wired into the store factory |

### v0.2.0

| Feature | PR | Notes |
|---------|-----|-------|
| Distributed node mesh (Phase 1) | #74 | Writer/Reader/Index/Coordinator pools, session affinity, embedded mode |
| Health probes (/readyz, /livez) | #73 | Kubernetes-style readiness and liveness probes |
| Expanded /health endpoint | #73 | DB status, uptime, memory count, git status |
| Write tracking helpers | #73 | TrackTask, TrackDecision, TrackConversation |
| MCP config generator | #73 | `magi mcp-config` subcommand |
| Chaos testing framework | #73 | Concurrent writes, search-during-ingestion, kill recovery, cache overflow |
| SQL Server backend | #71 | Full SQL Server / Azure SQL support |
| Go concurrency improvements | #72 | Benchmarks, performance tuning |

### v0.1.0

| Feature | PR | Notes |
|---------|-----|-------|
| Git-backed memory versioning | #62 | VersionedStore middleware, history/diff API |
| Async write pipeline + caching layer | #63 | 202 Accepted in <10ms, query/memory/embedding caches |
| Go concurrency improvements | #65 | Benchmarks, performance tuning |
| Pluggable SQL backends | #67 | SQLite, Turso, PostgreSQL, MySQL |
| SetTags transaction + REST dedup fix | #58 | Bug fixes |

### v0.1.0 — Baseline (shipped)
- MCP stdio server (agent integration)
- HTTP API (OpenClaw/external services)
- Turso cloud sync with local libSQL replica
- ONNX embeddings (all-MiniLM-L6-v2, local, no API key)
- Tools: remember, recall, forget, list_memories, update_memory
- Resources: recent, decisions, preferences
- Markdown import (cmd/import)
- Git-backed memory versioning (PR #62)
- Async write pipeline with worker pool (PR #63)
- Caching layer: query, memory, embedding caches (PR #63)
- Pluggable SQL backends: SQLite, Turso, PostgreSQL, MySQL, SQL Server (PR #67, #71)
- Go concurrency improvements and benchmarks (PR #65, #72)

---

## Cross-Cutting Priorities

These themes now cut across multiple releases and should stay visible in planning:

- shared memory and continuity across machines, sessions, and agents
- speed-first single-node performance
- role-separated container scale-out
- `magi-sync` as the edge bridge from isolated machines into MAGI
- authenticated user, machine, and agent identity
- owner/viewer/group-aware access control
- safe schema and data migration for major DB evolution

See also:

- `docs/strategy-rollup.md`
- `docs/magi-sync-design.md`
- `docs/auth-architecture.md`
- `docs/migration-strategy.md`

## v0.3.1 — Container-Ready Distributed Node Mesh (next)

### The pattern
Keep the in-process fast path for single-node deployments, but add a distributed transport so MAGI roles can run in separate LXC/Docker containers and scale independently.

### Features

#### gRPC node transport
- Keep Go channels for embedded mode
- Add gRPC streams between node roles in distributed mode
- Node discovery via registry (mDNS or static config)
- Writer/Reader/Index/Embedder nodes can run as separate processes or containers

#### Partition-aware routing
- Coordinator routes writes by project hash to specific Writer nodes
- Reader queries fan out to all Reader nodes, results merged
- Consistent hashing for partition assignment

#### Health-aware pool management
- Unhealthy nodes removed from rotation
- Automatic rebalancing when nodes join/leave
- Metrics per-node for capacity planning

#### Speed-first autoscaling signals
- Scale on queue depth, embedding duration, write latency, and search latency
- Split embedder capacity from API capacity
- Preserve read-your-writes guarantees per session where possible

---

## v0.3.2 — Dedicated Worker Containers

### The pattern
Specialize containers by role so the hot path can be scaled without cloning the entire application.

### Features

#### Role-separated containers
- `magi-api` for ingress, auth, and orchestration
- `magi-writer` for write enrichment and persistence
- `magi-reader` for search and recall
- `magi-index` for background maintenance
- `magi-embedder` for CPU-bound embedding work

#### Queue-aware backpressure
- Writers stop accepting unbounded work under sustained overload
- Embedders expose queue depth and saturation explicitly
- Graceful degradation to BM25-first recall when embedding capacity is constrained

---

## v0.3.3 — Replication and Read Scaling

### The pattern
Replicated reads with strong primary writes and scalable read capacity.

### Features

#### Read replicas
- Dedicated read-focused nodes or services
- Auto-scaling read capacity without affecting write path
- Configurable stale-read tolerance for lower latency

#### Replication-aware routing
- Prefer local readers when replicas are healthy
- Protect primary writer latency under heavy recall traffic
- Track replication lag and expose it in health/metrics

---

## v0.3.4 — Auth And Identity Foundation

### The pattern

Authenticate humans and machines separately, then derive trusted user, machine, and agent identity inside MAGI.

### Features

#### Human auth lane

- OIDC-backed UI and human API access
- Auth proxy support for self-hosted and enterprise deployments
- group-aware identity mapping

#### Machine auth lane

- per-machine tokens for `magi-sync`
- machine identity registry
- machine revocation and rotation path
- dedicated sync/API endpoint for non-browser clients

#### Access model

- owner/viewer/viewer_group semantics documented and enforced
- query filtering across HTTP, MCP, and web surfaces
- visibility levels expanded to support team/shared workflows

---

## v0.3.5 — Schema And Data Migration Framework

### The pattern

Keep startup schema migrations for additive DDL, but add a first-class path for major memory-model changes, backfills, and safe operator-managed upgrades.

### Features

#### Data migration tracking

- add `data_migrations` alongside `schema_migrations`
- make backfills idempotent and resumable
- track status for long-running migration work

#### Expand-and-contract rollout

- add new structures first
- backfill old memories in batches
- switch reads and writes after validation
- remove legacy structures in a later release

#### Recovery and safety

- clearer backup requirements before major upgrades
- export/import fallback for operator-managed changes
- tighter alignment with git-backed rebuild paths

## v0.4 — Project-Scoped Memory

### The pattern
Download a repo → binary detects it → syncs project memories → work begins with full context.

### Features

#### Project auto-detection
- On startup, run `git remote get-url origin` in cwd
- Parse to a canonical project key: `github.com/yourname/your-repo`
- Set as default project tag for all reads/writes in this session
- Fallback: directory name if not a git repo

#### Sync gate
- Before serving any MCP tool call, check last sync timestamp for current project
- If stale (> `MAGI_SYNC_MAX_AGE`, default 5 min): force sync, wait for completion
- Emit a log line: `syncing project memories for yourname/your-repo...`
- Then serve the request

#### Auto-tag on write
- `remember` calls inherit current project if `project` field is empty
- Never have to manually specify project — comes from git context

#### New MCP resource: `memory://sync-status`
```json
{
  "project": "github.com/yourname/your-repo",
  "last_sync": "2026-03-20T21:00:00Z",
  "seconds_ago": 183,
  "pending_writes": 0,
  "memory_count": 47,
  "status": "fresh"   // fresh | stale | syncing | error
}
```
The agent reads this before starting work. If `stale` or `syncing`, waits.

#### New MCP tool: `sync_now`
Force a sync immediately. Returns when complete.
```json
{"synced_at": "...", "records_synced": 12, "project": "..."}
```

#### New env var: `MAGI_SYNC_MAX_AGE`
Seconds before a sync is considered stale. Default: 300 (5 min).

### Config injection (`project config` injection)
```markdown
Before starting any task:
1. Check memory://sync-status
2. If status is not "fresh", call sync_now and wait
3. Call recall with the task description to load relevant context
4. Proceed with task
```

---

## v0.4.1 — Project Context in Turso (no CLAUDE.md in repos)

### The pattern
Instead of committing `CLAUDE.md` to every project, store project instructions in Turso
tagged as `type: project_context`. Clean repos, instructions travel with the memory.

### How it works
- Special memory type: `project_context` — highest priority in recall
- Stored as: `remember("Use Go 1.21+. All errors wrapped. No globals.", type="project_context", project="yourname/your-repo")`
- On project detection, `memory://project-context` resource returns these immediately
- The agent reads them before any other context

### New MCP resource: `memory://project-context`
Returns all `project_context` memories for the current project, formatted as instructions.
The agent injects these at the top of context automatically.

### Migration
- Existing `CLAUDE.md` files can be imported: `magi-import --type project_context --file CLAUDE.md`
- After import: delete the file, add to `.gitignore` as a safety net
- Instructions now invisible to anyone browsing the repo

### Benefits
- Repos stay clean — no AI instruction files visible
- Instructions version-controlled inside Turso, not Git
- Update instructions with `update_memory` without touching the repo
- Different instructions per branch if needed (future)

---

## v0.5 — Cross-Machine Identity

### The pattern
Multiple machines (server-01, laptop, future machines) all write to the same MAGI server through authenticated edge sync and agent connections.

### Features

#### Machine identity tag
- Each enrolled machine has a stable identity (e.g. `server-01`, `laptop`, `work-laptop`)
- Stored on synced records as identity tags like `machine:server-01`
- Useful for: "what did I work on from my laptop last week?"

#### User and agent identity
- Support `UserA.MachineA.AgentX` identity chains
- Store `owner`, `viewer`, and `viewer_group` metadata alongside memories
- Enable per-user and per-team filtering for recall and search

#### Auth and access
- per-machine token enrollment first
- dedicated sync write endpoint for `magi-sync`
- owner/viewer/viewer_group-aware filtering on authenticated query paths
- OIDC / Authentik path for humans later

#### New MCP resource: `memory://machines`
Shows which machines have written recently, last seen timestamps.

---

## v0.5 — Ingestion Pipeline

### The pattern
Work offline on laptop → Agent session logs captured → ingested into MAGI → the server agent has full context on next session.

### Features

#### Edge sync service
- Separate binary: `magi-sync`
- Scans local agent/project artifacts and uploads approved context
- Parses JSONL session logs
- Stores session summaries, per-turn conversation memories, and project context
- Uses machine enrollment plus `POST /sync/memories`

#### Import sources
- Session logs (`~/.magi/projects/**/*.jsonl`)
- Git commit messages (decisions + context)
- Markdown notes (manual, existing `cmd/import`)
- Future: VS Code history, terminal history

#### Sync config (`~/.config/magi-sync/config.yaml`)
```yaml
server:
  url: https://sync.memory.example.com
  enroll_token_env: MAGI_ADMIN_TOKEN
machine:
  id: laptop
  user: UserA
  groups:
    - platform
agents:
  - type: claude
    name: claude-main
    enabled: true
    owner: UserA
    viewer_groups:
      - platform
    paths:
      - ~/.claude
    include:
      - "**/projects/**/*.jsonl"
      - "**/CLAUDE.md"
    exclude:
      - "**/cache/**"
```

#### Packaging and rollout

- Homebrew formula for macOS
- `deb`, `rpm`, and `apk` artifacts for Linux
- `winget` package for Windows
- Tailscale-friendly deployment guidance for remote sync

---

## v0.6 — Cross-Channel Conversation Sync (Issue #6)

### The problem
OpenClaw sessions are isolated. MEMORY.md covers long-term facts. Nothing covers 
"what did we discuss 20 minutes ago in Discord?" when talking in webchat.

### New HTTP endpoints
- `POST /conversations` — store a conversation summary
- `GET /conversations` — list recent conversations (filter by channel, date)
- `POST /conversations/search` — semantic search across conversation history
- `GET /conversations/{id}` — get a specific conversation

### New MCP tools
- `store_conversation` — store summary with channel/session metadata
- `recall_conversations` — semantic search across conversation history
- `recent_conversations` — list N most recent across all channels

### Data model
New memory type `conversation` with metadata: channel, session_key, started_at, ended_at, turn_count, topics.

### Bootstrap path (no code changes — do this first)
Use existing `/remember` with `type=conversation` tag written via heartbeat. Recall on startup via `/recall`.

---

## v0.7 — Per-Turn Conversation Indexing (Issue #7)

### The problem
Summaries are lossy. Can't answer "what exactly did the user say about X?"

### Design
- Each turn stored as a separate embedding with channel + timestamp metadata
- OpenClaw writes turns to a local queue; background worker batches to magi
- magi chunks + embeds each turn
- Recall returns ranked turns by semantic similarity

### Trade-offs
More storage, more writes → dramatically better recall precision. Verbatim quote retrieval.

---

## Deployment targets

| Machine | Role | Binary |
|---|---|---|
| your-server | Server — HTTP API + MCP | magi (HTTP mode) |
| Local Agent | MCP client | magi (stdio MCP) |
| Laptop | MCP client + edge sync | magi + magi-sync |
| Future machines | MCP client + edge sync | same |

## Environment variables (full set)

| Variable | Default | Description |
|---|---|---|
| `MEMORY_BACKEND` | `sqlite` | Database backend: `sqlite`, `turso`, `postgres`, `mysql`, `sqlserver` |
| `TURSO_URL` | required | Turso DB URL (when using turso backend) |
| `TURSO_AUTH_TOKEN` | required | Turso auth token (when using turso backend) |
| `MAGI_REPLICA_PATH` | `~/.magi.db` | Local replica path |
| `MAGI_SYNC_INTERVAL` | `60` | Background sync interval (seconds) |
| `MAGI_SYNC_MAX_AGE` | `300` | Stale threshold (seconds) |
| `MAGI_MODEL_DIR` | `~/.magi/models` | ONNX model directory |
| `MAGI_HTTP_PORT` | `8300` | HTTP API port |
| `MAGI_API_TOKEN` | unset = no auth | Bearer token for HTTP API |
| `MAGI_MACHINE_ID` | hostname | Machine identity tag |
| `MAGI_AUTO_PROJECT` | `true` | Auto-detect project from git |
| `MAGI_GIT_ENABLED` | `false` | Enable git-backed memory versioning |
| `MAGI_GIT_PATH` | `~/.magi/memories` | Path to git repo for memory versioning |
| `MAGI_GIT_COMMIT_MODE` | `immediate` | Git commit mode: `immediate` or `batch` |
| `MAGI_ASYNC_WRITES` | `false` | Enable async write pipeline (returns 202) |
| `MAGI_NODE_MODE` | `embedded` | Node mesh communication mode |
| `MAGI_WRITER_POOL_SIZE` | `4` | Writer goroutine pool size |
| `MAGI_READER_POOL_SIZE` | `8` | Reader goroutine pool size |
| `MAGI_COORDINATOR_ENABLED` | `true` | Enable coordinator routing layer |

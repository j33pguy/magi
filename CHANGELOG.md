# Changelog

## v0.4.1

### Features
- **Hybrid search tuning knobs** (#144) — configurable RRF constant, fetch multiplier, and vector/BM25 weights via `MAGI_HYBRID_RRF_K`, `MAGI_HYBRID_FETCH_MULTIPLIER`, `MAGI_HYBRID_VECTOR_WEIGHT`, `MAGI_HYBRID_BM25_WEIGHT`
- **Search rewrite fallback** (#144) — optional `rewrite_fallback=1` parameter on `GET /search` re-runs the query with a deterministic rewrite when the first pass returns no results
- **Project-scoped deduplication** (#144) — dedupe now respects project boundaries; same content in different projects creates separate memories

### Fixes
- **Cross-project dedupe prevention** (#144) — memories in different projects are no longer incorrectly deduplicated against each other
- **Tag deduplication** — `normalizeTags` now removes duplicate tags before storage
- **Explicit ONNX threading** — ONNX runtime thread counts configurable via `MAGI_ONNX_INTRA_THREADS`, `MAGI_ONNX_INTER_THREADS`, `MAGI_ONNX_EXECUTION_MODE`

### Deps
- Bump `onnxruntime_go` (#142)
- Bump `actions/github-script` from 7 to 8 (#141)

## v0.4.0

### Features
- **Self-enrollment token flow** (#140) — machines can self-register with enrollment tokens
- **Procedure type + auto-type inference + agent guide** (#138) — new `procedure` memory type with automatic type inference

### Fixes
- **Enforce read-only mode, default proxy CIDR to loopback** (#139) — web UI respects read-only flag, proxy defaults to 127.0.0.0/8
- **Wire EnrollmentStore via SetEnrollmentStore** — enrollment store properly initialized at startup

### Docs
- Self-enrollment endpoints documentation
- `ghrepo` tag convention for repository-scoped memories

### Ecosystem
- **magi-sync** extracted to [j33pguy/magi-sync](https://github.com/j33pguy/magi-sync)
- **magi-ui** extracted to [j33pguy/magi-ui](https://github.com/j33pguy/magi-ui)

### CI
- Test auth isolation via `TestMain` + `autoAuthMux` pattern

## v0.3.9

### Fixes
- **FTS5 rebuild after V10 migration** — Migration V10 drops/recreates the memories table, which broke the FTS5 content-sync table and triggers. BM25 search silently returned 0 results. Now rebuilds FTS5 index and triggers after table recreation.
- **Visibility constraint** — DB now accepts `team` and `shared` visibility levels for multi-agent sync
- **Migration V10** — recreates memories table with widened CHECK constraint, handles partial failure gracefully
- **magi-sync project detection** — Claude project folder name used as project key instead of git remote URL
- **magi-sync speaker** — project context files now tagged `claude-subagent` instead of `system`
- **magi-sync include patterns** — example config fixed; patterns are relative to agent paths root

## v0.3.8

- **Release companion tools** — `mcp-config` binary included in GitHub releases (`magi-sync` moved to [j33pguy/magi-sync](https://github.com/j33pguy/magi-sync))
- Cross-compiled for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
- Pure Go binaries — download and run, zero dependencies

## v0.3.7

### Features
- **Pattern detection v2** (#124) — temporal trending, topic clustering, relationship pattern analysis
- **REST API endpoints** — `/patterns` and `/patterns/trending` for pattern queries
- **Examples** (#126) — Python client, LangChain integration, Docker quickstart compose
- **TypeScript SDK** (#118) — full REST API coverage
- **magi-sync watch mode** (#117) — real-time file watching with fsnotify
- **Settings sync** (#119) — cross-device conflict resolution for distributed setups
- **Architecture docs** (#127) — distributed architecture, git-backed memory, integration testing guides

### Security
- **Gitleaks secret scanning** (#125) — automated secret detection on all PRs and pushes
- **CI actor allowlist** — only `j33pguy` and `dependabot` can trigger CI workflows
- **SECURITY.md** — vulnerability reporting policy
- **CONTRIBUTING.md** — contributor guidelines and PR template
- **Dependabot** — automated updates for Go modules and GitHub Actions

### Fixes
- Pin `onnxruntime_go` to v1.17.0 for ORT 1.20.1 compatibility
- `TestWatchTriggersSync` race condition (#122)
- Docker build optimization (multi-stage, smaller images)
- Socket test reliability improvements
- Gitleaks workflow trigger fix (`pull_request_target` → `pull_request`)

### CI/CD
- Bumped `actions/checkout` v4→v6, `actions/setup-go` v5→v6, `docker/login-action` v3→v4
- Go dependency group update (#131)

## v0.3.6

- **gRPC task queue methods** (#114) — comprehensive test coverage for task operations
- **Version bump** — internal version constant aligned with release tags

## v0.3.5

- **Documentation refresh** — all docs current with codebase, removed stale draft docs for unimplemented features
- **Multi-stage Dockerfile** (PR #110) — smaller images, faster builds
- **Auth header spoofing fix** (PR #111) — server-set identity headers cannot be spoofed by clients
- **Dependabot security updates** — dependency patches for reported vulnerabilities
- **24 MCP tools** — updated tool count across all documentation

## v0.3.0

- **Separate Task Queue** — tasks now live outside the memory stack with explicit statuses, task events, and memory references for orchestrator/worker coordination
- **Task MCP Tools** — agents can now `create_task`, `list_tasks`, `get_task`, `update_task`, `add_task_event`, and `list_task_events`
- **Web UI Auth** — Web UI now enforces Bearer auth via `MAGI_API_TOKEN` and respects memory visibility
- **UI Toggle** — `MAGI_UI_ENABLED` enables or disables the web UI server
- **Async Writes Now Live** — The async write pipeline is fully functional with `MAGI_ASYNC_WRITES=true`
- **gRPC Graph Parity** — `LinkMemories` and `GetRelated` RPCs are now implemented
- **PostgreSQL + MySQL Factory Wiring** — Backend factory now includes PostgreSQL and MySQL
- **Unified Remember Enrichment** — classify, secret detection, dedup, and contradiction checks now run consistently across MCP, gRPC, and REST
- **External Secret Store Support** — remember flows can now externalize detected secrets into Vault-backed KV storage instead of forcing raw secrets into memory
- **New remember Service Layer** — `internal/remember` centralizes write enrichment logic
- **stdio-Only MCP Mode** — `--mcp-only` runs MCP stdio without HTTP/gRPC servers for agent subprocess integrations

## v0.2.0

- **Distributed Node Mesh** — Writer, Reader, Index, and Coordinator node types with goroutine pool routing, session affinity for read-your-writes consistency, zero overhead in embedded mode (PR #74)
- **Metrics Endpoint** — 9 metrics: write/search latency, embedding duration, queue depth, memory count, session count, cache hit/miss, git commits. Scrape `/metrics` (PR #73)
- **Health Probes** — `/readyz` and `/livez` for Kubernetes, expanded `/health` with DB status, uptime, memory count, git status (PR #73)
- **Write Tracking Helpers** — `TrackTask`, `TrackDecision`, `TrackConversation` for production dogfooding (PR #73)
- **MCP Config Generator** — `magi mcp-config` outputs ready-to-paste JSON for MCP clients (PR #73)
- **Chaos Testing** — Concurrent writes, search-during-ingestion, kill recovery, cache overflow (PR #73)
- **SQL Server Backend** — Full support for SQL Server / Azure SQL (PR #71)

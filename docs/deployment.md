# Deployment Guide

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

## Production Notice

MAGI is usable today but still evolving. Test in a staging environment, keep backups, and plan for rollback before relying on it for critical workloads.

## Build

Prereqs:

- Go 1.25+ with CGO enabled
- ONNX Runtime shared library installed
- A supported backend (SQLite, PostgreSQL, MySQL, SQL Server, or a remote sync-backed replica)

Build locally from your checkout:

```bash
CGO_ENABLED=1 make build
```

Binaries:

- `magi` (server)
- `magi-import` (markdown import helper)

## Core Configuration

These are the most commonly used environment variables. All values here are real and match `main.go` and `internal/`.

| Env Var | Default | Description |
|---------|---------|-------------|
| `MEMORY_BACKEND` | `sqlite` | Storage backend: `sqlite`, `turso`, `postgres`, `mysql`, `sqlserver` |
| `SQLITE_PATH` | `~/.magi/memory-local.db` | SQLite file path |
| `POSTGRES_URL` | none | PostgreSQL connection string |
| `MYSQL_DSN` | none | MySQL/MariaDB DSN |
| `SQLSERVER_URL` | none | SQL Server DSN (or use `SQLSERVER_HOST`/`SQLSERVER_PORT`/`SQLSERVER_DATABASE`/`SQLSERVER_USER`/`SQLSERVER_PASSWORD`) |
| `TURSO_URL` | none | Remote sync service URL (used when `MEMORY_BACKEND=turso`) |
| `TURSO_AUTH_TOKEN` | none | Auth token for remote sync service |
| `MAGI_REPLICA_PATH` | `~/.magi/memory.db` | Local replica path for sync-backed stores |
| `MAGI_SYNC_INTERVAL` | `60s` | Replica sync interval (seconds) |
| `MAGI_API_TOKEN` | empty | Admin bearer token (unset = read-only GETs only) |
| `MAGI_MACHINE_TOKENS_JSON` | empty | Bootstrap machine tokens as JSON array |
| `MAGI_MACHINE_TOKENS_FILE` | empty | Path to bootstrap machine token JSON file |
| `MAGI_GRPC_PORT` | `8300` | gRPC server port |
| `MAGI_HTTP_PORT` | `8301` | gRPC gateway (HTTP/JSON) port |
| `MAGI_LEGACY_HTTP_PORT` | `8302` | Legacy REST API port |
| `MAGI_UI_ENABLED` | `true` | Enable or disable the web UI server |
| `MAGI_UI_PORT` | `8080` | Web UI port |

## Performance And Pipeline Tuning

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_ASYNC_WRITES` | `false` | Enable async write pipeline |
| `MAGI_WRITE_WORKERS` | `NumCPU` | Async write worker count |
| `MAGI_WRITE_QUEUE_SIZE` | `1000` | Async write queue depth |
| `MAGI_BATCH_FLUSH_INTERVAL` | `100ms` | Batch flush interval |
| `MAGI_BATCH_MAX_SIZE` | `50` | Max batch size per flush |
| `MAGI_CACHE_ENABLED` | `false` | Enable hot caches |
| `MAGI_CACHE_QUERY_TTL` | `60s` | TTL for cached recall/search results |
| `MAGI_CACHE_MEMORY_SIZE` | `1000` | Max memories in hot cache |
| `MAGI_CACHE_EMBEDDING_SIZE` | `5000` | Max embeddings in hot cache |
| `MAGI_EMBED_WORKERS` | `min(NumCPU, 8)` | Embedding worker count |
| `MAGI_MODEL_DIR` | `~/.magi/models` | Directory for ONNX model files |
| `ONNXRUNTIME_LIB` | empty | Override ONNX Runtime shared library path |
| `MAGI_TURBOQUANT_ENABLED` | empty | Enable TurboQuant (if available) |
| `MAGI_TURBOQUANT_BITS` | empty | TurboQuant bit depth |
| `MAGI_HYBRID_FETCH_MULTIPLIER` | `3` | Over-fetch factor for hybrid search (topK * multiplier) |
| `MAGI_HYBRID_RRF_K` | `60.0` | Reciprocal Rank Fusion constant K |
| `MAGI_HYBRID_VECTOR_WEIGHT` | `1.0` | Vector search weight in RRF fusion |
| `MAGI_HYBRID_BM25_WEIGHT` | `1.0` | BM25 search weight in RRF fusion |
| `MAGI_ONNX_INTRA_THREADS` | `1` | ONNX intra-op thread count |
| `MAGI_ONNX_INTER_THREADS` | `1` | ONNX inter-op thread count |
| `MAGI_ONNX_EXECUTION_MODE` | `sequential` | ONNX execution mode (`parallel` or `sequential`) |

## Auth And Secrets

Auth is bearer-token based today. Set `MAGI_API_TOKEN` to enable write access and admin-only endpoints.

Machine enrollment options:

- `MAGI_MACHINE_TOKENS_JSON`
- `MAGI_MACHINE_TOKENS_FILE`
- Machine registry endpoints (see `docs/http-api.md`)

Secret handling:

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_SECRET_MODE` | `reject` | `reject` or `externalize` |
| `MAGI_SECRET_BACKEND` | empty | Secret backend identifier (current built-in value: `vault`) |
| `MAGI_REDACTED` | empty | Secret backend base URL |
| `MAGI_VAULT_TOKEN` | empty | Secret backend token |
| `MAGI_VAULT_MOUNT` | `secret` | Secret backend KV mount |
| `MAGI_VAULT_NAMESPACE` | empty | Optional namespace |

## Git-Backed History

Optional git-backed versioning uses:

- `MAGI_GIT_ENABLED`
- `MAGI_GIT_PATH`
- `MAGI_GIT_COMMIT_MODE` (`immediate` or `batch`)
- `MAGI_GIT_BATCH_INTERVAL` (seconds)

## Node Mesh (Optional)

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_NODE_MODE` | `embedded` | Communication mode |
| `MAGI_WRITER_POOL_SIZE` | `4` | Writer goroutine count |
| `MAGI_READER_POOL_SIZE` | `8` | Reader goroutine count |
| `MAGI_COORDINATOR_ENABLED` | `true` | Enable coordinator routing |

## Example Systemd Service

This example is generic and safe to adapt.

```ini
[Unit]
Description=MAGI memory server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/magi/bin/magi --http-only
Restart=on-failure
RestartSec=5

Environment=MEMORY_BACKEND=sqlite
Environment=MAGI_API_TOKEN=admin-token-example
Environment=MAGI_MODEL_DIR=/opt/magi/models
Environment=MAGI_GRPC_PORT=8300
Environment=MAGI_HTTP_PORT=8301
Environment=MAGI_LEGACY_HTTP_PORT=8302
Environment=MAGI_UI_PORT=8080

[Install]
WantedBy=multi-user.target
```

## Health Endpoints

- `GET /health`
- `GET /readyz`
- `GET /livez`

These are on the legacy HTTP API port (`MAGI_LEGACY_HTTP_PORT`).

## Deployment Guidance

- Keep MAGI private by default and expose only through a trusted network boundary.
- If you must expose it publicly, put it behind an authenticated reverse proxy and keep bearer tokens private.
- Prefer MCP tools first; REST and gRPC mirror MCP functionality.

## Model Setup

The ONNX model (`all-MiniLM-L6-v2`) auto-downloads to `MAGI_MODEL_DIR` on first run. For air-gapped environments, place the model files in that directory before starting the server.

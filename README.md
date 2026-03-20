# claude-memory

A RAG-based memory server for Claude — runs as both an MCP server (for Claude Code) and an HTTP API (for OpenClaw/other services).

Uses [Turso](https://turso.tech) (libSQL + vector search) for distributed storage with local embedded replicas, and local ONNX embeddings (all-MiniLM-L6-v2, 384-dim) — no external embedding API needed.

## Architecture

```
Claude Code (stdio MCP) ──┐
                           ├─→ claude-memory (Go) ─→ Local libSQL replica ↔ Turso cloud
OpenClaw / services (HTTP)─┘        │
                                    └─→ ONNX Runtime (local embeddings)
```

Every machine gets a local embedded replica. Reads are fast and offline-capable. Writes sync to Turso cloud, keeping all Claude instances in sync.

## MCP Tools

| Tool | Description |
|------|-------------|
| `remember` | Store a memory with auto-embedding |
| `recall` | Semantic search via vector similarity |
| `forget` | Soft-delete (archive) or hard-delete |
| `list_memories` | Browse/filter without semantic search |
| `update_memory` | Modify content/metadata, re-embeds if changed |

## MCP Resources

| Resource | Description |
|----------|-------------|
| `memory://recent` | Most recent memories |
| `memory://decisions` | Decision-type memories |
| `memory://preferences` | User preferences |

## HTTP API

Runs on port `8300` alongside the stdio MCP server.

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check (no auth) |
| `POST /recall` | Semantic search |
| `POST /remember` | Store a memory |
| `GET /memories` | List/filter memories |
| `DELETE /memories/{id}` | Archive a memory |

Auth: `Authorization: Bearer <token>` (set via `CLAUDE_MEMORY_API_TOKEN`).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TURSO_URL` | required | libSQL database URL |
| `TURSO_AUTH_TOKEN` | required | Turso auth token |
| `CLAUDE_MEMORY_REPLICA_PATH` | `~/.claude/memory.db` | Local replica path |
| `CLAUDE_MEMORY_SYNC_INTERVAL` | `60` | Sync interval (seconds) |
| `CLAUDE_MEMORY_MODEL_DIR` | `~/.claude/models` | ONNX model directory |
| `CLAUDE_MEMORY_HTTP_PORT` | `8300` | HTTP API port |
| `CLAUDE_MEMORY_API_TOKEN` | _(unset = no auth)_ | Bearer token for HTTP API |

## Build

Requires CGO (for go-libsql + onnxruntime):

```bash
# Install ONNX Runtime first
# macOS:
brew install onnxruntime
# Linux (Fedora):
dnf install onnxruntime-devel  # or install .so manually

CGO_ENABLED=1 make build
make install
```

## Claude Code Integration

```bash
claude mcp add -s user claude-memory -- claude-memory
```

## Import existing memory files

```bash
claude-memory-import --dir ~/.openclaw/workspace/memory/
```

## Deployment

Deployed to `memory01` (10.5.5.40) via Ansible. See [IaC PR #52](https://github.com/j33pguy/IaC/pull/52).

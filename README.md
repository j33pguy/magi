# claude-memory

A RAG-based memory server for Claude — runs as an MCP server (for Claude Code), a gRPC service, and an HTTP/JSON API (via grpc-gateway) for OpenClaw and other services.

Uses [Turso](https://turso.tech) (libSQL + vector search) for distributed storage with local embedded replicas, and local ONNX embeddings (all-MiniLM-L6-v2, 384-dim) — no external embedding API needed.

## Architecture

```
Claude Code (stdio MCP) ──┐
                           ├─→ claude-memory (Go) ─→ Local libSQL replica ↔ Turso cloud
OpenClaw / services ───────┘        │
  ├ gRPC    (:8300)                 └─→ ONNX Runtime (local embeddings)
  └ HTTP/JSON (:8301, grpc-gateway)
```

Every machine gets a local embedded replica. Reads are fast and offline-capable. Writes sync to Turso cloud, keeping all Claude instances in sync.

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| `:8300` | gRPC (h2) | Primary API — native gRPC clients |
| `:8301` | HTTP/JSON | grpc-gateway reverse proxy — REST-compatible |
| `:8302` | HTTP/JSON | Legacy HTTP API (will be removed once grpc-gateway is proven) |

## MCP Tools

| Tool | Description |
|------|-------------|
| `remember` | Store a memory with auto-embedding |
| `recall` | Semantic search via vector similarity |
| `forget` | Soft-delete (archive) or hard-delete |
| `list_memories` | Browse/filter without semantic search |
| `update_memory` | Modify content/metadata, re-embeds if changed |
| `store_conversation` | Store a conversation summary with auto-embedding |
| `recall_conversations` | Search conversation history (hybrid retrieval) |
| `recent_conversations` | List recent conversations across channels |

## MCP Resources

| Resource | Description |
|----------|-------------|
| `memory://recent` | Most recent memories |
| `memory://decisions` | Decision-type memories |
| `memory://preferences` | User preferences |

## gRPC / HTTP API

The gRPC service and grpc-gateway provide the same endpoints. Auth via `Authorization: Bearer <token>` metadata (gRPC) or header (HTTP).

| gRPC RPC | HTTP Endpoint | Description |
|----------|---------------|-------------|
| `Health` | `GET /health` | Health check (no auth) |
| `Remember` | `POST /remember` | Store a memory |
| `Recall` | `POST /recall` | Semantic search |
| `Forget` | `DELETE /memories/{id}` | Archive a memory |
| `List` | `GET /memories` | List/filter memories |
| `CreateConversation` | `POST /conversations` | Store a conversation summary |
| `SearchConversations` | `POST /conversations/search` | Search conversations semantically |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TURSO_URL` | required | libSQL database URL |
| `TURSO_AUTH_TOKEN` | required | Turso auth token |
| `CLAUDE_MEMORY_REPLICA_PATH` | `~/.claude/memory.db` | Local replica path |
| `CLAUDE_MEMORY_SYNC_INTERVAL` | `60` | Sync interval (seconds) |
| `CLAUDE_MEMORY_MODEL_DIR` | `~/.claude/models` | ONNX model directory |
| `CLAUDE_MEMORY_GRPC_PORT` | `8300` | gRPC server port |
| `CLAUDE_MEMORY_HTTP_PORT` | `8301` | grpc-gateway HTTP port |
| `CLAUDE_MEMORY_LEGACY_HTTP_PORT` | `8302` | Legacy HTTP API port |
| `CLAUDE_MEMORY_API_TOKEN` | _(unset = no auth)_ | Bearer token for API auth |

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

### Regenerate protobuf stubs

```bash
# Requires: buf, protoc-gen-go, protoc-gen-go-grpc, protoc-gen-grpc-gateway
make proto
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

# claude-memory

> Personal AI memory server. Semantic search, cross-channel conversation sync, and behavioral pattern learning for Claude.

## What is this?

Claude has no persistent memory across sessions or channels. Every conversation starts from scratch — context vanishes the moment a session ends. **claude-memory** fixes that.

It's a self-hosted Go server that gives Claude (and any other AI agent) a persistent, searchable memory layer. Memories are stored as vector embeddings in [Turso](https://turso.tech) (distributed libSQL), with a local embedded replica for fast offline reads. Embeddings are generated locally using ONNX Runtime (all-MiniLM-L6-v2, 384 dimensions) — no OpenAI API key, no cloud embedding service, no data leaving your machine.

claude-memory exposes four interfaces: **MCP** (for Claude Code via stdio), **gRPC** (for services), **HTTP/JSON** (via grpc-gateway), and a **web UI** (for humans). Whether Claude is talking to you in a terminal, a Discord bot, or a web chat — it remembers.

## Features

### Memory Storage
- **Semantic vector search** — hybrid retrieval (BM25 keyword + cosine similarity) fused via RRF
- **Local ONNX embeddings** — all-MiniLM-L6-v2, 384 dims, no API keys needed
- **Structured taxonomy** — area, sub_area, type, speaker, importance
- **Auto-classification** — rules-based inference of area/sub_area from content
- **Contradiction detection** — automatic checks on write, never blocks, warns
- **Deduplication** — content-hash and cosine similarity checks prevent duplicates
- **Visibility levels** — private, internal, public
- **Memory graph** — directed relationships between memories (caused_by, led_to, supersedes, etc.)
- **Soft-delete** (archive) and hard-delete

### MCP Tools (for Claude Code)

| Tool | Purpose |
|------|---------|
| `remember` | Store a memory with auto-embedding and contradiction detection |
| `recall` | Hybrid semantic + keyword search with adaptive query rewriting |
| `forget` | Soft-delete (archive) or hard-delete a memory |
| `list_memories` | Browse/filter memories without semantic search |
| `update_memory` | Modify content/metadata, re-embeds if content changes |
| `index_turn` | Index a single conversation turn (passive memory building) |
| `index_session` | Bulk-index a completed conversation session |
| `check_contradictions` | Check if content contradicts existing memories |
| `store_conversation` | Store cross-channel conversation summary |
| `recall_conversations` | Search conversation history semantically |
| `recent_conversations` | List recent conversations across all channels |
| `recall_lessons` | Search lesson memories (gotchas, hard-won knowledge) |
| `recall_incidents` | Search incident memories (what broke, how it was fixed) |
| `ingest_conversation` | Import Grok/ChatGPT/text conversation exports |
| `link_memories` | Create directed relationships between memories |
| `get_related` | Traverse the memory graph (BFS, configurable depth) |
| `unlink_memories` | Remove a relationship between memories |

### MCP Resources

| Resource | Purpose |
|----------|---------|
| `memory://context` | Auto-inject recent/important memories at session start |
| `memory://recent/{project}` | Recent memories for a project |
| `memory://decisions/{project}` | Decision-type memories for a project |
| `memory://preferences` | User preference memories |
| `memory://conversations/recent` | Recent cross-channel conversations |
| `memory://patterns` | Auto-detected behavioral patterns |

### HTTP API
Full REST API on `:8302` with Bearer token auth. See [docs/http-api.md](docs/http-api.md).

### gRPC API
Native gRPC on `:8300` with grpc-gateway JSON proxy on `:8301`. See [proto/memory/v1/memory.proto](proto/memory/v1/memory.proto).

### Web UI
Dark-theme HTMX interface on `:8080`:
- Memory list with filtering (speaker, area, type)
- Semantic search
- Memory detail view with related memories
- Create new memories
- Knowledge graph visualization (D3 force-directed)
- Conversations timeline
- Import page (drag-and-drop Grok/ChatGPT exports)
- Behavioral patterns dashboard
- Statistics (totals, speaker/area breakdowns, top tags)

### Behavioral Pattern Learning
Heuristic-based analysis that detects patterns from your memory corpus:
- **Technology preferences** — tools you consistently use or avoid
- **Decision style** — security-first, comparative, decisive
- **Work patterns** — weekend concentration, peak hours
- **Communication style** — concise, detailed, direct

## Quick Start

### Prerequisites
- Go 1.25+
- ONNX Runtime (`brew install onnxruntime` on macOS, `dnf install onnxruntime-devel` on Fedora)
- A [Turso](https://turso.tech) database (free tier works)

### Build

```bash
git clone https://github.com/j33pguy/claude-memory
cd claude-memory
CGO_ENABLED=1 make build
```

### Configure

```bash
export TURSO_URL=libsql://claude-memory-<you>.turso.io
export TURSO_AUTH_TOKEN=<token>
export CLAUDE_MEMORY_API_TOKEN=<your-bearer-token>  # optional, unset = no auth
```

See [Environment Variables](#environment-variables) for the full list.

### Run

```bash
# MCP mode (stdio) — for Claude Code
./bin/claude-memory

# HTTP-only mode — for server deployments
./bin/claude-memory --http-only
```

### Claude Code MCP Setup

```bash
claude mcp add -s user claude-memory -- claude-memory
```

Or add to your MCP config manually:

```json
{
  "mcpServers": {
    "claude-memory": {
      "command": "/usr/local/bin/claude-memory",
      "env": {
        "TURSO_URL": "libsql://claude-memory-<you>.turso.io",
        "TURSO_AUTH_TOKEN": "<token>"
      }
    }
  }
}
```

## Architecture

```
┌─────────────────┐   stdio    ┌───────────────────────────────────────────┐
│  Claude Code    │───────────▶│              claude-memory                │
└─────────────────┘            │                                           │
┌─────────────────┐   gRPC    │  ┌─────────┐  ┌────────────────────────┐  │
│  gRPC clients   │──:8300───▶│  │  Tools   │  │  ONNX Runtime          │  │
└─────────────────┘            │  │  + API   │  │  all-MiniLM-L6-v2     │  │
┌─────────────────┐   HTTP    │  │  handlers│  │  384-dim embeddings    │  │
│  Services       │──:8301───▶│  └────┬─────┘  │  (local, no API key)  │  │
│  (grpc-gateway) │            │       │        └────────────────────────┘  │
└─────────────────┘            │       ▼                                    │
┌─────────────────┐   HTTP    │  ┌─────────────────────────────────────┐   │
│  Legacy clients │──:8302───▶│  │  Turso / libSQL                     │   │
└─────────────────┘            │  │  ┌───────────────┐  ┌───────────┐  │   │
┌─────────────────┐   HTTP    │  │  │ Local replica  │◀▶│  Cloud DB │  │   │
│  Web browser    │──:8080───▶│  │  │ (fast reads)   │  │  (sync)   │  │   │
└─────────────────┘            │  │  └───────────────┘  └───────────┘  │   │
                               │  └─────────────────────────────────────┘   │
                               └───────────────────────────────────────────┘
```

See [docs/architecture.md](docs/architecture.md) for detailed data flow.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TURSO_URL` | _(required)_ | libSQL database URL |
| `TURSO_AUTH_TOKEN` | _(required)_ | Turso auth token |
| `CLAUDE_MEMORY_REPLICA_PATH` | `~/.claude/memory.db` | Local embedded replica path |
| `CLAUDE_MEMORY_SYNC_INTERVAL` | `60` | Turso sync interval (seconds) |
| `CLAUDE_MEMORY_MODEL_DIR` | `~/.claude/models` | ONNX model directory |
| `CLAUDE_MEMORY_API_TOKEN` | _(unset = no auth)_ | Bearer token for gRPC/HTTP auth |
| `CLAUDE_MEMORY_GRPC_PORT` | `8300` | gRPC server port |
| `CLAUDE_MEMORY_HTTP_PORT` | `8301` | grpc-gateway HTTP port |
| `CLAUDE_MEMORY_LEGACY_HTTP_PORT` | `8302` | Legacy HTTP API port |
| `CLAUDE_MEMORY_UI_PORT` | `8080` | Web UI port |
| `ONNXRUNTIME_LIB` | _(auto-detect)_ | Override ONNX Runtime library path |

## Importing Existing Data

### From memory markdown files

```bash
claude-memory-import --dir ~/.claude/memory/
```

### From Grok/ChatGPT exports

Use the `ingest_conversation` MCP tool, the web UI import page, or POST to `/ingest`:

```bash
curl -X POST http://localhost:8302/ingest \
  -H "Authorization: Bearer $TOKEN" \
  -d @chatgpt-export.json
```

## Deployment

See [docs/deployment.md](docs/deployment.md) for systemd unit files and reverse proxy setup.

## Development

```bash
make test      # Run tests
make fmt       # Format code
make lint      # Run linter
make proto     # Regenerate protobuf stubs (requires buf)
```

CI runs on push to `main` via GitHub Actions on a self-hosted runner, with auto-deploy to the homelab server.

## Documentation

- [MCP Tools Reference](docs/mcp-tools.md) — all 17 tools with parameters and examples
- [HTTP API Reference](docs/http-api.md) — REST endpoints with curl examples
- [Architecture](docs/architecture.md) — detailed data flow and design decisions
- [Deployment Guide](docs/deployment.md) — systemd, reverse proxy, production setup

## License

[MIT](LICENSE)

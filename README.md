# MAGI

> **Multi-Agent Graph Intelligence** — Universal memory server for AI agents.

MAGI gives your AI agents persistent, semantic memory that works across platforms, sessions, and providers. Store conversations, recall context, detect behavioral patterns, and visualize knowledge graphs — all from a single self-hosted server.

Works with **any agent**: Claude, GPT, Grok, Gemini, Cursor, Codex, local LLMs, or custom agents via MCP, gRPC, or REST.

## Features

- **Semantic Search** — Hybrid vector + BM25 retrieval with ONNX embeddings (all-MiniLM-L6-v2, 384-dim)
- **Knowledge Graph** — Auto-linked memories with D3.js force-directed visualization
- **Behavioral Patterns** — Detects preferences, work habits, decision styles across conversations
- **Cross-Channel Sync** — Unify memory across Discord, Slack, webchat, MCP, and custom channels
- **Conversation Tracking** — Store, search, and replay multi-turn conversations with topic extraction
- **Web Dashboard** — Full UI for browsing, searching, graphing, and managing memories
- **Multi-Protocol** — MCP (stdio), gRPC, REST API, and Web UI on separate ports
- **Self-Hosted** — Your data stays on your hardware. SQLite or Turso/libSQL backend.
- **Platform Agnostic** — No vendor lock-in. Standard protocols. MIT licensed.

## Quick Start

### Docker (easiest)

```bash
docker run -d --name magi \
  -p 8300:8300 -p 8301:8301 -p 8302:8302 -p 8080:8080 \
  -v magi-data:/data \
  ghcr.io/j33pguy/magi:latest
```

### Binary

```bash
# Download latest release
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-linux-amd64 -o magi
chmod +x magi

# Run with SQLite (no external deps)
MEMORY_BACKEND=sqlite ./magi --http-only
```

### From Source

```bash
git clone https://github.com/j33pguy/magi.git
cd magi
make build
MEMORY_BACKEND=sqlite ./bin/magi --http-only
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     AI Agents                           │
│  Claude · GPT · Grok · Gemini · Cursor · Local LLMs    │
├─────────┬──────────┬───────────┬────────────────────────┤
│   MCP   │   gRPC   │   REST    │        Web UI          │
│  :8301  │  :8300   │  :8302    │        :8080           │
├─────────┴──────────┴───────────┴────────────────────────┤
│                    MAGI Core                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐  │
│  │ Remember  │ │  Recall  │ │ Patterns │ │   Graph   │  │
│  │ + Embed   │ │ + Search │ │ Analyzer │ │  Linker   │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────┘  │
├─────────────────────────────────────────────────────────┤
│  ONNX Embeddings (all-MiniLM-L6-v2, 384-dim)          │
├─────────────────────────────────────────────────────────┤
│  SQLite / Turso libSQL (vector search + FTS5)          │
└─────────────────────────────────────────────────────────┘
```

## MCP Integration

MAGI exposes 20 MCP tools for direct agent integration:

```bash
# Claude Code
claude mcp add -s user magi -- ./magi

# Any MCP-compatible agent
./magi  # Runs in stdio MCP mode by default
```

**Tools:** `remember`, `recall`, `recall_conversations`, `recall_incidents`, `recall_lessons`, `store_conversation`, `index_turn`, `index_session`, `forget`, `update_memory`, `list_memories`, `link_memories`, `unlink_memories`, `check_contradictions`, `ingest`, and more.

## REST API

```bash
# Store a memory
curl -X POST http://localhost:8302/remember \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content": "User prefers Go over Python", "type": "preference", "project": "global"}'

# Semantic search
curl -X POST http://localhost:8302/recall \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"query": "language preferences", "limit": 5}'

# Store a conversation
curl -X POST http://localhost:8302/conversations \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"channel": "discord", "summary": "Discussed architecture", "topics": ["grpc", "design"]}'
```

## Web Dashboard

Access at `http://localhost:8080`:

- **List** — Browse all memories with filters (speaker, area, type)
- **Search** — Semantic search with hybrid scoring
- **Stats** — Memory distribution, top tags, activity
- **Conversations** — Timeline view grouped by date and channel
- **Patterns** — Behavioral pattern detection with related memories
- **Graph** — D3.js knowledge graph with area/type/weight filters
- **Import** — Bulk ingest from markdown, JSON, or other formats

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MEMORY_BACKEND` | `turso` | Storage backend: `turso` or `sqlite` |
| `TURSO_URL` | — | Turso database URL |
| `TURSO_AUTH_TOKEN` | — | Turso auth token |
| `MAGI_REPLICA_PATH` | `~/.magi/memory.db` | Local embedded replica path |
| `MAGI_MODEL_DIR` | `~/.magi/models` | ONNX embedding model directory |
| `MAGI_GRPC_PORT` | `8300` | gRPC server port |
| `MAGI_HTTP_PORT` | `8301` | MCP gateway port |
| `MAGI_LEGACY_HTTP_PORT` | `8302` | REST API port |
| `MAGI_UI_PORT` | `8080` | Web dashboard port |
| `MAGI_API_TOKEN` | — | API authentication token |
| `MAGI_MACHINE_ID` | `default` | Machine identifier for multi-node |

## Memory Types

| Type | Description |
|------|-------------|
| `memory` | General memory |
| `conversation` | Multi-turn conversation summary |
| `decision` | Architectural or operational decision |
| `incident` | Something that went wrong + resolution |
| `lesson` | Lesson learned from experience |
| `preference` | User/agent preference |
| `context` | Background context (identity, project info) |
| `runbook` | Operational procedure |
| `state` | Current infrastructure/project state |
| `security` | Security-related information |

## Multi-Agent Architecture

MAGI is designed for multi-agent environments:

```
  Agent A (Claude)     Agent B (GPT)     Agent C (Local LLM)
       │                    │                    │
       └────────────────────┼────────────────────┘
                            │
                      ┌─────┴─────┐
                      │   MAGI    │
                      │  Server   │
                      └─────┬─────┘
                            │
                   ┌────────┼────────┐
                   │        │        │
              Memories   Graph    Patterns
```

Each agent can:
- **Remember** — Store observations, decisions, and context
- **Recall** — Search semantically across all agents' memories
- **Link** — Create relationships between memories (caused_by, led_to, related_to)
- **Analyze** — Detect behavioral patterns across conversations

Memories are tagged with `speaker` (which agent wrote it) and `source` (which channel it came from), enabling per-agent filtering while maintaining a unified knowledge base.

## License

MIT

# claude-memory — RAG-based MCP Memory Server

Go MCP server that provides semantic memory for Claude Code using Turso (distributed libSQL with vector search) and local ONNX embeddings (all-MiniLM-L6-v2).

## Architecture

```
Claude Code (stdio) → claude-memory (Go binary)
    → Local ONNX embeddings (all-MiniLM-L6-v2, 384 dims)
    → Embedded SQLite replica (fast reads) ↔ Turso Cloud DB (sync)
```

## Project Structure

- `main.go` — Entry point, stdio MCP server
- `server/` — MCP server setup, tool/resource registration
- `db/` — Turso client, schema migrations, Memory CRUD, tags
- `tools/` — MCP tool handlers (remember, recall, forget, list, update)
- `resources/` — MCP resource handlers (recent, decisions, preferences)
- `embeddings/` — ONNX embedding provider + BERT WordPiece tokenizer
- `chunking/` — Markdown-aware text splitter
- `migrate/` — Markdown file importer
- `cmd/import/` — CLI for importing existing memory files

## Dependencies

Requires CGO for both `go-libsql` (embedded replicas) and `onnxruntime_go`:
```
CGO_ENABLED=1 go build .
```

ONNX Runtime shared library must be installed:
```
brew install onnxruntime   # macOS
```

## Environment Variables

```
TURSO_URL=libsql://claude-memory-<user>.turso.io
TURSO_AUTH_TOKEN=<token>
CLAUDE_MEMORY_REPLICA_PATH=~/.claude/memory.db
CLAUDE_MEMORY_SYNC_INTERVAL=60
CLAUDE_MEMORY_MODEL_DIR=~/.claude/models
ONNXRUNTIME_LIB=/opt/homebrew/lib/libonnxruntime.dylib  # override auto-detect
```

## Build & Install

```bash
make build    # Build to bin/
make install  # Install to /usr/local/bin
make test     # Run tests
```

## Claude Code Integration

```bash
claude mcp add -s user claude-memory -- claude-memory
```

## MCP Tools

| Tool | Purpose |
|------|---------|
| `remember` | Store a memory with auto-embedding |
| `recall` | Semantic search via vector similarity |
| `forget` | Soft-delete (archive) or hard-delete |
| `list_memories` | Browse/filter without semantic search |
| `update_memory` | Modify content/metadata, re-embeds if changed |
| `index_turn` | Index a single conversation turn as a memory |
| `index_session` | Bulk-index a completed conversation session |

## Passive Indexing

To build up memory automatically, call `index_turn` at the end of significant turns:
- After completing a task or making a decision
- After learning something new about the project or preferences
- NOT for every trivial exchange — use judgment

Or use `index_session` at session end to bulk-index a batch of turns.

## Session Start

At the beginning of every session, read the `memory://context` resource to get
recent and important memories pre-loaded. This ensures you have relevant context
without needing explicit recall calls.

## Conventions

- One struct per file, file named after the struct
- `log/slog` for structured logging
- Standard `database/sql` for DB access via go-libsql connector
- Vectors stored as `F32_BLOB(384)`, passed via `vector32()` SQL function
- Embeddings: all-MiniLM-L6-v2 via ONNX runtime (local, no API keys)

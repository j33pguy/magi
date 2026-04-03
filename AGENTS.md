# MAGI — Memory Agent for General Intelligence

## What This Is
A persistent semantic memory server for AI agents. MCP + gRPC + REST APIs.
Go codebase, SQLite/Turso/Postgres backends, local ONNX embeddings.

## Architecture
- `internal/api/` — REST API handlers
- `internal/grpc/` — gRPC server
- `internal/tools/` — MCP tool implementations (24 tools)
- `internal/db/` — Database layer (pluggable backends)
- `internal/embeddings/` — ONNX all-MiniLM-L6-v2 embeddings
- `internal/classify/` — Auto-classification engine
- `internal/contradiction/` — Contradiction detection
- `internal/patterns/` — Behavioral pattern analysis
- `internal/search/` — Semantic + FTS5 hybrid search
- `internal/remember/` — Centralized write enrichment pipeline
- `internal/web/` — Web UI
- `internal/node/` — Distributed node mesh
- `internal/vcs/` — Git-backed versioning
- `internal/auth/` — Authentication and identity
- `internal/cache/` — Hot caches for memories, embeddings, queries
- `internal/pipeline/` — Async write pipeline
- `internal/secretstore/` — Secret detection and externalization
- `internal/ingest/` — Conversation ingestion
- `internal/metrics/` — Prometheus metrics
- `internal/rewrite/` — Query rewriting
- `internal/syncagent/` — Sync agent support
- `internal/server/` — Server initialization and wiring

## Rules
1. **No personal infrastructure references.** All examples must be 100% generic.
   - No specific product names, network configs, IP addresses, hostnames
   - Use generic examples: "production server", "API service", "database cluster"
2. **All tests must pass.** Run `go test ./...` before committing.
3. **Keep it backend-agnostic.** Code should work with SQLite, Turso, Postgres, MySQL, SQL Server.
4. **MCP tools are the primary interface.** REST and gRPC mirror MCP functionality.

## Testing
```bash
go test ./...                    # All tests
go test ./internal/api/...       # Specific package
go test -race ./...              # Race detection
```

## Building
```bash
go build -o magi .
./magi serve                     # Start server
./magi mcp-config                # Generate MCP client config
```

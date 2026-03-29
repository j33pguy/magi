# Server Testing

Server initialization requires all subsystems (DB, embeddings, MCP, gRPC).
Unit tests cover configuration parsing and basic setup.

Full integration tests with `--tags=integration`:

```bash
go test --tags=integration ./internal/server/
```

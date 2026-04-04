# Contributing to MAGI

Thanks for your interest in contributing to MAGI! This guide will help you get started.

## Development Setup

### Prerequisites

- **Go 1.23+** (check `go.mod` for exact version)
- **GCC/CGO** — required for SQLite (sqlite-vec uses CGO)
- **Git** — for version control and the VCS module

### Clone and Build

```bash
git clone https://github.com/j33pguy/magi.git
cd magi
CGO_ENABLED=1 go build ./...
```

### Run Tests

```bash
# Full test suite (requires CGO)
CGO_ENABLED=1 go test ./... -count=1

# Specific package
CGO_ENABLED=1 go test ./internal/patterns/... -v

# With coverage
CGO_ENABLED=1 go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run Locally

```bash
# SQLite backend (zero config)
MEMORY_BACKEND=sqlite SQLITE_PATH=./dev.db go run .

# With auth token
MEMORY_API_TOKEN=dev-token-123 go run .
```

The server starts on:
- **:8302** — REST API (legacy HTTP)
- **:8301** — HTTP API
- **:8300** — gRPC
- **:8080** — Web UI

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused — one function, one job
- Write tests for new functionality
- Use table-driven tests where appropriate
- Error messages should be lowercase, no trailing punctuation

## Project Structure

```
internal/
├── api/          # REST API handlers
├── auth/         # Authentication middleware
├── db/           # Database layer (SQLite, Postgres, MySQL, SQL Server)
├── embeddings/   # Vector embedding providers (ONNX)
├── grpc/         # gRPC service implementation
├── patterns/     # Behavioral pattern detection
├── pipeline/     # Async write pipeline
├── search/       # Hybrid search (vector + BM25)
├── syncagent/    # magi-sync client
├── tools/        # MCP tool implementations
├── vcs/          # Git-backed memory versioning
└── web/          # Web UI server
```

## Pull Request Process

1. **Branch from `main`** — create a feature branch (`feat/`, `fix/`, `chore/`)
2. **Write tests** — new features need tests, bug fixes need regression tests
3. **Run the full suite** — `CGO_ENABLED=1 go test ./... -count=1`
4. **Keep PRs focused** — one feature or fix per PR
5. **Write a clear description** — what changed, why, and how to test it

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add temporal pattern trending
fix: MaxFileSizeKB zero-value causes silent file skip
chore: remove stale build artifacts
docs: add CONTRIBUTING.md
```

## Reporting Issues

- **Bug reports**: Include Go version, OS, backend type, and steps to reproduce
- **Feature requests**: Describe the use case, not just the solution
- **Security issues**: See [SECURITY.md](SECURITY.md) for responsible disclosure

## Architecture Decisions

- **No LLM calls in core** — pattern detection, search, and classification are all heuristic/embedding-based
- **Multi-protocol** — MCP (stdio), gRPC, REST, and Web UI all serve the same data
- **Backend-agnostic** — storage layer abstracts SQLite, PostgreSQL, MySQL, and SQL Server
- **Async writes** — the pipeline accepts writes immediately and processes in the background

## License

By contributing, you agree that your contributions will be licensed under the [Elastic License 2.0](LICENSE).

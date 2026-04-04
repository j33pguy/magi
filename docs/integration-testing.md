# Integration Testing

This guide explains how to run MAGI's storage integration tests against each backend. SQLite is the default and is the only backend exercised in CI today. Other backends are supported but less frequently tested.

## General Notes

- Tests live under `./internal/db/...`.
- Set `MEMORY_BACKEND` plus the backend-specific connection env vars.
- Most tests will run `Migrate()` internally to create the schema.
- For Docker-backed databases, start `docker-compose.test.yml` first and wait for containers to become ready.

Start the shared test databases:

```bash
docker compose -f docker-compose.test.yml up -d
```

Stop and clean up:

```bash
docker compose -f docker-compose.test.yml down -v
```

## SQLite (default, CI-backed)

SQLite uses the local libSQL/SQLite file and is the default backend in CI.

Env vars:

- `MEMORY_BACKEND=sqlite`
- `SQLITE_PATH=/tmp/magi-test.db` (or any writable path)

Example:

```bash
MEMORY_BACKEND=sqlite SQLITE_PATH=/tmp/magi-test.db go test ./internal/db/... -v
```

## PostgreSQL (pgvector)

PostgreSQL uses pgvector for embeddings and tsvector for BM25. Use the provided container image with pgvector installed.

Start the database (from `docker-compose.test.yml`):

- Host: `localhost`
- Port: `5432`
- DB: `magi`
- User: `magi`
- Password: `magi`

Env vars:

- `MEMORY_BACKEND=postgres`
- `POSTGRES_URL=postgres://magi:magi@localhost:5432/magi?sslmode=disable`

Example:

```bash
MEMORY_BACKEND=postgres POSTGRES_URL=postgres://magi:magi@localhost:5432/magi?sslmode=disable go test ./internal/db/... -v
```

## MySQL / MariaDB

MySQL stores embeddings as BLOBs and computes vector similarity in Go. BM25 is backed by FULLTEXT indexes.

Start the database (from `docker-compose.test.yml`):

- Host: `localhost`
- Port: `3306`
- DB: `magi`
- User: `magi`
- Password: `magi`

Env vars:

- `MEMORY_BACKEND=mysql`
- `MYSQL_DSN=magi:magi@tcp(localhost:3306)/magi?parseTime=true&multiStatements=true`

Example:

```bash
MEMORY_BACKEND=mysql MYSQL_DSN='magi:magi@tcp(localhost:3306)/magi?parseTime=true&multiStatements=true' go test ./internal/db/... -v
```

## SQL Server

SQL Server uses Go-side cosine reranking for vectors and SQL Server FTS for BM25.

Start the database (from `docker-compose.test.yml`):

- Host: `localhost`
- Port: `1433`
- DB: `magi`
- User: `sa`
- Password: `MagiPass123!`

Env vars (either format works):

- `MEMORY_BACKEND=sqlserver`
- `SQLSERVER_URL=sqlserver://sa:MagiPass123!@localhost:1433?database=magi`
- Or the split vars: `SQLSERVER_HOST`, `SQLSERVER_PORT`, `SQLSERVER_DATABASE`, `SQLSERVER_USER`, `SQLSERVER_PASSWORD`

Example:

```bash
MEMORY_BACKEND=sqlserver SQLSERVER_URL='sqlserver://sa:MagiPass123!@localhost:1433?database=magi' go test ./internal/db/... -v
```

## Turso (embedded replica)

Turso uses an embedded replica for local reads and syncs to the remote database. There is no Docker test container for Turso in this repo.

Env vars:

- `MEMORY_BACKEND=turso`
- `TURSO_URL=libsql://<your-db>.turso.io`
- `TURSO_AUTH_TOKEN=<token>`
- `MAGI_REPLICA_PATH=/tmp/magi-replica.db` (local replica file)
- Optional: `MAGI_SYNC_INTERVAL=60` (seconds)

Example:

```bash
MEMORY_BACKEND=turso TURSO_URL='libsql://example.turso.io' TURSO_AUTH_TOKEN='token' MAGI_REPLICA_PATH=/tmp/magi-replica.db go test ./internal/db/... -v
```

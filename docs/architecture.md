# Architecture

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

MCP tools are the primary interface. REST and gRPC mirror MCP functionality.

## Overview

MAGI optimizes for a fast single-node path first, with a clean scale-out path when one node is no longer enough.

- single-node fast path: one Go process, in-process goroutine pools, local caches, async writes, and local embeddings
- scale path: role-separated API, writer, reader, index, and embedder processes
- backend support: SQLite, PostgreSQL, MySQL, SQL Server, and optional remote sync-backed replicas

## Interfaces

MAGI runs multiple interfaces backed by the same core services:

- MCP stdio server (primary interface)
- gRPC server
- grpc-gateway HTTP/JSON proxy
- legacy HTTP API
- web UI server (optional)

## Core Services

- embeddings via ONNX (all-MiniLM-L6-v2)
- classification (area and sub-area inference)
- contradiction detection (non-blocking)
- hybrid search (BM25 + vector with RRF fusion)
- async write pipeline (optional)
- caching layer (optional)
- task queue and task event log
- memory graph links

## Data Layer

`internal/db` provides a backend-agnostic store with the same logical schema across supported databases.

Default and common choices:

- SQLite: single-node deployments
- PostgreSQL: shared or long-lived deployments
- MySQL or SQL Server: compatibility and enterprise environments
- remote sync-backed replica: optional remote replica with a local SQLite-style replica

## Write Path

The `remember` pipeline centralizes enrichment so every interface behaves the same:

- secret detection and handling
- auto-classification
- embedding generation
- deduplication and soft-group linking
- tag enrichment
- contradiction detection

## Search Path

MAGI supports two complementary query styles:

- `recall`: semantic + keyword hybrid search with optional query rewrite
- `search`: keyword-first hybrid search with optional recency weighting

## Node Mesh (Optional)

The node mesh lives in `internal/node` and routes reads and writes through worker pools.

Key environment variables:

- `MAGI_NODE_MODE`
- `MAGI_WRITER_POOL_SIZE`
- `MAGI_READER_POOL_SIZE`
- `MAGI_COORDINATOR_ENABLED`

## Web UI

The web UI is served from `internal/web` and runs on its own port (`MAGI_UI_PORT`). It provides:

- memory browsing and search
- conversation views
- import (ingest) UI
- behavioral patterns UI

The UI is optional and can be disabled with `MAGI_UI_ENABLED=false`.

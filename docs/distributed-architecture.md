# Distributed Node Mesh — Honest Architecture

MAGI ships today as a **single-node embedded** process. The node mesh is an abstraction layer that already exists in code, but it is not distributed yet. This doc describes what is implemented and what is planned.

## Current State (Single-Node Embedded)

- All node types run inside one process as goroutine pools.
- The coordinator routes requests to in-process pools, backed by a shared `db.Store`.
- There is no network transport, no node discovery, and no multi-process deployment in the current codebase.

## What Exists Today (Code Reality)

The implemented pieces live in `internal/node/` and `internal/node/local/`.

**Node interfaces and request types** (`internal/node/node.go`)

- Defines node roles: Writer, Reader, Index, Coordinator.
- Defines request/response structs for reads, writes, and indexing.

**Goroutine pools** (`internal/node/pool.go`)

- A generic worker pool with a shared work channel.
- Provides back-pressure via bounded buffering.

**Router with session metadata** (`internal/node/router.go`)

- Routes writes to the writer pool and reads to the reader pool.
- Tracks per-session write sequence numbers for future read-your-writes routing.
- In embedded mode, reads already see the latest writes because they share the same store.

**Registry** (`internal/node/registry.go`)

- In-memory registry of node capabilities (type, pool size, mode).

**Embedded implementations** (`internal/node/local/`)

- `Coordinator` creates writer and reader pools and registers capabilities.
- `Writer` wraps `db.Store` for save/update/archive/delete.
- `Reader` wraps `db.Store` for get/list/search operations.
- `Index` is a thin wrapper; indexing happens in the DB layer, and the index node only updates tags.
- `CoordinatedStore` routes most operations through the coordinator pools, while delegating some methods directly to the underlying store (e.g., graph/link ops and housekeeping).

## Planned Multi-Node Behavior (Not Implemented Yet)

These features are **planned** but do not ship today:

- **Session affinity and read-your-writes** across nodes. The router already tracks per-session write sequence metadata, but there is no distributed routing or state sharing yet.
- **Dedicated node pools** per role (Writer/Reader/Index/Coordinator) running as separate processes or services.
- **Distributed routing** so reads can target fresh replicas and writers can own partitions.

## Roadmap For Distributed Deployment (Design Intent)

The current code is structured to make these future steps possible, but they are not implemented in this branch:

- Add a network transport and serialization layer between nodes.
- Provide node discovery and capability advertisement beyond in-memory registry.
- Enforce session affinity and read-your-writes guarantees across nodes.
- Support dedicated, scalable pools per role with independent lifecycle and health checks.

Until those steps land, MAGI should be considered **single-node embedded** with an extensible node abstraction.

# Persistence and rebuild status

This is the current v0.5.0 persistence story for redesign-facing memory data.

## Git-backed today

When `MAGI_GIT_ENABLED=true` and the backend is the concrete sqlite/turso client:

- memory rows are committed to `memories/<memory-id>.json`
- outbound graph edges are committed to `links/<from-memory-id>.json`
- redesign-oriented memory context metadata is committed to `contexts/<memory-id>.json`
  - canonical repository identity
  - scope fields persisted in `memory_contexts`
  - provenance fields persisted in `memory_contexts`

These files are written on normal saves, tag updates, context saves, and link creation/deletion.

## Rebuild behavior

If the DB is empty and the git repo already has memories, startup rebuild now restores:

1. memories with preserved IDs
2. tags from memory JSON
3. `memory_contexts` and repository rows from `contexts/*.json`
4. graph edges from `links/*.json`

That means the graph remains reconstructable from git-backed state instead of depending on SQLite alone.

## Async remember parity

The async `/remember` pipeline now carries the full canonical `remember.Input`, not just the flattened legacy `db.Memory` subset.

That keeps async writes aligned with sync writes for:

- `source_file`
- repository inference inputs
- scope fields
- provenance fields
- human authored flag
- future additive remember metadata

This closes the staging gap where repo tags could be present while `memory_contexts.repository_id` or provenance fields were missing on the async path.

## What is still DB-only

A few pieces remain DB-only or partially rebuildable for v0.5.0:

- embeddings are regenerated during rebuild rather than stored in git
- repository and context rows are derived into SQL tables during rebuild rather than treated as the primary runtime store
- inbound link cleanup for deleted memories is still best-effort from the git history point of view, though rebuild skips links whose endpoints no longer exist
- richer redesign concepts like explicit multi-level subnodes, repository entities as first-class graph nodes, and provenance graphs are not implemented yet

## Recommended pre-prod validation

Before promoting to staging or production, verify:

- `go test ./internal/api ./internal/pipeline ./internal/vcs ./internal/remember`
- sync `POST /remember` with repo/scope/provenance metadata
- async `POST /remember` with the same metadata
- git repo contains matching `memories/`, `links/`, and `contexts/` files
- empty-db startup rebuild restores memories, contexts, and links

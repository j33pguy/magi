# Domain Model Scaffolding

Status: initial scaffolding
Phase: redesign support

This document captures the concrete domain-model work added to support the redesign without breaking existing clients.

## What exists now

The current memory row remains the compatibility surface.

Added alongside it:
- a redesign-oriented `remember.Envelope`
- explicit `Scope`, `Provenance`, and `RepositoryRef` structs in the remember layer
- repository normalization that handles HTTPS and git SSH forms
- schema migration scaffolding for first-class repository and memory-context tables

## New storage scaffolding

### `repositories`
A canonical repository registry intended to replace repo identity living only in tags.

Current columns:
- canonical_name
- owner
- name
- host
- default_branch
- display_name
- is_fork
- upstream_canonical_name

Purpose:
- support canonical repository identity
- leave room for rename, alias, fork, and multiple-remote handling
- give future recall and import flows a stable repo anchor

### `memory_contexts`
A sidecar table keyed by `memory_id` for redesign metadata that should not force a breaking change to the `memories` row.

Current columns:
- repository_id
- scope_owner
- scope_team
- scope_workspace
- scope_machine
- scope_agent
- scope_environment
- provenance_transport
- provenance_imported_from
- provenance_human_authored
- durable_at

Purpose:
- model scope and provenance explicitly
- avoid overloading tags for durable identity metadata
- allow gradual adoption by specific write paths

## Why this shape

This is intentionally additive.

It preserves:
- current remember, recall, and list clients
- current `memories` table reads and writes
- current repo facet behavior (`repo:owner/name`)

It enables:
- first-class repository attachment later
- provenance-aware writes later
- machine, agent, and workspace scope without stuffing everything into tags

## Recommended next steps

1. Add repository upsert and lookup helpers in `internal/db`.
2. Extend the canonical remember pipeline to populate `memory_contexts` on sync writes.
3. Add protocol fields only after the write contract is clarified.
4. Teach recall to prefer same repo, machine, agent, and workspace when the caller opts in.
5. Introduce canonical repository linking for session imports and task artifacts.

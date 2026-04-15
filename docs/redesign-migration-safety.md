# Redesign Migration Safety Notes

Status: draft
Owner: redesign migration/safety pass
Related: `docs/redesign.md`

## Goal

Prepare MAGI for the redesign without breaking existing clients, existing stored data, or current multi-backend support.

This document captures the safe-first migration posture for the redesign.

## Migration posture

Use an expand-contract strategy.

1. Expand
   - add new schema elements without deleting old ones
   - dual-write where necessary
   - keep old read paths working
   - make new semantics opt-in when possible

2. Verify
   - compare old and new representations
   - add observability around acceptance, durability, and indexing state
   - prove read compatibility before flipping defaults

3. Contract
   - switch canonical reads to the new model only after backfill and compatibility validation
   - remove legacy fields only in a later cleanup phase
   - never combine expansion and destructive cleanup in one migration wave

## Safe-first rules

- No destructive column drops during the redesign setup phase.
- No table rewrites that require full copy-and-swap just to prepare architecture.
- No transport-specific schema forks.
- No backend-specific feature rollout unless SQLite, Postgres, MySQL, and SQL Server compatibility is explicitly addressed.
- No new first-class graph or hierarchy semantics hidden only in tags.

## Recommended sequence

### Phase A, freeze current compatibility surface

Treat these as compatibility anchors:
- `memories`
- `memory_tags`
- `memory_links`
- `tasks`
- `task_events`
- current `Memory` API shape
- current `Task` and `TaskEvent` API shapes

### Phase B, additive graph-native expansion

When redesign work starts, prefer new additive structures such as:
- new node metadata columns or side tables
- provenance side tables or JSON metadata columns
- explicit durability state fields or write receipts
- richer typed edge catalogs
- repository identity tables keyed from the current `repo:owner/name` facet

During this phase:
- continue serving the current `Memory` shape
- continue accepting current link relations
- preserve `ParentID` compatibility even if richer containment lands elsewhere

### Phase C, dual-read / dual-write window

For any move from flat memory rows to richer nodes/subnodes:
- write legacy memory fields and new node structures together
- read from the legacy path by default until parity is proven
- add adapters that project the new model back into the old API shape

### Phase D, backfill and cutover

Only cut over after:
- all supported backends have equivalent additive migrations
- backfill has been tested on real databases
- read-your-writes semantics are defined for sync and async modes
- API contracts document accepted vs durable vs indexed states

## Known schema pressure points

### 1. Link relation growth

`memory_links.relation` is currently constrained to a small fixed set.

Implication:
- redesign concepts like `contains`, `derived_from`, `implements`, `blocks`, and `references` cannot be introduced casually
- adding them is a schema migration across every supported backend

Safe approach:
- centralize allowed relations in code first
- add new relation values additively in a dedicated migration wave
- avoid changing meaning of existing relation strings

### 2. Hierarchy evolution

Current containment is represented by:
- `memories.parent_id`
- `memories.chunk_index`

Implication:
- these fields are compatibility shims for future node/subnode modeling
- they should not be removed until all clients can consume the richer hierarchy model

Safe approach:
- treat `parent_id` as a long-lived compatibility field
- introduce richer containment elsewhere first
- back-project new hierarchy into `parent_id` where feasible during the dual-write window

### 3. Provenance expansion

Current provenance is partial and split across memory fields, tags, and task events.

Safe approach:
- add provenance fields or side tables first
- do not overload `source`, `source_file`, or tags with incompatible new meanings
- preserve existing fields as compatibility outputs even after richer provenance lands

### 4. Task and memory separation

This separation is healthy and should remain intact.

Safe approach:
- link tasks to memories and future nodes through references
- do not merge task state into durable memory tables for convenience
- keep task migration work independent from memory node-model migration where possible

## Immediate safe changes already aligned with this plan

- keep repository identity as an additive facet (`repo:owner/name`) before promoting it to a first-class entity
- centralize allowed link relations in code so future schema expansion has one compatibility checkpoint
- document migration phases before changing storage contracts

## Exit criteria for any destructive cleanup later

Do not drop or repurpose legacy fields until all of the following are true:
- old clients have a supported compatibility adapter or have been retired
- new writes no longer depend on legacy-only storage
- backfill completeness has been measured
- rollback strategy exists
- every supported backend has completed the same migration stage

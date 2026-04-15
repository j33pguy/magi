# MAGI redesign implementation plan

Status: initial implementation scaffold
Updated: 2026-04-15

## Immediate objective

Create the safest architectural foundations for the redesign without breaking current clients or forcing destructive schema changes.

## Phase 1 foundation now in progress

### 1. Canonical remember semantics

Goal: one semantic remember pipeline, with sync and async differing only by execution policy.

Immediate steps:
- move shared remember enrichment decisions into reusable helpers
- ensure sync and async paths derive the same default facets and repository identity
- keep async status truth conservative: accepted, processing, durable complete, failed

Follow-up work:
- route async worker logic through a shared service-level remember pipeline or shared staged helpers instead of duplicating dedup and contradiction logic inline
- define explicit write mode contract across REST and gRPC (`sync` and `async`)
- make read-your-writes guarantees honest per mode

### 2. Domain model additions to target

These should become first-class concepts in the model before they become first-class schema everywhere.

#### Scope
- project
- visibility
- owner
- team
- workspace
- machine
- agent
- environment

#### Provenance
- source
- transport
- imported_from
- machine
- agent
- authorship mode (human, agent, imported)
- durable_at

#### Repository identity
- canonical repository ref with host, owner, name, canonical slug
- immediate compatibility facet: `repo:owner/name`
- long-term first-class `Repository` node plus aliases/renames/forks

### 3. Safe schema additions to prepare

These are additive and should be done before any destructive model rewrite.

Preferred new columns on `memories`:
- `scope_owner`
- `scope_workspace`
- `scope_agent`
- `scope_machine`
- `scope_environment`
- `provenance_transport`
- `provenance_imported_from`
- `provenance_agent`
- `provenance_machine`
- `durable_at`
- `repository_host`
- `repository_owner`
- `repository_name`
- `repository_canonical`

Notes:
- keep existing `project`, `visibility`, `source`, tags, and parent/link behavior for compatibility
- backfill lazily from existing tags and known request metadata
- avoid startup migrations that rewrite or reinterpret old rows destructively

### 4. Repository-aware recall compatibility

Near term:
- derive `repo:owner/name` facet automatically where repository identity is inferable
- allow existing tag filters to use that facet immediately

Later:
- add repository-aware filtering as a first-class recall constraint
- link memories to canonical repository nodes instead of relying only on string facets

## Changes landed in this kickoff

- shared tag/facet builder for remember semantics
- repository facet inference helper with canonical `repo:owner/name` support
- explicit draft domain structs for scope, provenance, and repository identity to guide future schema work

## Remaining work

- unify async dedup and contradiction execution more fully with the remember service layer
- define and expose protocol-level write mode semantics
- add additive schema migrations across all supported backends
- propagate new scope/provenance fields through API, gRPC, syncagent, and storage
- introduce first-class repository entities and graph links

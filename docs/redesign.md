# MAGI Redesign

Status: draft
Phase: architecture and planning

## MAGI Core Principles

### 1. Shared memory for isolated agents
MAGI exists to give isolated AI agents a shared memory space that survives beyond any single session, runtime, provider, or machine.

### 2. Cross-machine continuity
Memory must follow the work across laptops, desktops, servers, containers, and remote environments without depending on local agent state or machine-specific folders.

### 3. Agent-agnostic interoperability
MAGI must serve multiple agent runtimes, including Claude, Codex, OpenClaw, local models, and future systems, without hard-coding assumptions around any one vendor's local conventions.

### 4. Multi-protocol access
The same conceptual operations should be available consistently across MCP, gRPC, REST, and any future protocol surfaces.

### 5. Graph-native memory
MAGI should treat memory as a graph of related ideas, entities, artifacts, sessions, tasks, and decisions, not as a flat pile of text records with search bolted on.

### 6. Hierarchical nodes and subnodes
MAGI should support nodes, subnodes, and deeper nested structures so that large concepts can contain smaller structured units while still linking laterally across the graph.

### 7. Rich facets and typed relationships
Tags should be rich and expressive for classification and filtering, but hierarchy and typed links must remain first-class instead of being flattened into tag hacks.

### 8. Durable, trustworthy recall semantics
MAGI must tell the truth about what is accepted, what is durable, what is indexed, and what is retrievable. Clients should not have to guess.

### 9. Tasks separate from long-term memory
Short-lived coordination and active task state should stay distinct from durable memory, while still allowing tasks to link to decisions, lessons, incidents, and artifacts.

### 10. Self-hosted and portable
MAGI should remain useful when self-hosted, portable across environments, and not dependent on a cloud provider's managed stack.

### 11. Fast local-first path, clean scale-out path
MAGI should be fast and simple on one machine first, then scale cleanly into role-separated services without changing semantics.

### 12. Memory outside provider-local silos
MAGI should store and expose memory independently of folders like `CLAUDE.md`, Codex local state, or other provider-specific storage assumptions.

## Mindset Freeze for This Phase

These rules apply during the redesign phase.

- No benchmark-driven hacks.
- No API contract patches without principle review.
- No flattening graph structure into tags because it is convenient.
- No pretending async semantics are acceptable if they weaken trust.
- No implementation-first fixes while architecture is unsettled.
- No temporary shortcuts that create permanent scar tissue.

## Current Architecture Summary

MAGI today is a multi-protocol memory service with MCP, gRPC, REST, and Web UI surfaces backed by a shared database and embedding pipeline.

Current strengths:
- broad backend support
- useful hybrid recall path
- local embedding generation
- emerging task separation
- existing graph/link concepts
- async write capability
- good self-hosted posture

Current architectural direction is good, but important pieces are not yet aligned with the stated principles.

## Confirmed Violations and Drift

### 1. Transport semantics drift
HTTP remember behavior diverges from gRPC remember behavior.

- REST remember can return `202 Accepted` when async pipeline is enabled.
- gRPC remember behaves synchronously and returns only after completion.

This violates the principle that the same conceptual operation should behave consistently across protocols.

### 2. Ambiguous durability semantics
The system currently blurs the difference between:
- request accepted
- processing started
- write durable in storage
- tags saved
- indexed and searchable

This violates trustworthy recall and write semantics.

### 3. Split semantic implementation
The sync remember path uses shared service-layer logic, but the async path reimplements key behavior separately.

That creates drift risk in:
- classification
- deduplication
- contradiction detection
- tag handling
- persistence semantics

### 4. Graph model is present but not yet first-class
MAGI has links, parent references, tags, and graph visualization, but the current core model still behaves largely like flat memory rows plus optional relationships.

This falls short of a true graph-native memory system.

### 5. Hierarchy is under-modeled
Parent-child support exists in limited form, but not yet as an explicit multi-level node and subnode model suitable for project trees, imported sessions, structured artifacts, and linked thought networks.

### 6. Tag overload risk
Tags are useful and should become richer, but the current direction risks using tags as a substitute for missing structure.

## Target Conceptual Model

### Memory object types
MAGI should move toward a richer domain model with first-class support for:
- nodes
- subnodes
- typed edges
- facets
- provenance
- scope
- artifacts
- task references
- conversation/session imports

### Nodes
A node is the canonical memory object.

Examples:
- project
- decision
- incident
- lesson
- person
- machine
- repository
- service
- session
- artifact
- concept

A node should be more than text plus tags. It should support identity, type, content, metadata, provenance, and relationships.

### Subnodes
Subnodes represent structured children of a node.

Examples:
- transcript chunks inside a session node
- sections inside a design doc node
- evidence items under an incident node
- implementation notes under a task-derived artifact node

A subnode should preserve containment while remaining linkable.

### Typed links
Links must be first-class and typed.

Examples:
- `part_of`
- `contains`
- `relates_to`
- `derived_from`
- `supersedes`
- `contradicts`
- `decision_for`
- `context_for`
- `caused_by`
- `implements`
- `blocks`
- `references`

Typed edges should carry semantic meaning, not merely indicate that two things are somehow nearby.

### Facets and tags
Tags should evolve into richer facets for filtering and discovery.

Good uses:
- agent
- machine
- runtime
- source
- project
- topic
- visibility
- ownership
- workflow state
- confidence
- import type
- git repository identity

Recommended immediate repository facet:
- `repo:owner/name`

This should allow fast filtering and auto-linking of memories to known repositories.

Longer-term direction:
- repository identity should also become a first-class modeled entity, not only a string facet
- memories, sessions, tasks, artifacts, and projects should be able to link to a canonical `Repository` node
- canonical repository modeling should handle rename, alias, fork, and multiple-remote cases more safely than string tags alone

Tags should not be used to fake:
- hierarchy
- provenance graphs
- typed relationship semantics
- canonical repository identity over the long term

### Provenance
Every node should be attributable.

Examples:
- which agent wrote it
- which machine observed it
- which session imported it
- which transport created it
- whether it was human-authored, agent-generated, or imported
- when it became durable

### Scope and visibility
Scope should become clearer than a simple visibility string.

Potential dimensions:
- owner
- team
- project
- workspace
- machine
- agent
- environment
- public/internal/private

## Write Contract Redesign

This is one of the highest-priority redesign areas.

### Problem
MAGI currently exposes inconsistent semantics around writes, especially when async mode is enabled.

Clients need a clean contract that answers:
- was the write accepted?
- was it persisted?
- was it indexed?
- when is it safe to recall?

### Design goal
One conceptual remember operation, with clearly defined execution modes.

### Recommended direction
Support explicit write modes:
- `sync`: response means durable write completed
- `async`: response means accepted for processing
- optional future `deferred/indexed` semantics if indexing becomes decoupled

The protocol surface should make this explicit instead of hiding it behind transport-specific behavior.

### Principle
Sync vs async should be an execution policy, not a semantic fork in the product model.

## Retrieval and Recall Redesign

Recall should become graph-aware, hierarchy-aware, provenance-aware, and type-aware.

### Current direction
Current hybrid retrieval is useful, but it is still mostly row retrieval with ranking.

### Target direction
Recall should be able to reason over:
- node relevance
- subnode relevance
- structural containment
- typed relationships
- provenance constraints
- time and recency
- scope and visibility
- exact-match versus contextual expansion

### Desired retrieval behaviors
- return a relevant subnode with enough parent context to make it understandable
- surface related nodes when graph context improves continuity
- distinguish canonical node from supporting evidence node
- prefer same-project, same-agent, or same-machine context when appropriate
- allow cross-project or cross-agent lateral recall when explicitly useful

## Task vs Memory Separation

The current principle here is correct and should be preserved.

### Tasks
Tasks are active coordination objects.
They represent work in motion.

### Memories
Memories are durable context objects.
They represent what should remain useful after the active coordination ends.

### Relationship
Tasks should be able to reference memories, and memories should be able to reference tasks, but they should not be collapsed into the same storage concept.

## Cross-Agent and Cross-Machine Continuity

This is a central purpose of MAGI.

MAGI should act as a shared external memory plane for:
- isolated agents
- multiple machines
- multiple runtimes
- multiple protocols
- multiple sessions over time

### Practical implications
- memory must not depend on one local provider folder
- imports from local runtime-specific stores should preserve provenance
- Claude-derived memories and Codex-derived memories should be available in one shared model
- machine-local context should be ingestible without becoming machine-trapped

## Migration and Refactor Phases

### Phase 0. Architecture freeze and design agreement
- define core principles
- identify confirmed violations
- capture open questions
- avoid premature code fixes

### Phase 1. Contract cleanup
- define canonical write semantics
- unify sync and async conceptual behavior
- document client-visible guarantees

### Phase 2. Service-layer unification
- create one canonical remember pipeline
- route transports through the same semantic core
- make async a mode of execution, not a separate implementation

### Phase 3. Graph-native data model expansion
- strengthen node model
- formalize subnodes
- expand typed edges
- separate facets from structure
- improve provenance model

### Phase 4. Retrieval redesign
- make retrieval graph-aware and hierarchy-aware
- support node and subnode level recall
- improve context expansion rules

### Phase 5. Migration strategy
- map current flat memories into richer node model
- preserve compatibility where practical
- provide upgrade paths for existing clients and stored data
- follow the expand-contract safety notes in `docs/redesign-migration-safety.md`

## Non-Goals

These things should not drive the redesign:
- chasing benchmark numbers at the expense of model integrity
- forcing everything into tags
- designing for one provider's local filesystem conventions
- introducing distributed complexity before semantics are stable
- collapsing tasks into long-term memory for convenience

## Open Questions

This section is intentionally left open during planning. It should absorb new principles and constraints as they emerge.

Questions already in play:
- what is the canonical node schema?
- how deep should subnode nesting go?
- should tags evolve into a structured facet system?
- what is the exact sync vs async contract?
- what does read-your-writes guarantee mean under graph-aware retrieval?
- what should be first-class provenance fields versus derived metadata?
- how should imported provider-local memory be normalized?

## Audit Findings Against Current Codebase

### Audit pass 1, write path and contract model

#### Finding: write semantics are transport-dependent
Current implementation violates the multi-protocol consistency principle.

Evidence:
- HTTP remember switches to async behavior when the pipeline is enabled and returns `202 Accepted`.
- gRPC remember stays synchronous and returns only after the remember service completes.

Impact:
- same conceptual operation, different durability semantics
- client behavior depends on transport instead of explicit contract
- external compatibility becomes fragile

Assessment:
- severity: high
- principle violation: multi-protocol consistency, trustworthy write semantics

#### Finding: async status and durability model are not cleanly separated
The async pipeline exposes acceptance and status tracking, but the internal notion of completion is muddy.

Evidence:
- a write receives an ID before persistence
- worker completion accounting increments before the batch inserter performs the actual `SaveMemory`
- durable persistence occurs later in the batch flush path

Impact:
- metrics and status-adjacent behavior can imply progress before durability
- future read-your-writes guarantees become hard to define precisely
- indexing and persistence truth are not modeled as separate explicit states

Assessment:
- severity: high
- principle violation: durable, trustworthy recall and write semantics

#### Finding: remember semantics are split across two implementations
The codebase currently has a semantic split between sync and async remember behavior.

Evidence:
- sync HTTP and gRPC paths use `internal/remember`
- async HTTP path uses the pipeline implementation, which separately performs embedding, classification, dedup, contradiction detection, tag construction, and persistence handoff

Impact:
- drift risk between execution modes
- fixes applied to one path may not carry to the other
- semantic guarantees become difficult to document honestly

Assessment:
- severity: high
- principle violation: one conceptual remember operation, clean service-layer semantics

### Audit pass 2, graph, hierarchy, tags, and provenance model

#### Finding: graph support exists, but graph is not yet the core storage model
Current graph support is real but secondary.

Evidence:
- the store interface includes link and traversal methods
- there is a `memory_links` table and graph traversal helpers
- graph visualization support exists via `GetGraphData`
- the primary memory object is still a mostly flat row model

Impact:
- graph behavior feels attached to memories rather than foundational to them
- retrieval and persistence still center on rows first, graph second
- the product is graph-capable, not yet graph-native

Assessment:
- severity: medium-high
- principle violation: graph-native memory

#### Finding: hierarchy is only minimally represented
Current hierarchy support is limited to `ParentID` and `ChunkIndex` on the memory row.

Evidence:
- `Memory` includes `ParentID` and `ChunkIndex`
- there is no explicit node/subnode domain layer
- no deeper structured nesting model is present in the core store interface

Impact:
- basic chunking and parent grouping are possible
- richer nested structures, document sections, evidence trees, and multi-level containment are under-modeled
- hierarchy is present as a field, not as a first-class design

Assessment:
- severity: high
- principle violation: hierarchical nodes and subnodes

#### Finding: tags are doing useful work, but they remain structurally weak
Tags currently support filtering and access metadata, but they are still simple string tags.

Evidence:
- store interface exposes `GetTags` and `SetTags`
- filtering uses tag membership checks
- access control leans on owner/viewer-style tag patterns according to code comments

Impact:
- tags are useful as facets
- tags are not typed enough to carry richer semantic or structural meaning safely
- system risks pushing too much responsibility onto tag conventions

Assessment:
- severity: medium
- principle violation: rich facets and typed relationships, if overused to replace structure

#### Finding: provenance is partial, not first-class
Current memory records include some provenance-like fields, but the model is incomplete.

Evidence:
- `Memory` contains `Source`, `SourceFile`, `Speaker`, `Project`, timestamps, and tags
- task events capture actor user, machine, and agent fields
- memory records do not yet have explicit first-class provenance fields for machine, runtime, agent identity, transport mode, import origin type, or durability state

Impact:
- some provenance can be reconstructed indirectly
- consistent cross-machine and cross-agent attribution remains under-modeled
- imported local runtime memory will be harder to normalize cleanly

Assessment:
- severity: medium-high
- principle violation: cross-machine continuity, provenance clarity

### Audit pass 3, retrieval and task separation

#### Finding: retrieval is still row-centric rather than graph-aware
Current recall behavior is useful but still primarily based on ranked flat retrieval.

Evidence:
- recall API constructs a `MemoryFilter` and calls adaptive search
- search behavior centers on query embedding, hybrid ranking, filters, and recency
- graph traversal is not part of the normal recall contract at this layer

Impact:
- current retrieval works as hybrid search over memory rows
- hierarchy, typed edges, provenance, and containment are not first-class retrieval dimensions
- graph context is not yet a core recall primitive

Assessment:
- severity: medium-high
- principle violation: graph-aware and hierarchy-aware recall

#### Finding: retrieval does not yet explicitly optimize for shrinking context windows
MAGI's stated product purpose includes helping agents survive fragile and shrinking context windows, but the recall model currently behaves like ranked search rather than context-budget curation.

Evidence:
- read path is centered on embedding, BM25, fusion, recency, and thresholding
- current API surface returns relevant rows, but does not yet expose an explicit model for context packing, compression, canonical-versus-supporting memory selection, or budget-aware recall assembly

Impact:
- MAGI can return relevant results, but it does not yet explicitly help decide what deserves scarce context budget
- agent continuity under shrinking windows remains only partially addressed

Assessment:
- severity: high for product vision
- principle violation: continuity under shrinking context windows

#### Finding: task separation is directionally correct
The task model is meaningfully separate from long-term memory and should be preserved.

Evidence:
- task handlers use a distinct task store
- task events are separate objects
- task events can reference memory IDs rather than collapsing tasks into the memory table

Impact:
- this aligns with the redesign principle
- the separation should be preserved during future refactors
- the linking surface between tasks and memory can become richer without collapsing the models

Assessment:
- severity: good current direction
- principle alignment: strong

### Audit pass 4, scale-out architecture, identity layers, and multi-tenant scope

#### Finding: scale-out direction is strong, but semantic layers are ahead of the domain model
The architecture docs already describe a clean path from one binary to multiple role-separated LXC or container services.

Evidence:
- architecture docs define `magi-api`, `magi-writer`, `magi-reader`, `magi-index`, and `magi-embedder`
- strategy docs emphasize scaling by role rather than cloning full nodes
- auth docs define separate human and machine lanes plus user, machine, and agent identities

Impact:
- scale-out thinking is already stronger than much of the current storage model
- domain scoping needs to catch up so distributed deployment does not amplify semantic ambiguity

Assessment:
- severity: medium
- principle alignment: good architecture direction, incomplete model support

#### Finding: identity layering is recognized in docs, but under-modeled in core memory records
The docs clearly anticipate layered identity such as domain, user, machine, and agent, but the current memory row model does not yet represent that cleanly.

Evidence:
- auth architecture defines `user`, `machine`, and `agent` as distinct identities
- strategy docs describe structures like `UserA.MachineA.claude-main`
- current `Memory` record mainly offers `Project`, `Source`, `SourceFile`, `Speaker`, visibility, and free-form tags

Impact:
- access control can be approximated with tags and request filters
- true multi-tenant scope, machine lineage, and runtime provenance remain under-modeled
- future domain or organization layers will be awkward if added only through tag conventions

Assessment:
- severity: high
- principle violation: rich scope model, provenance clarity, enterprise-ready continuity

#### Finding: session affinity exists for distributed reads and writes, but durability semantics still undermine the promise
The node mesh already aims to preserve read-your-writes across sessions, which is good, but this promise depends on a clean durability model.

Evidence:
- architecture docs describe router-tracked session affinity and monotonic write sequence behavior
- current async pipeline semantics still blur accepted, processed, and durable states

Impact:
- the scale-out architecture is trying to preserve trust semantics
- the write contract must be clarified before the distributed promise can be considered reliable

Assessment:
- severity: high
- principle violation: durable, trustworthy write and recall semantics

## Design Constraints Added During Review

### 1. Multi-layer identity and scope model
The redesign should account for layered scope and identity dimensions such as:
- domain or organization
- user
- machine
- agent
- project
- workspace
- environment

These should become first-class modeling concepts instead of emerging only through tag conventions.

### 2. Context-window reduction with relevance enrichment
MAGI should not only retrieve relevant memories. It should help decide which memories deserve scarce context budget.

That implies future support for:
- canonical memory selection
- supporting evidence expansion only when needed
- node-versus-subnode recall policy
- context packing and compression strategies
- recall tuned for answer quality under shrinking context windows

### 3. Repository-aware memory linking
MAGI should understand git repository identity well enough to auto-link memories to repos.

Immediate requirement:
- support repository facets such as `repo:owner/name`

Longer-term requirement:
- model repositories as first-class entities so memories can link to canonical repo identity instead of depending only on string tags
- support repo-aware recall, project rehydration, and cross-session continuity across machines and agents

## Decision Log

### 2026-04-15
- redesign is being treated as architecture-first, not benchmark-first
- graph-native memory is now considered a core principle
- hierarchy via nodes and subnodes is now considered a core principle
- tags are rich facets, not a replacement for structure
- cross-machine and cross-agent continuity remain central to the product vision
- async semantics and transport drift are confirmed redesign targets
- current code audit confirms drift in write semantics, graph primacy, hierarchy modeling, and provenance depth
- current code audit confirms task separation is one of the healthier parts of the architecture
- repository identity should be tracked immediately as a facet like `repo:owner/name` and later promoted to a first-class repository entity

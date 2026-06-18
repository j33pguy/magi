# Local Claude handoff — MAGI v0.5.0 persistence/rebuild lane

## Branch and intent

- Branch: `feat/v0.3.5`
- Goal: get the working v0.5.0 persistence/rebuild changes onto GitHub so a local Claude/workstation lane can continue review, polish, and rollout validation.
- Baseline remote before this handoff: `origin/feat/v0.3.5` at `522a85a`.

## What changed

### Already committed before this handoff

Three local commits were already ahead of `origin/feat/v0.3.5`:

1. `docs: capture redesign foundations`
2. `feat: scaffold repository-aware memory contexts`
3. `refactor: route async writes through remember pipeline`

Those commits introduced the redesign foundation, repository-aware memory context scaffolding, and async `/remember` routing through the canonical remember pipeline.

### Additional working-tree changes included in this handoff

This handoff commit carries the remaining working changes:

- Adds prepared-memory persistence plumbing so memory, tags, and memory context can be persisted coherently.
- Extends DB store interfaces and backend implementations with:
  - `SaveMemoryContext`
  - `PersistPreparedMemory`
  - hybrid/search configuration helpers
- Extends git-backed serialization to cover memory contexts and graph links.
- Extends empty-DB rebuild so git-backed state can restore:
  - memory rows with preserved IDs
  - tags
  - `memory_contexts` / repository rows from `contexts/*.json`
  - graph links from `links/*.json`
- Keeps async remember parity with sync remember for repository/scope/provenance metadata.
- Adds/updates tests across API, cache, DB, gRPC, local node store, pipeline, remember, search reranking, and VCS rebuild/serialization.
- Adds generic rollout docs/checklists for v0.5.0 persistence and rebuild validation.
- Adds `internal/buildinfo` so release/version metadata can be surfaced without hardcoding it in every runtime path.
- Ignores `.tmp-release/` so local built binaries do not hitchhike into GitHub like a parasite with a file extension.

## Verification run on Gilfoyle/Hermes

Commands run from repo root:

```bash
gofmt -w <all touched Go files>
git diff --check
go test ./...
```

Observed result:

- `git diff --check`: clean
- `go test ./...`: pass for all packages

No production/staging deployment was performed by this handoff.

## Security/privacy cleanup done before push

- Removed concrete production host/IP references from the runbook.
- Kept deployment docs generic per `AGENTS.md` rule: no personal infrastructure references in repo docs.
- Did not commit `.tmp-release/magi`.
- Did not print or store tokens/secrets in the handoff.

## Suggested next steps for local Claude

1. Pull the branch and inspect the final handoff commit:

   ```bash
   git fetch origin
   git checkout feat/v0.3.5
   git pull --ff-only
   git log --oneline --decorate -8
   git show --stat --summary HEAD
   ```

2. Re-run the same verification locally:

   ```bash
   go test ./...
   git diff --check
   ```

3. Review DB backend parity carefully:
   - SQLite/Turso concrete client is the only rebuild target right now.
   - Generic prepared persistence fallbacks must remain backend-agnostic.
   - Confirm MySQL/Postgres/SQL Server implementations compile and preserve existing behavior.

4. Review rebuild semantics:
   - Preserved IDs during rebuild are intentional.
   - Embeddings are regenerated during rebuild.
   - Links are skipped if either endpoint is missing.
   - Context rows are restored only for known memory IDs.

5. Review async remember behavior:
   - Async path should carry canonical `remember.Input`, not just flattened `db.Memory`.
   - Verify `memory_contexts.repository_id` and provenance fields are populated when async writes are enabled.

6. Before any production promotion:
   - build release binary,
   - run staging remember/search/rebuild smoke tests,
   - back up binary, DB, and git-backed memory dir,
   - validate health/search/remember/UI/listeners,
   - keep rollback binary available.

## Known caveats

- The release checklist still says the reported app version must be bumped to `0.5.0`; verify whether the buildinfo path fully satisfies that or whether CLI/API version strings need additional wiring.
- Rebuild currently requires the concrete sqlite/turso client; that is deliberate for now, but local Claude should not pretend it magically rebuilds arbitrary SQL backends from git. Magic is for stage magicians and insecure YAML.
- Runtime staging/prod checks were not executed in this session; only local compile/tests were verified.

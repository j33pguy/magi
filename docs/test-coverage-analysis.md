# Test Coverage Analysis

Generated: 2026-04-13

## Overall Summary

| Metric | Value |
|---|---|
| **Total statement coverage** | **58.5%** |
| **Test files** | 84 |
| **Test lines of code** | ~29,700 |
| **Functions at 0% coverage** | **650** |
| **Packages at 100%** | chunking, classify, metrics, rewrite, syncstate |
| **Packages below 50%** | server (4.3%), db (30.5%), grpc (45.5%), vcs (47.0%), embeddings (49.5%) |

## Packages Ranked by Coverage (Lowest First)

| Package | Coverage | Assessment |
|---|---|---|
| `server` | **4.3%** | Nearly untested |
| `db` | **30.5%** | Critical gap — largest package |
| `grpc` | **45.5%** | Below target |
| `vcs` | **47.0%** | Below target |
| `embeddings` | **49.5%** | Below target |
| `node/local` | 51.5% | Below target |
| `cache` | 57.9% | Moderate |
| `api` | 59.2% | Moderate |
| `secretstore` | 59.6% | Moderate |
| `remember` | 74.2% | Decent |
| `pipeline` | 75.1% | Decent |
| `tools` | 85.1% | Good |
| `tracking` | 87.0% | Good |
| `patterns` | 90.0% | Good |
| `resources` | 92.3% | Strong |
| `web` | 94.7% | Strong |
| `node` | 95.5% | Strong |
| `migrate` | 95.7% | Strong |
| `contradiction` | 96.5% | Strong |
| `ingest` | 97.3% | Strong |
| `project` | 97.3% | Strong |
| `polarquant` | 97.9% | Strong |
| `search` | 98.3% | Excellent |
| `auth` | 98.9% | Excellent |
| `chunking` | 100% | Complete |
| `classify` | 100% | Complete |
| `metrics` | 100% | Complete |
| `rewrite` | 100% | Complete |
| `syncstate` | 100% | Complete |

## Recommended Improvements (Priority Order)

### 1. `internal/server` — 4.3% coverage (HIGH PRIORITY)

Main orchestration package wiring together gRPC, HTTP, Web UI, and MCP servers.
650+ lines with nearly zero test coverage.

Key untested functions:
- `New()` — main server constructor with all dependency injection
- `ServeGRPC()`, `ServeGateway()`, `ServeWeb()`, `ServeHTTP()` — all listener startup
- `registerTools()` — MCP tool registration (24 tools)
- `registerResources()` — resource registration
- `Run()` — top-level run loop
- `ensureFreshSync()` — Turso sync gate logic

**Recommendation:** Add tests that construct a `Server` with mock dependencies and
verify routes/tools are registered correctly. Test lifecycle (`New` → `Close`) and
sync gate logic. Use `httptest.Server` patterns and mock stores — no real ports needed.

### 2. `internal/db` — 30.5% coverage (HIGH PRIORITY)

Largest package (~7,500 lines) and the foundation of the system.

Coverage gap breakdown:
- **Postgres backend** (`postgres.go`): 38 functions at 0% — all CRUD, search, filter, migration, link operations
- **MySQL backend** (`mysql.go`): 29 functions at 0% — full implementation untested
- **SQL Server backend** (`sqlserver.go` + `sqlserver_schema.go`): 31+ functions at 0%
- **Enrollment tokens** (`enrollment_tokens.go`): All 6 functions at 0%
- **Machine credentials** on non-SQLite backends: 15 functions at 0%
- **Task operations** on non-SQLite backends: 18 functions at 0%

SQLite has reasonable coverage since it's used by existing tests. The three other
database backends have essentially **zero** test coverage.

**Recommendations:**
- **Short-term**: Interface-level integration tests running the same suite against each
  backend via `docker-compose.test.yml`. Use build tags or `-short` to skip when
  containers aren't available.
- **Short-term**: Unit tests for `enrollment_tokens.go` against SQLite — completely
  untested even on the primary backend.
- **Medium-term**: Test hybrid search tuning functions (`hybridFetchMultiplier`,
  `hybridRRFK`, `hybridVectorWeight`, `hybridBM25Weight`) at 42.9%.

### 3. `internal/vcs` — 47.0% coverage (HIGH PRIORITY)

Two entirely untested files:

- **`store.go`** (166 lines): `VersionedStore` wrapper — every write function
  (`SaveMemory`, `UpdateMemory`, `DeleteMemory`, `ArchiveMemory`, `SetTags`,
  `CreateLink`, `DeleteLink`) at **0%**. This is the DB↔Git integration point.
- **`rebuild.go`**: `RebuildDB()` and `DBIsEmpty()` at 0% — disaster recovery path.
- **`config.go`**: `ConfigFromEnv()` at 0%.

**Recommendation:** Create temp Git repo + SQLite store, run write operations through
`VersionedStore`, verify both DB state and Git commit history. Test `RebuildDB` by
populating Git history and verifying database reconstruction.

### 4. `internal/grpc` — 45.5% coverage (MEDIUM PRIORITY)

Significant gaps:
- `handleRememberRPC` and streaming/batch operations
- All task gRPC methods (`CreateTask`, `ListTasks`, `GetTask`, `UpdateTask`,
  `CreateTaskEvent`, `ListTaskEvents`)
- History operations (`GetMemoryHistory`, `GetMemoryDiff`)

**Recommendation:** Expand `server_test.go` with table-driven tests for task and
history methods using the existing mock-based approach.

### 5. `internal/embeddings` — 49.5% coverage (MEDIUM PRIORITY)

ONNX runtime integration (`onnx.go`) entirely untested — 11 functions at 0%:
- `NewOnnxProvider`, `Embed`, `EmbedBatch`, `Dimensions`, `Destroy`
- Worker session management (`newWorkerSession`, `embedWithWorker`)

Understandable since ONNX requires native runtime, but surrounding logic is testable.

**Recommendation:** Mock ONNX C API calls or test surrounding logic independently.
At minimum test `findOnnxRuntimeLib` and `downloadIfMissing` (pure Go logic).

### 6. `internal/cache/store.go` — Mostly 0% (MEDIUM PRIORITY)

20+ delegated methods at 0%. Cache invalidation and warming logic untested:
`warmMemories`, `warmHybridResults`, `warmVectorResults`.

**Recommendation:** Test cache invalidation on writes, and warm operations populating
the cache correctly. Focus on completely untested paths: `SearchMemories`,
`FindSimilar`, `GetContextMemories`.

### 7. `internal/api` — Specific Gaps (MEDIUM PRIORITY)

Overall 59.2%, but important handlers at 0%:
- **Enrollment endpoints**: `handleCreateEnrollmentToken`, `handleListEnrollmentTokens`,
  `handleRevokeEnrollmentToken`, `handleSelfEnroll` — entire enrollment flow untested
- **History endpoints**: `handleMemoryHistory`, `handleMemoryDiff`
- **Pipeline endpoints**: `handleMemoryStatus`, `handlePipelineStats`
- **Access control**: `canAccessTags`, `mergeHybridResults`

**Recommendation:** Add HTTP handler tests for enrollment flow (create token → enroll
machine → list credentials → revoke). Security-critical path.

### 8. `internal/node/local` — 51.5% coverage (LOW PRIORITY)

17 pass-through functions in `store.go` and coordinator search paths at 0%.
Distributed mesh is still experimental.

**Recommendation:** Focus tests on coordinator write-path routing and error handling.

## Structural Recommendations

1. **Shared test harness for database backends.** A `testutil` package with
   `NewTestStore(t, backend)` would enable running the same suite across all backends
   via `docker-compose.test.yml`.

2. **Coverage tracking in CI.** Add `go test -coverprofile` to CI with a minimum
   threshold (start at 60%, ramp to 70%) to prevent regression.

3. **Prioritize security-critical paths.** Auth enrollment (`auth_enrollment.go`,
   `enrollment_tokens.go`) and access control (`access.go`) handle tokens and
   authorization but are largely untested.

4. **End-to-end write pipeline integration test.** The remember → pipeline → db → vcs
   path is the core write flow. A test exercising `Remember` → verify DB → verify Git
   commit would catch integration bugs between stages.

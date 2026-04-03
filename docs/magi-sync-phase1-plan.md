# magi-sync Phase 1 Plan

Status: Draft

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

This plan tracks Phase 1 of `magi-sync` against the current implementation in `internal/syncagent/`.

## Implemented

- push-only sync mode
- HTTP transport
- machine enrollment flow (`/auth/machines/enroll`)
- upload to `/sync/memories` with fallback to `/remember`
- config file loading and validation
- include/exclude glob rules
- secret redaction (`redact_secrets`)
- local JSON state file for dedup checkpoints
- built-in adapter (type value defined in code)
- payload types: `project_context`, `conversation_summary`, `conversation`

## Not Implemented Yet

- pull or bidirectional sync modes
- file watching (`sync.watch`)
- batch size enforcement (`sync.max_batch_size`)
- multiple adapters beyond the built-in one
- richer privacy modes beyond include/exclude

## Next Validation Steps

1. confirm enrollment with admin token
2. confirm sync uploads with machine token
3. confirm include/exclude rules match intended paths
4. confirm redaction removes secret-like patterns
5. confirm state file prevents duplicate uploads

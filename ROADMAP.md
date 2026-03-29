# magi Roadmap

## v0.1 — Current (shipped)
- MCP stdio server (Claude Code integration)
- HTTP API (OpenClaw/external services)
- Turso cloud sync with local libSQL replica
- ONNX embeddings (all-MiniLM-L6-v2, local, no API key)
- Tools: remember, recall, forget, list_memories, update_memory
- Resources: recent, decisions, preferences
- Markdown import (cmd/import)

---

## v0.2 — Project-Scoped Memory (next)

### The pattern
Download a repo → binary detects it → syncs project memories → work begins with full context.

### Features

#### Project auto-detection
- On startup, run `git remote get-url origin` in cwd
- Parse to a canonical project key: `github.com/j33pguy/IaC`
- Set as default project tag for all reads/writes in this session
- Fallback: directory name if not a git repo

#### Sync gate
- Before serving any MCP tool call, check last sync timestamp for current project
- If stale (> `MAGI_SYNC_MAX_AGE`, default 5 min): force sync, wait for completion
- Emit a log line: `syncing project memories for j33pguy/IaC...`
- Then serve the request

#### Auto-tag on write
- `remember` calls inherit current project if `project` field is empty
- Never have to manually specify project — comes from git context

#### New MCP resource: `memory://sync-status`
```json
{
  "project": "github.com/j33pguy/IaC",
  "last_sync": "2026-03-20T21:00:00Z",
  "seconds_ago": 183,
  "pending_writes": 0,
  "memory_count": 47,
  "status": "fresh"   // fresh | stale | syncing | error
}
```
Claude Code reads this before starting work. If `stale` or `syncing`, waits.

#### New MCP tool: `sync_now`
Force a sync immediately. Returns when complete.
```json
{"synced_at": "...", "records_pulled": 12, "project": "..."}
```

#### New env var: `MAGI_SYNC_MAX_AGE`
Seconds before a sync is considered stale. Default: 300 (5 min).

### Config in Claude Code (`~/.claude/CLAUDE.md` injection)
```markdown
Before starting any task:
1. Check memory://sync-status
2. If status is not "fresh", call sync_now and wait
3. Call recall with the task description to load relevant context
4. Proceed with task
```

---

## v0.2.1 — Project Context in Turso (no CLAUDE.md in repos)

### The pattern
Instead of committing `CLAUDE.md` to every project, store project instructions in Turso
tagged as `type: project_context`. Clean repos, instructions travel with the memory.

### How it works
- Special memory type: `project_context` — highest priority in recall
- Stored as: `remember("Use Go 1.21+. All errors wrapped. No globals.", type="project_context", project="j33pguy/IaC")`
- On project detection, `memory://project-context` resource returns these immediately
- Claude Code reads them before any other context

### New MCP resource: `memory://project-context`
Returns all `project_context` memories for the current project, formatted as instructions.
Claude Code injects these at the top of context automatically.

### Migration
- Existing `CLAUDE.md` files can be imported: `magi-import --type project_context --file CLAUDE.md`
- After import: delete the file, add to `.gitignore` as a safety net
- Instructions now invisible to anyone browsing the repo

### Benefits
- Repos stay clean — no AI instruction files visible
- Instructions version-controlled inside Turso, not Git
- Update instructions with `update_memory` without touching the repo
- Different instructions per branch if needed (future)

---

## v0.3 — Cross-Machine Identity

### The pattern
Multiple machines (homelab server, MacBook, future machines) all write to the same Turso DB.
Each machine has its own local replica. Writes sync up, reads pull down.

### Features

#### Machine identity tag
- Each binary instance has a `MAGI_MACHINE_ID` (e.g. `homelab`, `macbook`, `work-laptop`)
- Stored on every memory: `source_machine: homelab`
- Useful for: "what did I work on from my MacBook last week?"

#### Conflict resolution
- Turso handles this via its distributed write protocol
- Local replica is read-heavy, writes go to cloud then propagate
- No special handling needed for most cases

#### New MCP resource: `memory://machines`
Shows which machines have written recently, last seen timestamps.

---

## v0.4 — Ingestion Pipeline

### The pattern
Work offline on MacBook → Agent session logs captured → ingested into Turso → homelab Claude has full context on next session.

### Features

#### Session log watcher service
- Separate binary: `magi-watcher`
- Watches `~/.magi/projects/*/` for new session log files
- Parses JSONL session logs
- Extracts: decisions made, code written, errors hit, solutions found
- Calls `remember` API to store with project tag + `source: magi_session`

#### Import sources
- Agent session logs (`~/.magi/projects/**/*.jsonl`)
- Git commit messages (decisions + context)
- Markdown notes (manual, existing `cmd/import`)
- Future: VS Code history, terminal history

#### Watcher config (`~/.magi-watcher.yaml`)
```yaml
watch_paths:
  - ~/.magi/projects
  - ~/Projects
import_on_startup: true
sync_interval: 60
min_session_length: 5  # min turns before importing a session
```

---

## v0.5 — Cross-Channel Conversation Sync (Issue #6)

### The problem
OpenClaw sessions are isolated. MEMORY.md covers long-term facts. Nothing covers 
"what did we discuss 20 minutes ago in Discord?" when talking in webchat.

### New HTTP endpoints
- `POST /conversations` — store a conversation summary
- `GET /conversations` — list recent conversations (filter by channel, date)
- `POST /conversations/search` — semantic search across conversation history
- `GET /conversations/{id}` — get a specific conversation

### New MCP tools
- `store_conversation` — store summary with channel/session metadata
- `recall_conversations` — semantic search across conversation history
- `recent_conversations` — list N most recent across all channels

### Data model
New memory type `conversation` with metadata: channel, session_key, started_at, ended_at, turn_count, topics.

### Bootstrap path (no code changes — do this first)
Use existing `/remember` with `type=conversation` tag written via heartbeat. Recall on startup via `/recall`.

---

## v0.6 — Per-Turn Conversation Indexing (Issue #7)

### The problem
Summaries are lossy. Can't answer "what exactly did j33p say about X?"

### Design
- Each turn stored as a separate embedding with channel + timestamp metadata
- OpenClaw writes turns to a local queue; background worker batches to magi
- magi chunks + embeds each turn
- Recall returns ranked turns by semantic similarity

### Trade-offs
More storage, more writes → dramatically better recall precision. Verbatim quote retrieval.

---

## Deployment targets

| Machine | Role | Binary |
|---|---|---|
| memory01 (10.5.5.40) | Server — HTTP API + MCP | magi (HTTP mode) |
| homelab Gilfoyle | MCP client | magi (stdio MCP) |
| MacBook | MCP client + watcher | magi + magi-watcher |
| Future machines | MCP client + watcher | same |

## Environment variables (full set)

| Variable | Default | Description |
|---|---|---|
| `TURSO_URL` | required | Turso DB URL |
| `TURSO_AUTH_TOKEN` | required | Turso auth token |
| `MAGI_REPLICA_PATH` | `~/.magi.db` | Local replica path |
| `MAGI_SYNC_INTERVAL` | `60` | Background sync interval (seconds) |
| `MAGI_SYNC_MAX_AGE` | `300` | Stale threshold (seconds) |
| `MAGI_MODEL_DIR` | `~/.magi/models` | ONNX model directory |
| `MAGI_HTTP_PORT` | `8300` | HTTP API port |
| `MAGI_API_TOKEN` | unset = no auth | Bearer token for HTTP API |
| `MAGI_MACHINE_ID` | hostname | Machine identity tag |
| `MAGI_AUTO_PROJECT` | `true` | Auto-detect project from git |

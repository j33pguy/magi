# claude-memory Roadmap

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
- If stale (> `CLAUDE_MEMORY_SYNC_MAX_AGE`, default 5 min): force sync, wait for completion
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

#### New env var: `CLAUDE_MEMORY_SYNC_MAX_AGE`
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

## v0.3 — Cross-Machine Identity

### The pattern
Multiple machines (homelab server, MacBook, future machines) all write to the same Turso DB.
Each machine has its own local replica. Writes sync up, reads pull down.

### Features

#### Machine identity tag
- Each binary instance has a `CLAUDE_MEMORY_MACHINE_ID` (e.g. `homelab`, `macbook`, `work-laptop`)
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
Work offline on MacBook → Claude Code session logs captured → ingested into Turso → homelab Claude has full context on next session.

### Features

#### Session log watcher service
- Separate binary: `claude-memory-watcher`
- Watches `~/.claude/projects/*/` for new session log files
- Parses JSONL session logs
- Extracts: decisions made, code written, errors hit, solutions found
- Calls `remember` API to store with project tag + `source: claude_session`

#### Import sources
- Claude Code session logs (`~/.claude/projects/**/*.jsonl`)
- Git commit messages (decisions + context)
- Markdown notes (manual, existing `cmd/import`)
- Future: VS Code history, terminal history

#### Watcher config (`~/.claude/memory-watcher.yaml`)
```yaml
watch_paths:
  - ~/.claude/projects
  - ~/Projects
import_on_startup: true
sync_interval: 60
min_session_length: 5  # min turns before importing a session
```

---

## Deployment targets

| Machine | Role | Binary |
|---|---|---|
| memory01 (10.5.5.40) | Server — HTTP API + MCP | claude-memory (HTTP mode) |
| homelab Gilfoyle | MCP client | claude-memory (stdio MCP) |
| MacBook | MCP client + watcher | claude-memory + claude-memory-watcher |
| Future machines | MCP client + watcher | same |

## Environment variables (full set)

| Variable | Default | Description |
|---|---|---|
| `TURSO_URL` | required | Turso DB URL |
| `TURSO_AUTH_TOKEN` | required | Turso auth token |
| `CLAUDE_MEMORY_REPLICA_PATH` | `~/.claude/memory.db` | Local replica path |
| `CLAUDE_MEMORY_SYNC_INTERVAL` | `60` | Background sync interval (seconds) |
| `CLAUDE_MEMORY_SYNC_MAX_AGE` | `300` | Stale threshold (seconds) |
| `CLAUDE_MEMORY_MODEL_DIR` | `~/.claude/models` | ONNX model directory |
| `CLAUDE_MEMORY_HTTP_PORT` | `8300` | HTTP API port |
| `CLAUDE_MEMORY_API_TOKEN` | unset = no auth | Bearer token for HTTP API |
| `CLAUDE_MEMORY_MACHINE_ID` | hostname | Machine identity tag |
| `CLAUDE_MEMORY_AUTO_PROJECT` | `true` | Auto-detect project from git |

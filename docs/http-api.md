# HTTP API Reference

magi exposes two HTTP APIs:

- **grpc-gateway** on `:8301` — auto-generated JSON proxy from the gRPC service definition
- **Legacy HTTP API** on `:8302` — hand-written REST handlers (will be removed once grpc-gateway is proven)

Both use Bearer token auth via the `Authorization` header.

- `MAGI_API_TOKEN` enables the admin bearer token
- enrolled machine credentials can also authenticate through the same bearer header
- if no explicit auth is configured, MAGI stays in read-only dev mode for `GET` requests and blocks writes

For a clean public URL shape, prefer resource-style paths under `/memory` and `/task`.

Legacy routes like `/remember`, `/recall`, `/memories`, and `/tasks` are still supported for compatibility.

## Authentication

```bash
curl -H "Authorization: Bearer $MAGI_API_TOKEN" http://localhost:8302/memories
```

## Legacy HTTP API (`:8302`)

### Health Check

```
GET /health
```

No auth required. Returns expanded server status including database health, uptime, memory count, and git versioning status.

```bash
curl http://localhost:8302/health
```

```json
{
  "ok": true,
  "version": "0.3.0",
  "uptime": "2h15m30s",
  "db_status": "ok",
  "memory_count": 1523,
  "git_status": "enabled"
}
```

Returns 200 if healthy, 503 if the database is unreachable.

---

### Readiness Probe

```
GET /readyz
```

No auth required. Kubernetes-style readiness probe. Returns 200 only when the database is ready to serve queries.

```bash
curl http://localhost:8302/readyz
```

**Response (200):**
```json
{"ready": true}
```

**Response (503):**
```json
{"ready": false, "error": "database connection failed"}
```

Use this as a Kubernetes `readinessProbe` — traffic will not be routed until the database is healthy.

---

### Liveness Probe

```
GET /livez
```

No auth required. Kubernetes-style liveness probe. Returns 200 if the process is alive. No dependency checks.

```bash
curl http://localhost:8302/livez
```

**Response (200):**
```json
{"alive": true}
```

Use this as a Kubernetes `livenessProbe` — the process will be restarted if this fails.

---

Metrics endpoint (Prometheus-compatible format): `GET /metrics`

---

### Store a Memory

```
POST /remember
```

Preferred alias:

```
POST /memory
```

```bash
curl -X POST http://localhost:8302/remember \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Switched from Terraform to Ansible for infrastructure IaC",
    "project": "iac",
    "type": "decision",
    "speaker": "grok",
    "area": "infrastructure",
    "sub_area": "iac",
    "tags": ["infrastructure", "tooling"]
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | yes | Memory content |
| `summary` | string | no | One-line summary |
| `project` | string | no | Project namespace |
| `type` | string | no | Memory type |
| `visibility` | string | no | `private`, `internal`, `public` |
| `tags` | string[] | no | Tags |
| `source` | string | no | Source identifier |
| `speaker` | string | no | `user, assistant, agent, system` |
| `area` | string | no | Top-level area |
| `sub_area` | string | no | Sub-area |

**Response (201):**
```json
{"id": "a1b2c3d4e5f6...", "ok": true}
```

If tags failed to save: `{"id": "...", "ok": true, "tag_warning": "failed to set tags: ..."}`

Authenticated owner/viewer/viewer_group metadata is used later to filter recall, search, list, and conversation access.

---

### Machine Sync Write

```
POST /sync/memories
```

Preferred alias:

```
POST /memory/sync
```

Same request body as `POST /remember`, but intended for enrolled machine credentials such as `magi-sync`. Newer edge clients use this route first and fall back to `/remember` when talking to older MAGI servers.

```bash
curl -X POST http://localhost:8302/sync/memories \
  -H "Authorization: Bearer $MACHINE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "[assistant] Added machine enrollment flow",
    "summary": "Claude session summary",
    "project": "github.com/j33pguy/magi",
    "type": "conversation_summary",
    "visibility": "team",
    "tags": ["owner:UserA", "machine:laptop", "agent:claude"],
    "source": "claude-jsonl",
    "speaker": "assistant"
}'
```

---

### Task Queue

Tasks are stored separately from memories. Use them for shared work tracking between orchestrators and workers without polluting long-term recall.

#### Create Task

```
POST /tasks
```

Preferred alias: `POST /task`

```bash
curl -X POST http://localhost:8302/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "project": "github.com/j33pguy/magi",
    "queue": "default",
    "title": "Wire queue-backed tasks into MCP",
    "summary": "Keep active coordination out of the memory stack",
    "status": "queued",
    "priority": "high",
    "orchestrator": "claude-orchestrator",
    "worker": "codex-worker"
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | yes | Task title |
| `project` | string | no | Project namespace |
| `queue` | string | no | Queue name (default `default`) |
| `summary` | string | no | Short summary |
| `description` | string | no | Detailed task description |
| `status` | string | no | `queued`, `started`, `done`, `failed`, `blocked`, `canceled` |
| `priority` | string | no | `low`, `normal`, `high`, `urgent` |
| `created_by` | string | no | Creator identity |
| `orchestrator` | string | no | Orchestrator assignment |
| `worker` | string | no | Worker assignment |
| `parent_task_id` | string | no | Parent task reference |
| `metadata` | object | no | Free-form metadata |

**Response (201):** full task JSON

#### List Tasks

```
GET /tasks
```

Preferred alias: `GET /task`

Query params:

- `project`
- `queue`
- `status`
- `worker`
- `orchestrator`
- `limit`

#### Get Task

```
GET /tasks/{id}
```

Preferred alias: `GET /task/{id}`

Returns the task record as JSON.

#### Update Task

```
PATCH /tasks/{id}
```

Preferred alias: `PATCH /task/{id}`

Use this to change status, assignment, or task details. Status changes automatically create a `status` task event.

#### Add Task Event

```
POST /tasks/{id}/events
```

Preferred aliases:

- `POST /task/{id}/event`
- `POST /task/{id}/events`

Task events hold the coordination history for a task:

- `status`
- `communication`
- `issue`
- `lesson`
- `pitfall`
- `success`
- `memory_ref`
- `note`

```bash
curl -X POST http://localhost:8302/tasks/TASK_ID/events \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "communication",
    "actor_role": "worker",
    "actor_name": "codex-worker",
    "summary": "Progress update",
    "content": "Queue-backed MCP tools are wired and tested."
  }'
```

For durable findings, store a memory separately and attach it back to the task with a `memory_ref` event using `memory_id`.

#### List Task Events

```
GET /tasks/{id}/events
```

Preferred aliases:

- `GET /task/{id}/event`
- `GET /task/{id}/events`

Query params:

- `limit`

Returns the chronological event log for that task.

---

### Semantic Search

```
POST /recall
```

Preferred alias:

```
POST /memory/recall
```

```bash
curl -X POST http://localhost:8302/recall \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "DNS resolution problems",
    "area": "infrastructure",
    "top_k": 5,
    "min_relevance": 0.3
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | Natural language query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple projects |
| `type` | string | no | Filter by type |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Max results (default 5) |
| `limit` | number | no | Backwards-compatible alias for `top_k` |
| `min_relevance` | number | no | Min score 0.0–1.0 |
| `recency_decay` | number | no | Recency weighting (0.01 recommended) |
| `speaker` | string | no | Filter by speaker |
| `area` | string | no | Filter by area |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound (relative or RFC3339) |
| `before` | string | no | Time upper bound |

**Response:** Array of results with relevance scores.

---

### Keyword + Semantic Search

```
GET /search?q=<query>
```

Preferred alias:

```
GET /memory/search?q=<query>
```

```bash
curl "http://localhost:8302/search?q=reverse-proxy+config&top_k=3" \
  -H "Authorization: Bearer $TOKEN"
```

**Query Parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `q` | yes | Search query |
| `top_k` | no | Max results (default 5) |
| `recency_decay` | no | Recency weighting |
| `tags` | no | Comma-separated tag filter |
| `project` | no | Project filter |
| `type` | no | Type filter |

---

### List Memories

```
GET /memories
```

Preferred alias:

```
GET /memory
```

```bash
curl "http://localhost:8302/memories?area=infrastructure&type=decision&limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

**Query Parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 20 | Max results |
| `offset` | 0 | Pagination offset |
| `tags` | — | Comma-separated tag filter |
| `project` | — | Project filter |
| `type` | — | Type filter |
| `speaker` | — | Speaker filter |
| `area` | — | Area filter |
| `sub_area` | — | Sub-area filter |
| `after` | — | Time lower bound |
| `before` | — | Time upper bound |

---

### Delete a Memory

```
DELETE /memories/{id}
```

Preferred alias:

```
DELETE /memory/{id}
```

Archives (soft-deletes) the memory.

```bash
curl -X DELETE http://localhost:8302/memories/a1b2c3d4e5f6 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{"id": "a1b2c3d4e5f6", "ok": true}
```

Delete is authorization-aware: admins can archive any memory, and machine callers can archive only memories they own.

---

### Store a Conversation

```
POST /conversations
```

```bash
curl -X POST http://localhost:8302/conversations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "discord",
    "summary": "Discussed magi deployment strategy and cross-channel sync",
    "session_key": "abc123",
    "turn_count": 12,
    "topics": ["deployment", "cross-channel sync"],
    "decisions": ["deploy to prod-server via systemd"]
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel` | string | yes | Channel name |
| `summary` | string | yes | Conversation summary |
| `session_key` | string | no | Session identifier |
| `started_at` | string | no | Start time (RFC3339) |
| `ended_at` | string | no | End time (RFC3339) |
| `turn_count` | number | no | Turn count |
| `topics` | string[] | no | Topics discussed |
| `decisions` | string[] | no | Decisions made |
| `action_items` | string[] | no | Action items |

**Response (201):**
```json
{"id": "...", "ok": true}
```

Private conversations are automatically stamped with an owner tag from the authenticated caller so they stay scoped on later reads.

---

### List Conversations

```
GET /conversations
```

```bash
curl "http://localhost:8302/conversations?channel=discord&limit=5" \
  -H "Authorization: Bearer $TOKEN"
```

**Query Parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 10 | Max results |
| `channel` | — | Filter by channel |
| `since` | — | Only after this time (RFC3339) |

---

### Search Conversations

```
POST /conversations/search
```

```bash
curl -X POST http://localhost:8302/conversations/search \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "what did we decide about deployment",
    "limit": 5
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | Search query |
| `limit` | number | no | Max results |
| `channel` | string | no | Channel filter |
| `min_relevance` | number | no | Min score 0.0–1.0 |
| `recency_decay` | number | no | Recency weighting |

---

### Get a Conversation

```
GET /conversations/{id}
```

```bash
curl http://localhost:8302/conversations/a1b2c3d4e5f6 \
  -H "Authorization: Bearer $TOKEN"
```

---

### Import Conversations

```
POST /ingest
```

Upload a raw Grok, ChatGPT, or plaintext conversation export. Auto-detects format. Max body size: 10 MB.

```bash
curl -X POST http://localhost:8302/ingest \
  -H "Authorization: Bearer $TOKEN" \
  -d @grok-export.json
```

**Response:**
```json
{"imported": 12, "skipped": 3, "memories": ["id1", "id2", "..."]}
```

---

### Detect Import Format

```
POST /api/ingest/detect
```

Preview the detected format without importing.

```bash
curl -X POST http://localhost:8302/api/ingest/detect \
  -H "Authorization: Bearer $TOKEN" \
  -d @export.json
```

**Response:**
```json
{"format": "grok", "turns": 24}
```

---

### Analyze Behavioral Patterns

```
POST /api/analyze-patterns
```

Trigger heuristic pattern detection across the memory corpus.

```bash
curl -X POST http://localhost:8302/api/analyze-patterns \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{"patterns_found": 5, "patterns_stored": 3, "skipped_duplicates": 2}
```

---

### Memory Version History

```
GET /memories/:id/history
```

Returns the git commit history for a specific memory. Requires `MAGI_GIT_ENABLED=true`.

```bash
curl http://localhost:8302/memories/a1b2c3d4e5f6/history \
  -H "Authorization: Bearer $TOKEN"
```

**Response (200):**
```json
[
  {
    "hash": "e4f5a6b7c8d9...",
    "message": "update memory a1b2c3d4e5f6",
    "date": "2026-03-28T14:30:00Z"
  },
  {
    "hash": "b1c2d3e4f5a6...",
    "message": "create memory a1b2c3d4e5f6",
    "date": "2026-03-27T10:15:00Z"
  }
]
```

Returns 404 if the memory has no git history. Returns 501 if git versioning is not enabled.

---

### Memory Version Diff

```
GET /memories/:id/diff?from=<commit>&to=<commit>
```

Returns a unified diff between two versions of a memory. Requires `MAGI_GIT_ENABLED=true`.

```bash
curl "http://localhost:8302/memories/a1b2c3d4e5f6/diff?from=b1c2d3e4f5a6&to=e4f5a6b7c8d9" \
  -H "Authorization: Bearer $TOKEN"
```

**Query Parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `from` | yes | Source commit hash |
| `to` | yes | Target commit hash |

**Response (200):**
```json
{
  "from": "b1c2d3e4f5a6...",
  "to": "e4f5a6b7c8d9...",
  "content": "--- a/memories/a1b2c3d4e5f6.json\n+++ b/memories/a1b2c3d4e5f6.json\n@@ -2,3 +2,3 @@\n-  \"content\": \"old content\"\n+  \"content\": \"updated content\""
}
```

Returns 501 if git versioning is not enabled.

---

### Write Status

```
GET /memories/:id/status
```

Returns the current write pipeline status for a memory. Requires `MAGI_ASYNC_WRITES=true`.

```bash
curl http://localhost:8302/memories/a1b2c3d4e5f6/status \
  -H "Authorization: Bearer $TOKEN"
```

**Response (200):**
```json
{
  "state": "complete",
  "error": null,
  "started_at": "2026-03-28T14:30:00Z",
  "elapsed_ms": 42
}
```

| State | Description |
|-------|-------------|
| `pending` | Queued, not yet picked up by a worker |
| `processing` | Currently being processed (embed, classify, dedup, etc.) |
| `complete` | Successfully written to database |
| `failed` | Processing failed (see `error` field) |

Status entries are cleaned up after 5 minutes. Returns 404 if no status is tracked for the given ID. Returns 501 if async writes are not enabled.

---

### Pipeline Stats

```
GET /pipeline/stats
```

Returns aggregate statistics for the async write pipeline. Requires `MAGI_ASYNC_WRITES=true`.

```bash
curl http://localhost:8302/pipeline/stats \
  -H "Authorization: Bearer $TOKEN"
```

**Response (200):**
```json
{
  "queue_depth": 3,
  "batch_pending": 12,
  "workers": 4,
  "submitted": 1580,
  "completed": 1565,
  "failed": 2
}
```

| Field | Description |
|-------|-------------|
| `queue_depth` | Items waiting in the channel buffer |
| `batch_pending` | Items completed but not yet flushed to database |
| `workers` | Number of active worker goroutines |
| `submitted` | Total items submitted since startup |
| `completed` | Total items successfully written |
| `failed` | Total items that failed processing |

Returns 501 if async writes are not enabled.

---

### Machine Enrollment

```
POST /auth/machines/enroll
GET /auth/machines
POST /auth/machines/{id}/revoke
```

These admin-only endpoints manage machine credentials for `magi-sync` and other non-browser clients.

Example enrollment:

```bash
curl -X POST http://localhost:8302/auth/machines/enroll \
  -H "Authorization: Bearer $MAGI_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user": "UserA",
    "machine_id": "laptop-macbook",
    "agent_name": "magi-sync",
    "agent_type": "syncagent",
    "groups": ["platform"]
  }'
```

Response returns the one-time machine token and stored machine record.

---

### Secret Resolution

```
POST /auth/secrets/resolve
```

Admin-only route for resolving secret references previously externalized into the configured KV backend.

```bash
curl -X POST http://localhost:8302/auth/secrets/resolve \
  -H "Authorization: Bearer $MAGI_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "magi/my-project/1712012345-abcd1234",
    "key": "api_key"
  }'
```

---

## gRPC Gateway (`:8301`)

The grpc-gateway automatically maps gRPC RPCs to HTTP/JSON endpoints:

| gRPC RPC | HTTP Method | Path |
|----------|-------------|------|
| `Health` | GET | `/health` |
| `Remember` | POST | `/remember` |
| `Recall` | POST | `/recall` |
| `Forget` | DELETE | `/memories/{id}` |
| `List` | GET | `/memories` |
| `CreateConversation` | POST | `/conversations` |
| `SearchConversations` | POST | `/conversations/search` |
| `LinkMemories` | POST | `/links` |
| `GetRelated` | GET | `/memories/{memory_id}/related` |

Request/response formats match the protobuf definitions in `proto/memory/v1/memory.proto`. The gateway accepts standard JSON and returns JSON responses with the same field names as the proto messages.

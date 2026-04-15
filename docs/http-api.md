# HTTP API Reference

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

MCP tools are the primary interface. REST and gRPC mirror MCP functionality for integration and automation.

MAGI exposes two HTTP APIs:

- grpc-gateway on `:8301` (auto-generated JSON proxy from gRPC)
- legacy HTTP API on `:8302` (hand-written REST handlers)

Both use bearer token auth via the `Authorization` header.

- `MAGI_API_TOKEN` enables the admin bearer token
- enrolled machine credentials can authenticate using the same bearer header
- if no auth is configured, MAGI allows GETs only and blocks writes

For a clean URL shape, prefer resource-style paths under `/memory` and `/task`. Legacy routes like `/remember`, `/recall`, `/memories`, and `/tasks` remain supported.

## Authentication

```bash
curl -H "Authorization: Bearer $MAGI_API_TOKEN" http://MAGI_HTTP_ADDR:8302/memories
```

## Legacy HTTP API (`:8302`)

### Health Check

```
GET /health
```

No auth required. Returns server health details.

```bash
curl http://MAGI_HTTP_ADDR:8302/health
```

### Readiness Probe

```
GET /readyz
```

No auth required. Returns 200 only when the database is ready.

### Liveness Probe

```
GET /livez
```

No auth required. Returns 200 if the process is alive.

### Metrics

```
GET /metrics
```

Prometheus-compatible metrics.

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
curl -X POST http://MAGI_HTTP_ADDR:8302/remember \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Switched from tool A to tool B for infrastructure",
    "project": "example-project",
    "type": "decision",
    "speaker": "assistant",
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
| `tags` | string[] | no | Tags (see [tag conventions](AGENT-GUIDE.md#repository-tags): `ghrepo:owner/repo`, `inventory`, etc.) |
| `source` | string | no | Source identifier |
| `speaker` | string | no | `user`, `assistant`, `agent`, `system` |
| `area` | string | no | Top-level area |
| `sub_area` | string | no | Sub-area |

---

### Machine Sync Write

```
POST /sync/memories
```

Preferred alias:

```
POST /memory/sync
```

Same request body as `POST /remember`, but intended for enrolled machine credentials such as `magi-sync`. Clients try this route first and fall back to `/remember` when talking to older MAGI servers.

---

### Task Queue

#### Create Task

```
POST /tasks
```

Preferred alias: `POST /task`

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

#### Update Task

```
PATCH /tasks/{id}
```

Preferred alias: `PATCH /task/{id}`

#### Add Task Event

```
POST /tasks/{id}/events
```

Preferred aliases:

- `POST /task/{id}/event`
- `POST /task/{id}/events`

#### List Task Events

```
GET /tasks/{id}/events
```

Preferred aliases:

- `GET /task/{id}/event`
- `GET /task/{id}/events`

---

### Semantic Search

```
POST /recall
```

Preferred alias:

```
POST /memory/recall
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
| `limit` | number | no | Alias for `top_k` |
| `min_relevance` | number | no | Min score 0.0–1.0 |
| `recency_decay` | number | no | Recency weighting |
| `speaker` | string | no | Filter by speaker |
| `area` | string | no | Filter by area |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound (relative or RFC3339) |
| `before` | string | no | Time upper bound |

---

### Keyword + Semantic Search

```
GET /search?q=example
```

Preferred alias:

```
GET /memory/search?q=example
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
| `rewrite_fallback` | no | Set to `1` to re-run query with a deterministic rewrite when the first pass returns no results |

---

### List Memories

```
GET /memories
```

Preferred alias:

```
GET /memory
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

---

### Store a Conversation

```
POST /conversations
```

### List Conversations

```
GET /conversations
```

Query params:

- `limit`
- `channel`
- `since` (RFC3339)

### Search Conversations

```
POST /conversations/search
```

### Get a Conversation

```
GET /conversations/{id}
```

---

### Memory Version History

```
GET /memory/{id}/history
GET /memories/{id}/history
```

Requires `MAGI_GIT_ENABLED=true`.

### Memory Version Diff

```
GET /memory/{id}/diff?from=commit-a&to=commit-b
GET /memories/{id}/diff?from=commit-a&to=commit-b
```

Requires `MAGI_GIT_ENABLED=true`.

### Write Status

```
GET /memory/{id}/status
GET /memories/{id}/status
```

Requires `MAGI_ASYNC_WRITES=true`.

### Pipeline Stats

```
GET /pipeline/stats
```

Requires `MAGI_ASYNC_WRITES=true`.

---

### Machine Enrollment

```
POST /auth/machines/enroll
GET /auth/machines
POST /auth/machines/{id}/revoke
```

Admin-only endpoints that manage machine credentials.

---

### Self-Enrollment

```
POST /auth/enrollment-tokens
GET /auth/enrollment-tokens
POST /auth/enrollment-tokens/{id}/revoke
POST /auth/enroll
```

`POST /auth/enrollment-tokens` (admin-only) creates a limited-use token that machines can exchange for a permanent credential. `POST /auth/enroll` requires no Authorization header — the enrollment token is passed in the request body.

See `auth-architecture.md` for details.

---

### Behavioral Patterns

```
GET /patterns
```

Query params: `project`, `type`, `tags`, `limit`, `offset`, `speaker`, `area`, `sub_area`, `after`, `before`, `trend`, `pattern_area`, `source`, `max_patterns`.

```
GET /patterns/trending
```

Same query params as `/patterns`, plus `include_stable`. Returns patterns where `trend != stable`.

---

### Secret Resolution

```
POST /auth/secrets/resolve
```

Admin-only route for resolving secret references.

## gRPC Gateway (`:8301`)

The grpc-gateway maps gRPC RPCs to HTTP/JSON endpoints:

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

Request and response formats match the protobuf definitions in `proto/memory/v1/memory.proto`.

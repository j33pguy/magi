# HTTP API Reference

magi exposes two HTTP APIs:

- **grpc-gateway** on `:8301` — auto-generated JSON proxy from the gRPC service definition
- **Legacy HTTP API** on `:8302` — hand-written REST handlers (will be removed once grpc-gateway is proven)

Both use Bearer token auth via the `Authorization` header. Set `MAGI_API_TOKEN` to enable authentication. If unset, all requests are allowed (dev mode).

## Authentication

```bash
curl -H "Authorization: Bearer $MAGI_API_TOKEN" http://localhost:8302/memories
```

## Legacy HTTP API (`:8302`)

### Health Check

```
GET /health
```

No auth required. Returns server status.

```bash
curl http://localhost:8302/health
```

```json
{"ok": true, "version": "0.1.0"}
```

---

### Store a Memory

```
POST /remember
```

```bash
curl -X POST http://localhost:8302/remember \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Switched from Terraform to Ansible for homelab IaC",
    "project": "iac",
    "type": "decision",
    "speaker": "grok",
    "area": "homelab",
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

---

### Semantic Search

```
POST /recall
```

```bash
curl -X POST http://localhost:8302/recall \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "DNS resolution problems",
    "area": "homelab",
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

```bash
curl "http://localhost:8302/search?q=traefik+config&top_k=3" \
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

```bash
curl "http://localhost:8302/memories?area=homelab&type=decision&limit=10" \
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

Archives (soft-deletes) the memory.

```bash
curl -X DELETE http://localhost:8302/memories/a1b2c3d4e5f6 \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{"id": "a1b2c3d4e5f6", "ok": true}
```

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
    "decisions": ["deploy to magi-host via systemd"]
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

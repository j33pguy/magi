# MCP Tools Reference

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

MCP tools are the primary interface. REST and gRPC mirror MCP functionality.

## MCP Config Generator

Generate a ready-to-paste MCP configuration block:

```bash
magi mcp-config
```

Start MAGI with `MAGI_CACHE_ENABLED=true`, paste the generated config into your MCP client, and have the agent call `recall` before it starts work.

## Tool Index

- `remember`
- `recall`
- `forget`
- `list_memories`
- `update_memory`
- `create_task`
- `list_tasks`
- `get_task`
- `update_task`
- `add_task_event`
- `list_task_events`
- `index_turn`
- `index_session`
- `store_conversation`
- `recall_conversations`
- `recent_conversations`
- `recall_lessons`
- `recall_incidents`
- `ingest_conversation`
- `check_contradictions`
- `link_memories`
- `get_related`
- `unlink_memories`
- `sync_now`

---

## Core Memory Operations

### remember

Store a memory with automatic semantic embedding, deduplication, and contradiction detection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | The content to remember |
| `project` | string | yes* | Project/namespace name (required unless the server detected a default project) |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security`, `state`, `procedure` |
| `summary` | string | no | Brief one-line summary |
| `tags` | string[] | no | Tags for categorization |
| `dedup_threshold` | number | no | Similarity threshold for deduplication (0.0–1.0, default 0.95) |
| `speaker` | string | no | `user`, `assistant`, `agent`, `system` (default `assistant`) |
| `area` | string | no | `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Sub-domain within area |

### recall

Hybrid semantic + keyword search with adaptive query rewriting.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Natural language search query |
| `project` | string | no | Filter by a single project/namespace |
| `projects` | string[] | no | Filter by multiple namespaces (any match) |
| `type` | string | no | Memory type filter (same values as `remember`) |
| `tags` | string[] | no | Filter by tags (any match) |
| `top_k` | number | no | Number of results (default 5) |
| `min_relevance` | number | no | Minimum relevance score 0.0–1.0 (default 0.0) |
| `recency_decay` | number | no | Exponential decay rate (0.0 disabled, recommended 0.01) |
| `speaker` | string | no | `user`, `assistant`, `agent`, `system` |
| `area` | string | no | `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (RFC3339) |
| `before` | string | no | Time upper bound |

### forget

Remove a memory. Soft-deletes (archives) by default.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Memory ID |
| `permanent` | boolean | no | Hard-delete instead of archive (default false) |

### list_memories

Browse and filter memories without semantic search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Filter by project |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security`, `state`, `procedure` |
| `tags` | string[] | no | Filter by tags |
| `limit` | number | no | Max results (default 20) |
| `offset` | number | no | Pagination offset (default 0) |
| `speaker` | string | no | `user`, `assistant`, `agent`, `system` |
| `area` | string | no | `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound (relative or RFC3339) |
| `before` | string | no | Time upper bound |

### update_memory

Update an existing memory. Re-embeds automatically if content changes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Memory ID |
| `content` | string | no | New content (triggers re-embedding) |
| `summary` | string | no | New summary |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security`, `state`, `procedure` |
| `tags` | string[] | no | Replace all tags with these |

---

## Task Queue

### create_task

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Short task title |
| `project` | string | no | Project/namespace |
| `queue` | string | no | Queue name (default `default`) |
| `summary` | string | no | Short summary |
| `description` | string | no | Detailed description |
| `status` | string | no | `queued`, `started`, `done`, `failed`, `blocked`, `canceled` |
| `priority` | string | no | `low`, `normal`, `high`, `urgent` |
| `created_by` | string | no | Creator identity |
| `orchestrator` | string | no | Orchestrator assignment |
| `worker` | string | no | Worker assignment |
| `parent_task_id` | string | no | Parent task |
| `actor_role` | string | no | Initial event actor role |
| `actor_name` | string | no | Initial event actor name |
| `actor_agent` | string | no | Initial event agent name |
| `metadata_json` | string | no | JSON object with metadata |

### list_tasks

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Filter by project |
| `queue` | string | no | Filter by queue |
| `status` | string | no | Filter by task status |
| `worker` | string | no | Filter by worker |
| `orchestrator` | string | no | Filter by orchestrator |
| `limit` | number | no | Max results (default 25) |

### get_task

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Task ID |

### update_task

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Task ID |
| `project` | string | no | Project |
| `queue` | string | no | Queue |
| `title` | string | no | Title |
| `summary` | string | no | Summary |
| `description` | string | no | Description |
| `status` | string | no | `queued`, `started`, `done`, `failed`, `blocked`, `canceled` |
| `priority` | string | no | `low`, `normal`, `high`, `urgent` |
| `created_by` | string | no | Creator |
| `orchestrator` | string | no | Orchestrator |
| `worker` | string | no | Worker |
| `parent_task_id` | string | no | Parent task |
| `status_summary` | string | no | Summary for the generated status event |
| `metadata_json` | string | no | JSON object with metadata |

### add_task_event

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID |
| `event_type` | string | yes | `status`, `communication`, `issue`, `lesson`, `pitfall`, `success`, `memory_ref`, `note` |
| `actor_role` | string | no | Actor role |
| `actor_name` | string | no | Actor display name |
| `actor_user` | string | no | Actor user |
| `actor_machine` | string | no | Actor machine |
| `actor_agent` | string | no | Actor agent |
| `summary` | string | no | Short summary |
| `content` | string | no | Detailed content |
| `status` | string | no | Required for `status` events |
| `memory_id` | string | no | Linked memory ID |
| `source` | string | no | Source system |
| `metadata_json` | string | no | JSON object with metadata |

### list_task_events

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID |
| `limit` | number | no | Max results (default 100) |

---

## Conversation Indexing

### index_turn

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `role` | string | yes | `user` or `assistant` |
| `content` | string | yes | Message content |
| `project` | string | no | Project name/path (auto-detected if omitted) |
| `session_id` | string | no | Session identifier |

### index_session

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `turns` | object[] | yes | Array of `{role, content}` objects |
| `project` | string | no | Project name/path |
| `session_id` | string | no | Session identifier |
| `summarize` | boolean | no | Also store a rolled-up summary memory (default false) |

### store_conversation

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | yes | Channel name (example: `mcp`, `web`, `cli`) |
| `summary` | string | yes | Conversation summary |
| `session_key` | string | no | Unique session identifier |
| `started_at` | string | no | Start time (RFC3339) |
| `ended_at` | string | no | End time (RFC3339) |
| `turn_count` | number | no | Number of turns |
| `topics` | string[] | no | Topics discussed |
| `decisions` | string[] | no | Decisions made |
| `action_items` | string[] | no | Action items |

### recall_conversations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `channel` | string | no | Filter by channel |
| `top_k` | number | no | Number of results (default 5) |
| `min_relevance` | number | no | Minimum relevance 0.0–1.0 (default 0.0) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled) |

### recent_conversations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | no | Filter by channel |
| `since` | string | no | Only after this time (RFC3339) |
| `limit` | number | no | Max results (default 10) |

---

## Specialized Search

### recall_lessons

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled) |

### recall_incidents

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled) |

---

## Ingestion

### ingest_conversation

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Raw conversation text or JSON export |
| `source` | string | no | Format identifier (supported values are defined in code) |
| `project` | string | no | Associate memories with this project |
| `dry_run` | boolean | no | Preview import without storing (default false) |

---

## Contradiction Detection

### check_contradictions

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Text to check |
| `area` | string | no | Filter to this area |
| `sub_area` | string | no | Filter to this sub-area |
| `threshold` | number | no | Similarity threshold 0.0–1.0 (default 0.85) |

---

## Memory Graph

### link_memories

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `from_id` | string | yes | Source memory ID |
| `to_id` | string | yes | Target memory ID |
| `relation` | string | yes | One of: `caused_by`, `led_to`, `related_to`, `supersedes`, `part_of`, `contradicts` |
| `weight` | number | no | Relationship strength 0.0–1.0 (default 1.0) |

### get_related

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `memory_id` | string | yes | Starting memory ID |
| `depth` | number | no | Hops to traverse (default 1) |
| `direction` | string | no | `from`, `to`, or `both` (default `both`) |

### unlink_memories

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `link_id` | string | yes | Link ID to remove |

---

## Sync

### sync_now

Force a manual sync for backends that support it.

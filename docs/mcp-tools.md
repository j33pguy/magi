# MCP Tools Reference

magi exposes MCP tools via stdio for use with any MCP-compatible agent. All tools accept JSON parameters and return JSON responses.

## MCP Config Generator

Generate a ready-to-paste MCP configuration block for any MCP-compatible client:

```bash
magi mcp-config
```

For the easiest onboarding path, start MAGI with `MAGI_CACHE_ENABLED=true`, paste this config into Claude Code or Codex, and let the agent use `recall` before it starts work on a project.

**Output:**
```json
{
  "mcpServers": {
    "magi": {
      "command": "magi",
      "args": ["--mcp-only"],
      "env": {
        "MEMORY_BACKEND": "${MEMORY_BACKEND}",
        "SQLITE_PATH": "${SQLITE_PATH}",
        "POSTGRES_URL": "${POSTGRES_URL}",
        "MYSQL_DSN": "${MYSQL_DSN}",
        "SQLSERVER_URL": "${SQLSERVER_URL}",
        "TURSO_URL": "${TURSO_URL}",
        "TURSO_AUTH_TOKEN": "${TURSO_AUTH_TOKEN}",
        "MAGI_REPLICA_PATH": "${MAGI_REPLICA_PATH}",
        "MAGI_API_TOKEN": "${MAGI_API_TOKEN}",
        "MAGI_ASYNC_WRITES": "true",
        "MAGI_CACHE_ENABLED": "true",
        "MAGI_UI_ENABLED": "false"
      }
    }
  }
}
```

Copy the output into your agent's MCP configuration file. Replace only the backend and token placeholders you actually use.

---

## Core Memory Operations

### remember

Store a memory with automatic semantic embedding, deduplication, and contradiction detection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | The content to remember |
| `project` | string | yes | Project/namespace name |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security`, `state` |
| `summary` | string | no | Brief one-line summary |
| `tags` | string[] | no | Tags for categorization |
| `dedup_threshold` | number | no | Similarity threshold for deduplication (0.0–1.0, default 0.95) |
| `speaker` | string | no | Who said/wrote this: `user`, `assistant`, `agent`, `system` (default `assistant`) |
| `area` | string | no | Top-level domain: `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Sub-domain within area |

### recall

Hybrid semantic + keyword search with adaptive query rewriting (BM25 + vector, fused via RRF).

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
| `speaker` | string | no | Filter by speaker: `user`, `assistant`, `agent`, `system` |
| `area` | string | no | Filter by area: `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (`2006-01-02`, RFC3339) |
| `before` | string | no | Time upper bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (`2006-01-02`, RFC3339) |

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
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security` |
| `tags` | string[] | no | Filter by tags |
| `limit` | number | no | Max results (default 20) |
| `offset` | number | no | Pagination offset (default 0) |
| `speaker` | string | no | Filter by speaker: `user`, `assistant`, `agent`, `system` |
| `area` | string | no | Filter by area: `work`, `infrastructure`, `development`, `personal`, `project`, `meta` |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (`2006-01-02`, RFC3339) |
| `before` | string | no | Time upper bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (`2006-01-02`, RFC3339) |

### update_memory

Update an existing memory. Re-embeds automatically if content changes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Memory ID |
| `content` | string | no | New content (triggers re-embedding) |
| `summary` | string | no | New summary |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security` |
| `tags` | string[] | no | Replace all tags with these |

---

## Task Queue

Tasks are separate from memories. Use them for active coordination and shared progress between orchestrators and workers.

### create_task

Create a task in the shared task queue.

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

List tasks so agents can see each other’s progress.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Filter by project |
| `queue` | string | no | Filter by queue |
| `status` | string | no | Filter by task status |
| `worker` | string | no | Filter by worker |
| `orchestrator` | string | no | Filter by orchestrator |
| `limit` | number | no | Max results (default 25) |

### get_task

Fetch a single task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Task ID |

### update_task

Update task status, assignment, or details.

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

Append comms, issues, lessons, pitfalls, successes, or memory references to a task.

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
| `content` | string | no | Detailed content or communication text |
| `status` | string | no | Required for `status` events |
| `memory_id` | string | no | Linked memory ID |
| `source` | string | no | Source system |
| `metadata_json` | string | no | JSON object with metadata |

### list_task_events

List the activity log for a task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID |
| `limit` | number | no | Max results (default 100) |

---

## Conversation Indexing

### index_turn

Index a single conversation turn as a memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `role` | string | yes | Who sent this: `user` or `assistant` |
| `content` | string | yes | Message content |
| `project` | string | no | Project name/path (auto-detected if omitted) |
| `session_id` | string | no | Session identifier for grouping turns |

### index_session

Bulk-index a completed conversation session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `turns` | object[] | yes | Array of `{role, content}` objects |
| `project` | string | no | Project name/path |
| `session_id` | string | no | Session identifier |
| `summarize` | boolean | no | Also store a rolled-up summary memory (default false) |

### store_conversation

Store a cross-channel conversation summary with rich metadata.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | yes | Channel name (e.g. `mcp`, `webchat`, `mobile`) |
| `summary` | string | yes | Conversation summary |
| `session_key` | string | no | Unique session identifier |
| `started_at` | string | no | Start time (RFC3339) |
| `ended_at` | string | no | End time (RFC3339) |
| `turn_count` | number | no | Number of turns |
| `topics` | string[] | no | Topics discussed |
| `decisions` | string[] | no | Decisions made |
| `action_items` | string[] | no | Action items |

### recall_conversations

Search conversation memories using hybrid retrieval. Automatically filters to `type=conversation`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `channel` | string | no | Filter by channel |
| `top_k` | number | no | Number of results (default 5) |
| `min_relevance` | number | no | Minimum relevance 0.0–1.0 (default 0.0) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled, recommended 0.01) |

### recent_conversations

List recent conversation summaries in reverse chronological order.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | no | Filter by channel |
| `since` | string | no | Only after this time (RFC3339) |
| `limit` | number | no | Max results (default 10) |

---

## Specialized Search

### recall_lessons

Search lesson memories. Automatically filters to `type=lesson`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled, recommended 0.01) |

### recall_incidents

Search incident memories. Automatically filters to `type=incident`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting (0.0 disabled, recommended 0.01) |

---

## Ingestion

### ingest_conversation

Import a conversation export (supported formats: `grok`, `chatgpt`, `plaintext`, or `auto`).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Raw conversation text or JSON export |
| `source` | string | no | Format: `grok`, `chatgpt`, `plaintext`, `auto` (default) |
| `project` | string | no | Associate memories with this project |
| `dry_run` | boolean | no | Preview import without storing (default false) |

---

## Contradiction Detection

### check_contradictions

Check if content contradicts existing memories.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Text to check |
| `area` | string | no | Filter to this area |
| `sub_area` | string | no | Filter to this sub-area |
| `threshold` | number | no | Similarity threshold 0.0–1.0 (default 0.85) |

---

## Memory Graph

### link_memories

Create a directed relationship between two memories.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `from_id` | string | yes | Source memory ID |
| `to_id` | string | yes | Target memory ID |
| `relation` | string | yes | One of: `caused_by`, `led_to`, `related_to`, `supersedes`, `part_of`, `contradicts` |
| `weight` | number | no | Relationship strength 0.0–1.0 (default 1.0) |

### get_related

Traverse the memory graph from a starting memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `memory_id` | string | yes | Starting memory ID |
| `depth` | number | no | Hops to traverse (default 1) |
| `direction` | string | no | `from`, `to`, or `both` (default `both`) |

### unlink_memories

Remove a link between memories.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `link_id` | string | yes | Link ID to remove |

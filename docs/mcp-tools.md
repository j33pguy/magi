# MCP Tools Reference

magi exposes 17 MCP tools via stdio for use with any MCP-compatible agent. All tools accept JSON parameters and return JSON responses.

## MCP Config Generator

Generate a ready-to-paste MCP configuration block for Claude, Codex, or any MCP-compatible client:

```bash
magi mcp-config
```

**Output:**
```json
{
  "mcpServers": {
    "magi": {
      "command": "magi",
      "args": [],
      "env": {
        "MAGI_DB_URL": "${MAGI_DB_URL}",
        "MAGI_AUTH_TOKEN": "${MAGI_AUTH_TOKEN}",
        "MAGI_API_TOKEN": "${MAGI_API_TOKEN}",
        "MAGI_GRPC_PORT": "8300",
        "MAGI_HTTP_PORT": "8301",
        "MAGI_LEGACY_HTTP_PORT": "8302",
        "MAGI_UI_PORT": "8080"
      }
    }
  }
}
```

Copy the output into your agent's MCP configuration file (e.g., `claude_desktop_config.json`, `.codex/config.json`). Replace the `${...}` placeholders with your actual values.

---

## Core Memory Operations

### remember

Store a memory with automatic semantic embedding, deduplication, and contradiction detection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | The content to remember |
| `project` | string | yes | Project namespace (e.g. `iac`, `magi`, `global`) |
| `type` | string | no | Memory type: `memory`, `incident`, `lesson`, `decision`, `project_context`, `conversation`, `audit`, `runbook`, `preference`, `context`, `security` |
| `summary` | string | no | Brief one-line summary |
| `tags` | string[] | no | Tags for categorization |
| `speaker` | string | no | Who said this: `user, assistant, agent, system`. Default: `assistant` |
| `area` | string | no | Top-level domain: `work`, `home`, `family`, `infrastructure`, `project`, `meta` |
| `sub_area` | string | no | Sub-domain (e.g. `power-platform`, `networking`, `magi`) |
| `dedup_threshold` | number | no | Similarity threshold for dedup (0.0–1.0, default 0.95) |

**Behavior:**
- Auto-classifies area/sub_area from content if not provided
- Rejects content containing potential secrets
- Deduplicates: returns existing memory ID if near-duplicate found (>threshold similarity)
- Soft-groups: links to similar memories as parent (>0.85 similarity)
- Contradiction detection: returns warnings if contradictions found (never blocks writes)
- Auto-tags with taxonomy prefixes (`speaker:`, `area:`, `sub_area:`)

**Example:**
```json
{
  "content": "Switched from Terraform to Ansible for infrastructure IaC — Terraform state management was too painful for a single-node setup",
  "project": "iac",
  "type": "decision",
  "speaker": "grok",
  "area": "infrastructure",
  "sub_area": "iac"
}
```

---

### recall

Hybrid semantic + keyword search with adaptive query rewriting. Uses BM25 and vector cosine similarity fused via Reciprocal Rank Fusion (RRF).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Natural language search query |
| `project` | string | no | Filter by single project namespace |
| `projects` | string[] | no | Filter by multiple namespaces |
| `type` | string | no | Filter by memory type |
| `tags` | string[] | no | Filter by tags (any match) |
| `top_k` | number | no | Number of results (default 5) |
| `min_relevance` | number | no | Minimum relevance score 0.0–1.0 (default 0.0) |
| `recency_decay` | number | no | Exponential decay rate (0.0 = disabled, 0.01 recommended) |
| `speaker` | string | no | Filter by speaker |
| `area` | string | no | Filter by area |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound: relative (`7d`, `2w`, `1m`, `1y`) or absolute (RFC3339) |
| `before` | string | no | Time upper bound: same formats as `after` |

**Behavior:**
- If no results pass `min_relevance`, the query is automatically rewritten and retried once
- Returns results with RRF score, vector rank, BM25 rank, and weighted score

**Example:**
```json
{
  "query": "how did we fix the DNS resolution issue",
  "area": "infrastructure",
  "top_k": 3,
  "min_relevance": 0.3,
  "after": "30d"
}
```

---

### forget

Remove a memory. Soft-deletes (archives) by default.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Memory ID |
| `permanent` | boolean | no | Hard-delete instead of archive (default false) |

---

### list_memories

Browse and filter memories without semantic search.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Filter by project |
| `type` | string | no | Filter by memory type |
| `tags` | string[] | no | Filter by tags |
| `limit` | number | no | Max results (default 20) |
| `offset` | number | no | Pagination offset |
| `speaker` | string | no | Filter by speaker |
| `area` | string | no | Filter by area |
| `sub_area` | string | no | Filter by sub-area |
| `after` | string | no | Time lower bound |
| `before` | string | no | Time upper bound |

---

### update_memory

Modify an existing memory. Re-embeds automatically if content changes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Memory ID |
| `content` | string | no | New content (triggers re-embedding) |
| `summary` | string | no | New summary |
| `type` | string | no | New memory type |
| `tags` | string[] | no | Replace all tags with these |

---

## Conversation Indexing

### index_turn

Index a single conversation turn as a memory. Call at the end of significant turns to passively build memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `role` | string | yes | Who sent this: `user` or `assistant` |
| `content` | string | yes | Message content |
| `project` | string | no | Project name (auto-detected from PWD if omitted) |
| `session_id` | string | no | Session identifier for grouping |

**Behavior:**
- Maps `user` → speaker `user`, `assistant` → speaker `assistant`
- Uses SHA-256 content hash for deduplication (skips already-indexed turns)
- Auto-classifies area/sub_area
- Creates type `conversation` with auto-generated tags

---

### index_session

Bulk-index a completed conversation session. More efficient than calling `index_turn` per message.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `turns` | object[] | yes | Array of `{role, content}` objects |
| `project` | string | no | Project name |
| `session_id` | string | no | Session identifier |
| `summarize` | boolean | no | Also store a rolled-up summary memory (default false) |

**Behavior:**
- Content-hash deduplication per turn
- Skips empty turns, continues on individual failures
- If `summarize: true`, creates a separate `conversation_summary` type memory

---

### store_conversation

Store a cross-channel conversation summary with rich metadata.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `channel` | string | yes | Channel name (e.g. `mcp`, `discord`, `webchat`) |
| `summary` | string | yes | Conversation summary |
| `session_key` | string | no | Unique session identifier |
| `started_at` | string | no | Start time (RFC3339) |
| `ended_at` | string | no | End time (RFC3339) |
| `turn_count` | number | no | Number of turns |
| `topics` | string[] | no | Topics discussed |
| `decisions` | string[] | no | Decisions made |
| `action_items` | string[] | no | Action items |

**Behavior:**
- Auto-tags: `channel:<name>`, `conversation`, `topic:<name>`
- Deduplication at 0.95 similarity, soft-grouping at 0.85
- Visibility set to `private`

---

### recall_conversations

Search conversation history using hybrid retrieval. Automatically filters to type `conversation`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `channel` | string | no | Filter by channel |
| `top_k` | number | no | Number of results (default 5) |
| `min_relevance` | number | no | Minimum relevance 0.0–1.0 |
| `recency_decay` | number | no | Recency weighting (0.01 recommended) |

---

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

Search lesson memories — hard-won knowledge, gotchas, things learned the hard way. Automatically filters to type `lesson`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting |

---

### recall_incidents

Search incident memories — what broke and how it was fixed. Automatically filters to type `incident`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query |
| `project` | string | no | Filter by project |
| `projects` | string[] | no | Filter by multiple namespaces |
| `tags` | string[] | no | Filter by tags |
| `top_k` | number | no | Number of results (default 5) |
| `recency_decay` | number | no | Recency weighting |

---

## Ingestion

### ingest_conversation

Import a conversation export from Grok, ChatGPT, or plain text. Auto-detects format, extracts decisions, lessons, preferences, and context.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Raw conversation text or JSON export |
| `source` | string | no | Format: `grok`, `chatgpt`, `plaintext`, or `auto` (default) |
| `project` | string | no | Associate memories with this project |
| `dry_run` | boolean | no | Preview what would be imported without storing (default false) |

**Behavior:**
- Auto-detects format from content structure
- Extracts structured memories: decisions, lessons, preferences, project context
- Content-hash deduplication
- Dry-run returns preview with counts and sample memories

---

## Contradiction Detection

### check_contradictions

Check if content contradicts existing memories. Uses cosine similarity to find candidates, then applies heuristic scoring (numeric changes, boolean flips, replacement language).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Text to check |
| `area` | string | no | Filter to this area |
| `sub_area` | string | no | Filter to this sub-area |
| `threshold` | number | no | Similarity threshold 0.0–1.0 (default 0.85) |

**Returns:** Array of candidates with similarity %, contradiction score, and human-readable reason.

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

---

### get_related

Traverse the memory graph from a starting memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `memory_id` | string | yes | Starting memory ID |
| `depth` | number | no | Hops to traverse (default 1) |
| `direction` | string | no | `from`, `to`, or `both` (default `both`). Ignored for depth > 1 |

**Behavior:**
- depth=1: direct link query with direction filtering
- depth>1: BFS traversal (always bidirectional)
- Returns full memory objects with their link metadata

---

### unlink_memories

Remove a link between memories.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `link_id` | string | yes | Link ID to remove |

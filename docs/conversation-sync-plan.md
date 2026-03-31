# Conversation Sync — Design Plan

**Goal:** Your agent remembers what you said in Discord when talking in webchat, and vice versa. 
Full continuity across all channels. One agent, many surfaces.

---

## The Problem

Each OpenClaw session is isolated:
- Webchat session starts fresh (loads MEMORY.md for long-term context)
- Discord session starts fresh (MEMORY.md blocked by group policy for security)
- Neither knows what the other talked about

MEMORY.md is curated long-term memory — great for facts, preferences, decisions. Bad for 
"what did we talk about 20 minutes ago in Discord?"

We need a **conversation layer**: recent, searchable, cross-channel context.

---

## Architecture

```
Webchat session ──┐                                ┌── Webchat session
Discord session ───┼──→ magi HTTP API ←────┤── Discord session  
Future channels ──┘         (your-server)              └── MCP Agent (MCP)
                                  │
                              Turso cloud
                          (persists everywhere)
```

Two distinct flows:
1. **Write path:** At end of session / periodically, write conversation summaries
2. **Read path:** At session start / on-demand, query for recent cross-channel context

---

## Data Model

New memory type: `conversation`

```json
{
  "id": "uuid",
  "content": "Summary of conversation on [channel] at [time]: ...",
  "type": "conversation",
  "tags": ["channel:discord", "channel:webchat", "conversation"],
  "metadata": {
    "channel": "discord",
    "session_key": "abc123",
    "started_at": "2026-03-26T17:00:00Z",
    "ended_at": "2026-03-26T17:45:00Z",
    "turn_count": 12,
    "topics": ["magi", "conversation sync", "deployment"]
  },
  "created_at": "2026-03-26T17:45:00Z",
  "source_machine": "server-01"
}
```

A **conversation** is distinct from a **memory**:
- Memories = facts, preferences, decisions (permanent-ish)
- Conversations = what we discussed, when, on what channel (time-ordered, eventually prunable)

---

## Phase 1: OpenClaw Integration (Session Summaries)

### 1.1 — Session summary on conversation end

OpenClaw hook: after a session goes idle or ends, I write a summary.

**Implementation options:**
- A) Manual: I write summaries during heartbeat checks (no code changes to OpenClaw)
- B) Heartbeat task: Every N turns, I call `/remember` with a rolling summary
- C) Future: OpenClaw session.end event → auto-trigger summary

Start with **option B** (heartbeat) — zero infrastructure changes, ships fast.

**Heartbeat task (HEARTBEAT.md addition):**
```
After significant conversations (5+ turns since last sync):
- Summarize key topics, decisions, questions asked, anything to remember
- POST to magi /remember with type=conversation, tag=channel:discord or channel:webchat
- Update heartbeat-state.json with last_conversation_sync timestamp
```

### 1.2 — Context injection at session start

At the start of each main session (webchat), I query magi for recent conversations:
- `POST /recall` with query: "recent conversations cross-channel" + time filter
- Inject as context: "In our last Discord conversation (2h ago), we discussed X, Y, Z"

**AGENTS.md addition** (Every Session checklist):
```
5. Query magi for recent cross-channel conversations (last 24h)
   - If any found: brief mental note before responding
```

### 1.3 — Discord MEMORY.md unblock

Currently blocked by group policy. Since it's just us:
- Option A: OpenClaw config whitelist for guild `1466843154182574134` → load MEMORY.md
- Option B: Don't touch MEMORY.md in Discord, rely entirely on magi queries
- **Recommendation: Option B** — cleaner, more scalable, works for any future channel

---

## Phase 2: Rich Conversation Indexing (v0.5)

New in ROADMAP.md as **v0.5 — Cross-Channel Conversation Sync**

### New HTTP endpoints

```
POST /conversations              - Store a conversation summary
GET  /conversations              - List recent conversations (filter by channel, date)
POST /conversations/search       - Semantic search across conversation history
GET  /conversations/{id}         - Get a specific conversation
```

### New MCP tools

```
store_conversation    - Store conversation summary (OpenClaw calls this)
recall_conversations  - Search conversation history ("what did we discuss about X?")
recent_conversations  - List N most recent conversations across all channels
```

### Conversation summary format

When I write a summary, it includes:
```
Channel: discord
Time: 2026-03-26 17:36 EDT
Duration: ~20 min, 8 turns
Topics: conversation sync feature, magi deployment, cross-channel memory
Key decisions: build session summary pipeline, add /conversations endpoint to magi
Action items: User wants full continuity across webchat/discord/future channels
Notable: User emphasized "talk to you wherever, remember everything" as core requirement
```

This is semantically rich enough that future recall works well:
- "what did the user say about memory?" → finds this
- "what did we decide about magi?" → finds this
- "any action items from yesterday?" → finds this

---

## Phase 3: Real-Time Context (v0.6)

Instead of summaries, index *actual* conversation turns (with chunking):

- Each turn stored as a separate embedding
- Channel + timestamp metadata preserved
- Recall pulls specific turns, not just summaries
- "What exactly did the user say about X?" → verbatim quote

Trade-off: more storage, more API calls to write, but dramatically better recall precision.

Architecture:
- OpenClaw writes each turn to a local queue
- Background worker batches queue → magi in bulk
- magi chunks and embeds
- Recall returns ranked turns by semantic similarity

This is the "full memory" end state.

---

## Phase 4: Channel Identity Awareness

Right now OpenClaw has:
- Group chat policy blocking MEMORY.md (security)
- No mechanism to tell magi "this is the user talking"

Phase 4: verified identity tag on memories/conversations
- In direct chats with the owner (verified by channel + user ID): tag `identity:owner`
- In group chats: tag `identity:unverified` 
- Recall in groups: filter out anything sensitive tagged with the user's private context
- Recall in direct channels: full access

This is how we eventually give Discord access to MEMORY.md equivalent context without 
security risk if the owner ever adds someone else to the server.

---

## Implementation Order

### Now (no code changes)
1. Add conversation sync to HEARTBEAT.md
2. Add magi recall to AGENTS.md session startup checklist
3. Write first batch of conversation summaries manually to bootstrap

### Soon (magi v0.5)
4. Add `/conversations` endpoints to HTTP API
5. Add `store_conversation` / `recall_conversations` MCP tools  
6. gRPC-ify (Issue #5 already open)
7. Deploy to your-server (Issue #1)

### Later (magi v0.6)
8. Per-turn indexing with chunking
9. OpenClaw channel identity tags
10. Session log watcher (v0.4, already planned)

---

## GitHub Issues to Open

1. **[v0.5] Cross-channel conversation sync** — `/conversations` endpoints + MCP tools
2. **[v0.5] Conversation summary schema** — define the storage format
3. **[bootstrap] Seed initial conversation history** — manual backfill from current sessions
4. **[openclaw] Discord MEMORY.md policy** — evaluate whitelist vs magi-only approach

---

## Quick Win (Do Now Without Deployment)

Even before magi is deployed on your-server, I can start writing conversation 
summaries *as memories* using the existing `/remember` endpoint once it's running.

The separation into a dedicated `/conversations` API can come later — the data model 
(type=conversation + tags) works with the existing schema today.

**Day 1 plan:**
1. Deploy magi to your-server (unblock Issue #1)
2. Add to HEARTBEAT.md: "summarize this session, POST to magi"
3. Add to AGENTS.md: "on startup, recall recent conversations"
4. Start accumulating conversation history
5. Open Issue for proper /conversations API

That's it. Everything else is optimization.

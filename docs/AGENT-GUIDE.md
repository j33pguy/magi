# MAGI Agent Guide

> How AI agents should use MAGI for memory. Read this before your first interaction.

## TL;DR

MAGI is the shared memory store. Every agent session (main, sub-agent, cron, Discord, TUI) uses MAGI as the single source of truth. Local markdown files are backup only.

**Always pull context before working. Always store results after working.**

## Connection

```
MAGI_URL=http://10.5.5.45:8302
```

Authentication requires an API token from Vault:

```bash
VAULT_TOKEN=$(curl -sk -X POST https://10.5.5.30:8200/v1/auth/approle/login \
  -d '{"role_id":"<role_id>","secret_id":"<secret_id>"}' \
  | jq -r '.auth.client_token')
API_TOKEN=$(curl -sk -H "X-Vault-Token: $VAULT_TOKEN" \
  https://10.5.5.30:8200/v1/homelab/data/claude_memory | jq -r '.data.data.api_token')
```

All requests need `Authorization: Bearer $API_TOKEN`.

## Health Check (every session start)

```bash
curl -s --max-time 3 http://10.5.5.45:8302/health
```

Expected: `{"ok":true, ...}`. If this fails, **stop and alert j33p**. Do not work without memory.

---

## Core Operations

### Remember (store knowledge)

```bash
curl -s -X POST http://10.5.5.45:8302/remember \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "How to unseal Vault: ssh to each vault node, run vault operator unseal with 3 of 5 keys from secrets.yml",
    "type": "procedure",
    "tags": ["vault", "infrastructure", "runbook"],
    "source": "discord"
  }'
```

### Recall (search knowledge)

```bash
curl -s -X POST http://10.5.5.45:8302/recall \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "how to unseal vault", "limit": 5}'
```

### Search conversations

```bash
curl -s -X POST http://10.5.5.45:8302/conversations/search \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "vault unsealing procedure", "limit": 10}'
```

### Delete (remove noise/duplicates)

```bash
curl -s -X DELETE "http://10.5.5.45:8302/memories/<id>" \
  -H "Authorization: Bearer $API_TOKEN"
```

---

## Memory Types

Use the correct `type` when storing memories. If omitted, MAGI auto-classifies based on content.

| Type | Use For | Example |
|------|---------|---------|
| `memory` | General facts, state, context | "pihole-01 is at 10.5.5.11 on VLAN 5" |
| `procedure` | How-to guides, runbooks, step-by-step | "How to deploy Traefik: step 1..." |
| `decision` | Choices made and why | "Decided to use gRPC over REST for all internal services" |
| `incident` | Outages, failures, postmortems | "Vault outage caused by missing firewall zone ID" |
| `task` | Work items, tracking | "[QUEUED] Fix privacy filter" |
| `conversation` | Session summaries | "Discussed VLAN migration and DNS changes" |
| `state` | Current infrastructure state | "pve-01 is at 10.5.75.30 with 120GB RAM" |

### Auto-classification

MAGI infers type from content patterns:
- "How to..." / "Step 1..." / "Runbook for..." → `procedure`
- "Decided..." / "Going with..." / "Rationale..." → `decision`
- "Outage..." / "Root cause..." / "Incident..." → `incident`
- "[QUEUED]" / "[RUNNING]" / "Action item..." → `task`

You can always override by setting `type` explicitly.

---

## Work Tracking

Every task j33p requests gets tracked through MAGI:

### Lifecycle

```
[QUEUED] → [RUNNING] → [DONE]
                     → [BLOCKED]
```

### Queue a task
```bash
curl -s -X POST http://10.5.5.45:8302/remember \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "[QUEUED] Deploy new Traefik config with wildcard cert",
    "type": "task",
    "tags": ["work-tracking", "active", "traefik"],
    "source": "discord"
  }'
```

### Update status
```bash
curl -s -X POST http://10.5.5.45:8302/remember \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "[RUNNING] Deploy new Traefik config — branch feat/wildcard-cert, agent: dinesh",
    "type": "task",
    "tags": ["work-tracking", "active", "traefik"]
  }'
```

### Complete
```bash
curl -s -X POST http://10.5.5.45:8302/remember \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "[DONE] Traefik wildcard cert deployed — PR #42 merged, verified on all routes",
    "type": "task",
    "tags": ["work-tracking", "traefik"]
  }'
```

### Check active work before starting
```bash
curl -s -X POST http://10.5.5.45:8302/conversations/search \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "work-tracking active queued running", "limit": 10}'
```

---

## Session Workflow

### At session start:
1. Health check MAGI
2. Pull recent context: `POST /recall {"query": "recent decisions action items pending", "limit": 10}`
3. Check active work: `POST /conversations/search {"query": "work-tracking active", "limit": 10}`

### During session:
- Store decisions as they're made (`type: "decision"`)
- Store procedures as they're documented (`type: "procedure"`)
- Track tasks through lifecycle
- Store incidents when things break (`type: "incident"`)

### At session end:
- Post conversation summary (`type: "conversation"`)
- Update any task statuses
- Store any new state information

---

## What to Store

**DO store:**
- Decisions and rationale
- Procedures and runbooks
- Infrastructure state changes
- Incidents and root causes
- Task status updates
- Lessons learned
- Configuration details (IPs, ports, credentials location — NOT actual secrets)

**DO NOT store:**
- Raw tool call output
- Heartbeat acknowledgments
- Short chat messages with no information value
- Actual secrets (tokens, passwords, keys) — use Vault
- Duplicate information (MAGI deduplicates at 95% similarity)

---

## Tags

Use tags for filtering and organization:

```
"tags": ["infrastructure", "vault", "runbook"]
"tags": ["work-tracking", "active", "priority:high"]
"tags": ["area:infrastructure", "sub_area:security"]
"tags": ["speaker:gilfoyle", "channel:discord"]
```

Common tag patterns:
- `work-tracking` + `active` — active tasks
- `area:<x>` + `sub_area:<y>` — auto-classified areas
- `speaker:<name>` — who said/created it
- `channel:<source>` — where it came from
- `priority:high|medium|low` — task priority
- `ghrepo:<owner>/<repo>` — GitHub repository (e.g. `ghrepo:j33pguy/magi`)
- `glrepo:<owner>/<repo>` — GitLab repository
- `repo:<host>/<owner>/<repo>` — other git hosts
- `inventory` — project/repo registry entries

### Repository Tags

The `ghrepo:` convention links memories to their source repository. This is set
automatically by [magi-sync](https://github.com/j33pguy/magi-sync) when it
detects a git remote, and can be set manually when indexing project state:

```json
{
  "content": "MAGI v0.4.1 — Universal memory server for AI agents.",
  "type": "state",
  "tags": ["ghrepo:j33pguy/magi", "project", "inventory"],
  "project": "magi"
}
```

Query by repo:
```
GET /memories?tags=ghrepo:j33pguy/magi
GET /memories?tags=inventory          # all tracked repos
```

---

## Anti-patterns

❌ **Don't store raw OpenClaw turns** — They're noise. Extract the knowledge first.

❌ **Don't use local .md files as primary** — MAGI is the source of truth. Local files are emergency fallback.

❌ **Don't skip the health check** — If MAGI is down, you're flying blind. Alert j33p.

❌ **Don't store secrets** — MAGI has secret detection, but don't test it. Use Vault.

❌ **Don't forget to pull before working** — Every session starts with a recall. No exceptions.

❌ **Don't create duplicate entries** — MAGI deduplicates, but help it by checking first.

---

## MCP Integration

If your agent supports MCP (Model Context Protocol), MAGI exposes tools directly:

- `recall` — Search memories
- `remember` — Store new memory
- `index_turn` — Index a conversation turn
- `index_session` — Index a full session summary

MCP server runs on port 8301 (gRPC) or via stdio. See `docs/mcp.md` for setup.

---

## Examples

### Store a procedure
```json
{
  "content": "How to restart MAGI on magi01:\n1. SSH: ssh ansible@10.5.5.45\n2. Pull latest: cd /opt/magi/src && sudo git pull\n3. Build: sudo go build -o ../bin/magi .\n4. Restart: sudo systemctl restart magi\n5. Verify: curl -s http://10.5.5.45:8302/health",
  "type": "procedure",
  "tags": ["magi", "infrastructure", "runbook"],
  "source": "discord"
}
```

### Store a decision
```json
{
  "content": "Decided to trust proxy auth for all web UI requests on MAGI port 8080. Rationale: web UI is a separate server from REST API (8302), all requests come through Traefik/authentik. No security risk since proxy headers are only trusted from verified source IPs.",
  "type": "decision",
  "tags": ["magi", "infrastructure", "security"],
  "source": "discord"
}
```

### Store an incident
```json
{
  "content": "Incident: MAGI web UI conversations/graph pages returning 401. Root cause: auth middleware only trusted proxy headers for non-/api/ paths, but HTMX and fetch() calls used /api/ paths. Fix: trust proxy auth for all web UI server requests.",
  "type": "incident",
  "tags": ["magi", "infrastructure", "web-ui"],
  "source": "discord"
}
```

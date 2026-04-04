# @j33pguy/magi

TypeScript/JavaScript client SDK for [MAGI](https://github.com/j33pguy/magi) — Multi-Agent Graph Intelligence.

Zero dependencies. Uses native `fetch` (Node 18+).

## Install

```bash
npm install @j33pguy/magi
```

## Quick Start

```typescript
import { Magi } from '@j33pguy/magi';

const magi = new Magi({
  baseUrl: 'http://localhost:8302',
  token: 'your-token',
});
```

## Configuration

| Option | Type | Default | Description |
|---|---|---|---|
| `baseUrl` | `string` | — | MAGI server URL (required) |
| `token` | `string` | — | Bearer token for auth |
| `maxRetries` | `number` | `3` | Retries on 5xx / network errors (0 = none) |
| `retryBaseMs` | `number` | `200` | Initial backoff in ms (doubles each attempt) |

## Error Handling

All methods throw `MagiError` on non-2xx responses:

```typescript
import { MagiError } from '@j33pguy/magi';

try {
  await magi.recall({ query: 'test' });
} catch (err) {
  if (err instanceof MagiError) {
    console.error(err.status, err.message);
  }
}
```

## Memories

### remember — Store a memory

```typescript
const { id } = await magi.remember({
  content: 'v3 API deprecates /users',
  project: 'myapp',
  type: 'decision',
  speaker: 'grok',
  tags: ['api', 'deprecation'],
});
```

### recall — Semantic recall

```typescript
const { results } = await magi.recall({
  query: 'API changes',
  limit: 5,
  project: 'myapp',
});

for (const r of results) {
  console.log(r.score, r.memory.content);
}
```

### search — Hybrid search (BM25 + vector)

```typescript
const results = await magi.search('API deprecation', {
  top_k: 10,
  project: 'myapp',
  recency_decay: 0.01,
});
```

### list — List memories

```typescript
const memories = await magi.list({ project: 'myapp' });
```

### update — Patch a memory

```typescript
await magi.update(memoryId, { content: 'updated content', tags: ['new-tag'] });
```

### forget — Delete a memory

```typescript
await magi.forget(memoryId);
```

### memoryHistory — Git-backed version history

```typescript
const history = await magi.memoryHistory(memoryId);
for (const entry of history.entries) {
  console.log(entry.hash, entry.author, entry.message);
}
```

### memoryDiff — Diff between two versions

```typescript
const diff = await magi.memoryDiff(memoryId, 'abc123', 'def456');
console.log(diff.diff);
```

## Knowledge Graph

### link — Create a link between memories

```typescript
await magi.link(fromId, toId, 'related_to', 0.9);
```

### unlink — Delete a link

```typescript
await magi.unlink(linkId);
```

### getRelated — Get related memories via graph traversal

```typescript
const related = await magi.getRelated(memoryId);
for (const r of related) {
  console.log(r.memory.content, r.links.map(l => l.relation));
}
```

## Conversations

### storeConversation — Store a conversation summary

```typescript
const { id } = await magi.storeConversation({
  channel: 'slack-engineering',
  summary: 'Decided to migrate auth to OAuth2',
  topics: ['auth', 'oauth2'],
  decisions: ['Use Auth0 as provider'],
  action_items: ['Draft migration plan by Friday'],
});
```

### listConversations — List conversations

```typescript
const convos = await magi.listConversations({
  limit: 20,
  channel: 'slack-engineering',
  since: '2025-01-01T00:00:00Z',
});
```

### searchConversations — Semantic search over conversations

```typescript
const result = await magi.searchConversations({
  query: 'auth migration',
  limit: 5,
  min_relevance: 0.7,
});
```

### getConversation — Get a single conversation

```typescript
const convo = await magi.getConversation(conversationId);
```

## Tasks

### createTask — Create a task

```typescript
const task = await magi.createTask({
  title: 'Migrate auth middleware',
  project: 'myapp',
  priority: 'high',
  description: 'Replace legacy session auth with OAuth2',
  created_by: 'claude',
});
```

### listTasks — List tasks with filters

```typescript
const tasks = await magi.listTasks({
  project: 'myapp',
  status: 'started',
  limit: 50,
});
```

### getTask — Get a single task

```typescript
const task = await magi.getTask(taskId);
```

### updateTask — Patch a task

```typescript
const updated = await magi.updateTask(taskId, {
  status: 'done',
  status_comment: 'Migration complete',
  actor_name: 'claude',
});
```

### createTaskEvent — Log an event on a task

```typescript
const event = await magi.createTaskEvent(taskId, {
  event_type: 'status',
  status: 'started',
  actor_name: 'claude',
  summary: 'Beginning migration work',
});
```

### listTaskEvents — List events for a task

```typescript
const events = await magi.listTaskEvents(taskId, 100);
```

## Pipeline

### pipelineStats — Get async pipeline stats

```typescript
const stats = await magi.pipelineStats();
console.log(`Queue: ${stats.queue_depth}, Completed: ${stats.completed}`);
```

## Health

```typescript
const { ok, version } = await magi.health();
```

## License

MIT

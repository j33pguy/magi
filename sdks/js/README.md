# @j33pguy/magi

TypeScript/JavaScript client SDK for [MAGI](https://github.com/j33pguy/magi).

## Install

```bash
npm install @j33pguy/magi
```

## Usage

```typescript
import { Magi } from '@j33pguy/magi';

const magi = new Magi({ baseUrl: 'http://localhost:8302', token: 'your-token' });

await magi.remember({ content: 'v3 API deprecates /users', project: 'myapp', type: 'decision', speaker: 'grok' });

const { results } = await magi.recall({ query: 'API changes', limit: 5 });

await magi.link(results[0].memory.id, results[1].memory.id, 'related_to');
```

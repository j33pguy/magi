# MAGI Examples

Quick entry points for integrating MAGI across tools and stacks.

## Files

- `claude-mcp-config.json` — MCP config you can paste into Claude Desktop/Code.
- `python-client.py` — Requests-based client showing `remember`, `recall`, and `search` with token auth.
- `langchain-integration.py` — Conceptual LangChain memory wrapper backed by MAGI.
- `docker-compose.quickstart.yml` — Minimal compose file for a local server.
- `.env.example` — Environment template with all config options and defaults.

## Quickstart (Docker Compose)

1. Copy `.env.example` to `.env` and set `MAGI_API_TOKEN`.
2. Run:

```bash
cd examples
cp .env.example .env
docker compose -f docker-compose.quickstart.yml up
```

The legacy REST API listens on `http://localhost:8302`.

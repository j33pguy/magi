# magi-sync Design

Status: Draft

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

`magi-sync` is the local edge binary that runs on isolated machines and feeds durable context into a shared MAGI server.

## Goals

- ingest useful local agent context into a shared MAGI instance
- support multiple isolated machines
- enforce local privacy controls before anything leaves the machine
- keep the first experience simple: install, configure, point at MAGI, and sync

## Current Implementation (Phase 1)

These statements match `internal/syncagent/`.

- sync mode is `push` only
- transport is HTTP
- enrollment uses `POST /auth/machines/enroll`
- uploads use `POST /sync/memories` with a fallback to `POST /remember`
- config file default: `~/.config/magi-sync/config.yaml`
- local state file default: `~/.config/magi-sync/state.json`
- one built-in adapter is implemented (type value is defined in code)
- payload types produced today: `project_context`, `conversation_summary`, `conversation`
- privacy controls implemented today:
  - include/exclude glob rules
  - `max_file_size_kb` enforcement
  - `redact_secrets` (regex-based redaction)

Note: `privacy.mode`, `sync.watch`, and `sync.max_batch_size` are validated in config but not yet used to change runtime behavior. Replace `adapter` in the config with the built-in adapter name defined in code.

## Config Schema (Current)

```yaml
server:
  url: http://MAGI_HTTP_ADDR:8302
  token: ""
  token_env: ""
  enroll_token: ""
  enroll_token_env: ""
  protocol: http

machine:
  id: laptop
  user: user
  groups:
    - team

sync:
  mode: push
  watch: false
  interval: 30s
  retry_backoff: 5s
  max_batch_size: 50
  state_file: ~/.config/magi-sync/state.json

privacy:
  mode: allowlist
  redact_secrets: true
  max_file_size_kb: 512

agents:
  - type: adapter
    name: adapter
    enabled: true
    owner: user
    viewers:
      - teammate
    viewer_groups:
      - team
    visibility: internal
    paths:
      - ~/.config/agent
    include:
      - "**/*.jsonl"
      - "**/*.md"
    exclude:
      - "**/tmp/**"
      - "**/cache/**"
```

## Privacy Model (Current)

- include/exclude rules apply to all configured paths
- only files under `max_file_size_kb` are scanned
- redaction replaces common secret-like patterns with `[REDACTED]`

## Remote Access Guidance

Use a private network boundary or VPN to reach the MAGI server from remote machines. Avoid public exposure unless you have strong authentication and monitoring in place.

## Future Work (Non-Binding)

- pull or bidirectional sync
- additional adapters beyond the current built-in adapter
- richer privacy modes beyond include/exclude
- batching and queueing controls

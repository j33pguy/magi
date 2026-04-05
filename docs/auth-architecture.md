# Authentication Architecture

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

This document reflects the current auth behavior implemented in `internal/auth/` and the HTTP middleware.

## Identity Model

MAGI tracks these identity facets:

- `user`
- `machine`
- `agent`
- `groups`

Identity is derived server-side from bearer tokens. Caller-supplied identity headers are not trusted unless they were set by MAGI itself.

## Current Authentication Lanes

### Admin bearer token

- Set `MAGI_API_TOKEN`.
- Requests must include `Authorization: Bearer ADMIN_TOKEN`.
- Without any auth configured, MAGI allows GETs only and blocks writes.

### Machine tokens

Machine tokens are supported in two ways:

- bootstrap tokens via `MAGI_MACHINE_TOKENS_JSON` or `MAGI_MACHINE_TOKENS_FILE`
- DB-backed machine credentials created by the enrollment endpoints

Machine tokens authenticate the same way as admin tokens but resolve to a machine identity.

## Machine Management Endpoints

Admin-only endpoints on the legacy HTTP API:

- `POST /auth/machines/enroll` — admin creates a machine credential directly
- `GET /auth/machines` — list all machine credentials
- `POST /auth/machines/{id}/revoke` — revoke a machine credential

## Self-Enrollment (v0.4.1+)

For onboarding machines without sharing the admin API token, use enrollment tokens:

### Admin: Create Enrollment Token

```bash
curl -X POST /auth/enrollment-tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "label": "team-machines",
    "max_uses": 10,
    "default_user": "j33p",
    "default_groups": ["homelab"],
    "expires_in": "24h"
  }'
```

Returns a one-time token (plaintext shown once, hash stored).

### Client: Self-Enroll

```bash
curl -X POST /auth/enroll \
  -d '{
    "token": "<enrollment-token>",
    "machine_id": "my-laptop",
    "agent_name": "magi-sync",
    "agent_type": "syncagent"
  }'
```

**No Authorization header needed.** The enrollment token in the body is the auth.
Returns a machine credential (token + record). The enrollment token's use count
is incremented; once `max_uses` is reached, the token is exhausted.

### Admin: Manage Enrollment Tokens

- `GET /auth/enrollment-tokens` — list all enrollment tokens
- `POST /auth/enrollment-tokens/{id}/revoke` — revoke a token

### magi-sync Integration

With [magi-sync](https://github.com/j33pguy/magi-sync) v0.3.0+, set
`server.enroll_token` in config and run `magi-sync enroll`. The token is
exchanged for a machine credential which is saved to `server.token` automatically.

## Identity Propagation

When a token is valid, MAGI injects identity into request headers for downstream filtering:

- `X-MAGI-Auth-User`
- `X-MAGI-Auth-Groups`
- `X-MAGI-Auth-Machine`
- `X-MAGI-Auth-Agent`
- `X-MAGI-Auth-Kind`

These headers are set by MAGI itself, not trusted from the client.

## Access Control Model

MAGI uses tags and visibility for access filtering:

- `owner:user`
- `viewer:user`
- `viewer_group:team`

Visibility levels:

- `private`
- `internal`
- `team`
- `shared`
- `public`

Access enforcement is applied in recall, search, and list handlers when identity is available.

## Web UI Auth

The web UI uses the same admin bearer token by default. If `MAGI_TRUSTED_PROXY_AUTH=true`, the UI can trust a reverse-proxy auth header for non-API routes while API routes still require bearer tokens.

## Secret Handling

Secret material is treated as a separate trust boundary:

- detect likely secrets during `remember`
- reject by default
- optionally externalize to a configured secret backend
- resolve stored references only through authenticated server-side flows

The built-in backend identifier is `vault` and expects a KV v2-style service.

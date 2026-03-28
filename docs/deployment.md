# Deployment Guide

## Prerequisites

- Go 1.25+ with CGO enabled
- ONNX Runtime shared library installed
- A [Turso](https://turso.tech) database (free tier works)

### Installing ONNX Runtime

**macOS:**
```bash
brew install onnxruntime
```

**Fedora/RHEL:**
```bash
dnf install onnxruntime-devel
```

**Ubuntu/Debian:**
Download the release from [github.com/microsoft/onnxruntime/releases](https://github.com/microsoft/onnxruntime/releases) and install the `.so` to `/usr/local/lib/`:
```bash
tar xzf onnxruntime-linux-x64-*.tgz
sudo cp onnxruntime-linux-x64-*/lib/libonnxruntime.so* /usr/local/lib/
sudo ldconfig
```

## Building

```bash
git clone https://github.com/j33pguy/claude-memory
cd claude-memory
CGO_ENABLED=1 make build
```

This produces two binaries in `bin/`:
- `claude-memory` — main server
- `claude-memory-import` — memory file importer

Install system-wide:
```bash
sudo make install   # copies to /usr/local/bin/
```

## Systemd Service

Create `/etc/systemd/system/claude-memory.service`:

```ini
[Unit]
Description=claude-memory AI memory server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=claude-memory
Group=claude-memory
ExecStart=/opt/claude-memory/bin/claude-memory --http-only
Restart=on-failure
RestartSec=5

# Environment
Environment=TURSO_URL=libsql://claude-memory-<you>.turso.io
Environment=TURSO_AUTH_TOKEN=<token>
Environment=CLAUDE_MEMORY_API_TOKEN=<bearer-token>
Environment=CLAUDE_MEMORY_REPLICA_PATH=/var/lib/claude-memory/memory.db
Environment=CLAUDE_MEMORY_MODEL_DIR=/opt/claude-memory/models
Environment=CLAUDE_MEMORY_GRPC_PORT=8300
Environment=CLAUDE_MEMORY_HTTP_PORT=8301
Environment=CLAUDE_MEMORY_LEGACY_HTTP_PORT=8302
Environment=CLAUDE_MEMORY_UI_PORT=8080

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/claude-memory
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Set up the service:

```bash
# Create service user
sudo useradd -r -s /sbin/nologin claude-memory

# Create data directory
sudo mkdir -p /var/lib/claude-memory /opt/claude-memory/bin /opt/claude-memory/models
sudo chown claude-memory:claude-memory /var/lib/claude-memory

# Install binary
sudo cp bin/claude-memory /opt/claude-memory/bin/

# Download ONNX model (first run)
# The model is auto-downloaded to MODEL_DIR on first startup,
# or copy it manually from ~/.claude/models/

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable claude-memory
sudo systemctl start claude-memory

# Check status
sudo systemctl status claude-memory
journalctl -u claude-memory -f
```

**Note:** The `--http-only` flag runs gRPC, grpc-gateway, legacy HTTP, and web UI servers without the stdio MCP server. MCP mode is for direct Claude Code integration (where the binary is launched by Claude Code as a subprocess).

## Reverse Proxy (Traefik)

Example Traefik dynamic config to expose the web UI and API behind authentication:

```yaml
# traefik/dynamic/claude-memory.yml
http:
  routers:
    claude-memory-ui:
      rule: "Host(`memory.example.com`)"
      service: claude-memory-ui
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
      middlewares:
        - authentik@docker   # or your auth middleware

    claude-memory-api:
      rule: "Host(`memory-api.example.com`)"
      service: claude-memory-api
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
      # API uses Bearer token auth, no middleware needed

  services:
    claude-memory-ui:
      loadBalancer:
        servers:
          - url: "http://localhost:8080"

    claude-memory-api:
      loadBalancer:
        servers:
          - url: "http://localhost:8302"
```

## Ports Summary

| Port | Protocol | Interface | Purpose |
|------|----------|-----------|---------|
| 8300 | gRPC (h2) | `CLAUDE_MEMORY_GRPC_PORT` | Native gRPC clients |
| 8301 | HTTP/JSON | `CLAUDE_MEMORY_HTTP_PORT` | grpc-gateway reverse proxy |
| 8302 | HTTP/JSON | `CLAUDE_MEMORY_LEGACY_HTTP_PORT` | Legacy REST API |
| 8080 | HTTP | `CLAUDE_MEMORY_UI_PORT` | Web UI |

## CI/CD

The project uses GitHub Actions with a self-hosted runner. On push to `main`:

1. **Build & Test** — `go build`, `go test`, `go vet`
2. **Deploy** — builds production binary, installs to `/opt/claude-memory/bin/`, restarts systemd service, verifies health

The runner runs directly on the target server, so deployment is a local copy + restart (no SSH/SCP).

## Turso Setup

1. Create a free Turso account at [turso.tech](https://turso.tech)
2. Create a database:
   ```bash
   turso db create claude-memory
   ```
3. Get the URL and token:
   ```bash
   turso db show claude-memory --url
   turso db tokens create claude-memory
   ```
4. Set environment variables:
   ```bash
   export TURSO_URL=libsql://claude-memory-<you>.turso.io
   export TURSO_AUTH_TOKEN=<token>
   ```

Schema migrations run automatically on startup. The embedded replica syncs to Turso cloud every 60 seconds (configurable via `CLAUDE_MEMORY_SYNC_INTERVAL`).

## Model Setup

The ONNX model (all-MiniLM-L6-v2) is auto-downloaded on first run to `CLAUDE_MEMORY_MODEL_DIR` (default `~/.claude/models/`). For air-gapped deployments, download the model files manually:

- `model.onnx` — the ONNX model
- `tokenizer.json` or `vocab.txt` — BERT WordPiece vocabulary

Place them in the model directory before starting the server.

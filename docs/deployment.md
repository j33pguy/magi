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
git clone https://github.com/j33pguy/magi
cd magi
CGO_ENABLED=1 make build
```

This produces two binaries in `bin/`:
- `magi` — main server
- `magi-import` — memory file importer

Install system-wide:
```bash
sudo make install   # copies to /usr/local/bin/
```

## Systemd Service

Create `/etc/systemd/system/magi.service`:

```ini
[Unit]
Description=magi AI memory server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=magi
Group=magi
ExecStart=/opt/magi/bin/magi --http-only
Restart=on-failure
RestartSec=5

# Environment
Environment=TURSO_URL=libsql://magi-<you>.turso.io
Environment=TURSO_AUTH_TOKEN=<token>
Environment=MAGI_API_TOKEN=<bearer-token>
Environment=MAGI_REPLICA_PATH=/var/lib/magi/memory.db
Environment=MAGI_MODEL_DIR=/opt/magi/models
Environment=MAGI_GRPC_PORT=8300
Environment=MAGI_HTTP_PORT=8301
Environment=MAGI_LEGACY_HTTP_PORT=8302
Environment=MAGI_UI_PORT=8080

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/magi
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Set up the service:

```bash
# Create service user
sudo useradd -r -s /sbin/nologin magi

# Create data directory
sudo mkdir -p /var/lib/magi /opt/magi/bin /opt/magi/models
sudo chown magi:magi /var/lib/magi

# Install binary
sudo cp bin/magi /opt/magi/bin/

# Download ONNX model (first run)
# The model is auto-downloaded to MODEL_DIR on first startup,
# or copy it manually from ~/.claude/models/

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable magi
sudo systemctl start magi

# Check status
sudo systemctl status magi
journalctl -u magi -f
```

**Note:** The `--http-only` flag runs gRPC, grpc-gateway, legacy HTTP, and web UI servers without the stdio MCP server. MCP mode is for direct Claude Code integration (where the binary is launched by Claude Code as a subprocess).

## Reverse Proxy (Traefik)

Example Traefik dynamic config to expose the web UI and API behind authentication:

```yaml
# traefik/dynamic/magi.yml
http:
  routers:
    magi-ui:
      rule: "Host(`memory.example.com`)"
      service: magi-ui
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
      middlewares:
        - authentik@docker   # or your auth middleware

    magi-api:
      rule: "Host(`memory-api.example.com`)"
      service: magi-api
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
      # API uses Bearer token auth, no middleware needed

  services:
    magi-ui:
      loadBalancer:
        servers:
          - url: "http://localhost:8080"

    magi-api:
      loadBalancer:
        servers:
          - url: "http://localhost:8302"
```

## Ports Summary

| Port | Protocol | Interface | Purpose |
|------|----------|-----------|---------|
| 8300 | gRPC (h2) | `MAGI_GRPC_PORT` | Native gRPC clients |
| 8301 | HTTP/JSON | `MAGI_HTTP_PORT` | grpc-gateway reverse proxy |
| 8302 | HTTP/JSON | `MAGI_LEGACY_HTTP_PORT` | Legacy REST API |
| 8080 | HTTP | `MAGI_UI_PORT` | Web UI |

## CI/CD

The project uses GitHub Actions with a self-hosted runner. On push to `main`:

1. **Build & Test** — `go build`, `go test`, `go vet`
2. **Deploy** — builds production binary, installs to `/opt/magi/bin/`, restarts systemd service, verifies health

The runner runs directly on the target server, so deployment is a local copy + restart (no SSH/SCP).

## Turso Setup

1. Create a free Turso account at [turso.tech](https://turso.tech)
2. Create a database:
   ```bash
   turso db create magi
   ```
3. Get the URL and token:
   ```bash
   turso db show magi --url
   turso db tokens create magi
   ```
4. Set environment variables:
   ```bash
   export TURSO_URL=libsql://magi-<you>.turso.io
   export TURSO_AUTH_TOKEN=<token>
   ```

Schema migrations run automatically on startup. The embedded replica syncs to Turso cloud every 60 seconds (configurable via `MAGI_SYNC_INTERVAL`).

## Model Setup

The ONNX model (all-MiniLM-L6-v2) is auto-downloaded on first run to `MAGI_MODEL_DIR` (default `~/.claude/models/`). For air-gapped deployments, download the model files manually:

- `model.onnx` — the ONNX model
- `tokenizer.json` or `vocab.txt` — BERT WordPiece vocabulary

Place them in the model directory before starting the server.

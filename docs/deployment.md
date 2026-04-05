# Deployment Guide

MAGI is model- and agent-agnostic, self-hosted, and has zero cloud dependency by default.

## Production Notice

MAGI is usable today but still evolving. Test in a staging environment, keep backups, and plan for rollback before relying on it for critical workloads.

---

## Quick Install

### Pre-built Binaries (GitHub Releases)

Every tagged release publishes pre-built binaries. Download the right one for your platform:

**Linux (amd64):**
```bash
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-linux-amd64 -o magi
chmod +x magi
sudo mv magi /usr/local/bin/
```

**Linux (arm64):**
```bash
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-linux-arm64 -o magi
chmod +x magi
sudo mv magi /usr/local/bin/
```

**macOS (Apple Silicon):**
```bash
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-darwin-arm64 -o magi
chmod +x magi
sudo mv magi /usr/local/bin/
```

**macOS (Intel):**
```bash
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-darwin-amd64 -o magi
chmod +x magi
sudo mv magi /usr/local/bin/
```

Releases also include companion tools: `magi-sync` and `mcp-config` (available for Linux, macOS, and Windows).

### Build From Source

Prereqs:

- Go 1.25+ with CGO enabled
- ONNX Runtime shared library installed

```bash
git clone https://github.com/j33pguy/magi
cd magi
CGO_ENABLED=1 make build
```

Binaries land in `bin/`: `magi` (server) and `magi-import` (markdown importer).

### Docker

```bash
MAGI_API_TOKEN=your-token docker compose up -d
curl http://localhost:8302/health
```

---

## Platform Setup

### Linux

#### ONNX Runtime

**Fedora/RHEL:**
```bash
dnf install onnxruntime
```

**Ubuntu/Debian:**
```bash
# Download from https://github.com/microsoft/onnxruntime/releases
tar xzf onnxruntime-linux-x64-*.tgz
sudo cp onnxruntime-linux-x64-*/lib/libonnxruntime.so* /usr/local/lib/
sudo ldconfig
```

#### Systemd Service

Create `/etc/systemd/system/magi.service`:

```ini
[Unit]
Description=MAGI memory server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=magi
Group=magi
ExecStart=/usr/local/bin/magi --http-only
WorkingDirectory=/opt/magi
Restart=on-failure
RestartSec=5

Environment=MEMORY_BACKEND=sqlite
Environment=SQLITE_PATH=/opt/magi/data/memory.db
Environment=MAGI_API_TOKEN=your-token-here
Environment=MAGI_MODEL_DIR=/opt/magi/data/models
Environment=MAGI_LEGACY_HTTP_PORT=8302
Environment=MAGI_UI_PORT=8080

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/magi
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
# Setup
sudo useradd -r -s /sbin/nologin magi
sudo mkdir -p /opt/magi/data/models
sudo chown -R magi:magi /opt/magi

# Enable
sudo systemctl daemon-reload
sudo systemctl enable --now magi

# Verify
sudo systemctl status magi
curl http://localhost:8302/health
```

### macOS

#### ONNX Runtime

```bash
brew install onnxruntime
```

#### Launch Agent (auto-start on login)

Create `~/Library/LaunchAgents/com.magi.server.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.magi.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/magi</string>
        <string>--http-only</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>MEMORY_BACKEND</key>
        <string>sqlite</string>
        <key>SQLITE_PATH</key>
        <string>/Users/YOU/.magi/memory.db</string>
        <key>MAGI_API_TOKEN</key>
        <string>your-token-here</string>
        <key>MAGI_MODEL_DIR</key>
        <string>/Users/YOU/.magi/models</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/magi.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/magi.err</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.magi.server.plist
curl http://localhost:8302/health
```

### Windows

> **Note:** The main `magi` server binary requires CGO (ONNX Runtime + SQLite). Native Windows builds are not yet available in GitHub Releases. You can run MAGI on Windows via **WSL2** or **Docker Desktop**.

#### Option 1: WSL2 (Recommended)

Install WSL2 with Ubuntu, then follow the Linux instructions:

```powershell
wsl --install -d Ubuntu
```

Inside WSL2:
```bash
# Download MAGI binary
curl -L https://github.com/j33pguy/magi/releases/latest/download/magi-linux-amd64 -o magi
chmod +x magi
sudo mv magi /usr/local/bin/

# Install ONNX Runtime
sudo apt-get update
wget https://github.com/microsoft/onnxruntime/releases/download/v1.21.1/onnxruntime-linux-x64-1.21.1.tgz
tar xzf onnxruntime-linux-x64-*.tgz
sudo cp onnxruntime-linux-x64-*/lib/libonnxruntime.so* /usr/local/lib/
sudo ldconfig

# Run
export MEMORY_BACKEND=sqlite
export MAGI_API_TOKEN=your-token
magi --http-only
```

The server is accessible from Windows at `http://localhost:8302`.

#### Option 2: Docker Desktop

Install [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/), then:

```powershell
git clone https://github.com/j33pguy/magi
cd magi
docker compose up -d
curl http://localhost:8302/health
```

#### Windows-Native Companion Tools

`magi-sync` and `mcp-config` are pure Go and **do** have native Windows binaries:

```powershell
# Download magi-sync for Windows
Invoke-WebRequest -Uri "https://github.com/j33pguy/magi/releases/latest/download/magi-sync-windows-amd64.exe" -OutFile "magi-sync.exe"

# Run interactive setup
.\magi-sync.exe init

# Point it at your MAGI server (WSL2, Docker, or remote Linux host)
```

---

## Companion Tools Setup

### magi-sync (Cross-Machine Memory Sync)

`magi-sync` watches local agent files and syncs them to a central MAGI server. Available for Linux, macOS, and Windows.

**Interactive setup (recommended):**
```bash
magi-sync init
```

The wizard auto-detects installed agents (Claude, OpenClaw, Codex) and writes the config to `~/.config/magi-sync/config.yaml`.

**Modes:**
| Mode | Description |
|------|-------------|
| `init` | Interactive setup wizard |
| `enroll` | Enroll this machine with the server |
| `check` | Verify config and server connectivity |
| `dry-run` | Show what would sync without uploading |
| `once` | Sync once and exit |
| `run` | Sync on interval (default 30s) |
| `watch` | Sync on file changes (fsnotify) |

**Systemd service for continuous sync:**
```ini
[Unit]
Description=magi-sync agent
After=network-online.target

[Service]
Type=simple
User=your-user
ExecStart=/usr/local/bin/magi-sync watch
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### mcp-config

Generate MCP client config for your agent:

```bash
mcp-config
# Outputs JSON config to paste into your agent's MCP settings
```

---

## Auto-Deploy (CI/CD)

### GitHub Actions Deploy Workflow

MAGI ships with a deploy workflow (`.github/workflows/deploy.yml`) that:
- **Auto-triggers** after a successful Release workflow
- **Manual dispatch** with a version override via `workflow_dispatch`
- Downloads the release binary and deploys via SSH

Set these GitHub secrets for auto-deploy:
| Secret | Description |
|--------|-------------|
| `MAGI_DEPLOY_HOST` | Target server hostname or IP |
| `MAGI_DEPLOY_USER` | SSH user on the target |
| `MAGI_DEPLOY_KEY_PATH` | Path to SSH private key on the runner |

### Ansible Role

The IaC repo includes an Ansible role (`ansible/roles/magi`) for idempotent deployments:

```bash
ansible-playbook playbooks/magi.yml
# Or with a specific version:
ansible-playbook playbooks/magi.yml -e magi_version=v0.3.10
```

The role:
- Downloads the release binary from GitHub
- Manages systemd service and drop-in overrides
- Fetches API token from HashiCorp Vault
- Configures firewall rules (Fedora/firewalld)
- Cleans up old `claude-memory` service if present

---

## Core Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `MEMORY_BACKEND` | `sqlite` | Storage backend: `sqlite`, `turso`, `postgres`, `mysql`, `sqlserver` |
| `SQLITE_PATH` | `~/.magi/memory-local.db` | SQLite file path |
| `POSTGRES_URL` | none | PostgreSQL connection string |
| `MYSQL_DSN` | none | MySQL/MariaDB DSN |
| `SQLSERVER_URL` | none | SQL Server DSN |
| `TURSO_URL` | none | Turso/libSQL connection string |
| `TURSO_AUTH_TOKEN` | none | Turso auth token |
| `MAGI_REPLICA_PATH` | `~/.magi/memory.db` | Local replica path |
| `MAGI_SYNC_INTERVAL` | `60s` | Replica sync interval |
| `MAGI_API_TOKEN` | empty | Admin bearer token (unset = read-only GETs only) |
| `MAGI_GRPC_PORT` | `8300` | gRPC server port |
| `MAGI_HTTP_PORT` | `8301` | gRPC gateway port |
| `MAGI_LEGACY_HTTP_PORT` | `8302` | Legacy REST API port |
| `MAGI_UI_PORT` | `8080` | Web UI port |
| `MAGI_UI_ENABLED` | `true` | Enable web UI |

## Performance Tuning

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_ASYNC_WRITES` | `false` | Enable async write pipeline |
| `MAGI_WRITE_WORKERS` | `NumCPU` | Async write worker count |
| `MAGI_WRITE_QUEUE_SIZE` | `1000` | Async write queue depth |
| `MAGI_BATCH_FLUSH_INTERVAL` | `100ms` | Batch flush interval |
| `MAGI_BATCH_MAX_SIZE` | `50` | Max batch size per flush |
| `MAGI_CACHE_ENABLED` | `false` | Enable hot caches |
| `MAGI_CACHE_QUERY_TTL` | `60s` | Query cache TTL |
| `MAGI_CACHE_MEMORY_SIZE` | `1000` | Memory cache size |
| `MAGI_CACHE_EMBEDDING_SIZE` | `5000` | Embedding cache size |
| `MAGI_MODEL_DIR` | `~/.magi/models` | ONNX model directory |
| `ONNXRUNTIME_LIB` | empty | Override ONNX Runtime library path |

## Auth & Secrets

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_MACHINE_TOKENS_JSON` | empty | Bootstrap machine tokens (JSON array) |
| `MAGI_MACHINE_TOKENS_FILE` | empty | Path to machine token JSON file |
| `MAGI_SECRET_MODE` | `reject` | `reject` or `externalize` |
| `MAGI_SECRET_BACKEND` | empty | Secret backend (e.g. `vault`) |
| `MAGI_VAULT_TOKEN` | empty | Vault token |
| `MAGI_VAULT_MOUNT` | `secret` | Vault KV mount |

## Git-Backed History

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_GIT_ENABLED` | `false` | Enable git versioning |
| `MAGI_GIT_PATH` | empty | Git repo path for memory history |
| `MAGI_GIT_COMMIT_MODE` | `immediate` | `immediate` or `batch` |
| `MAGI_GIT_BATCH_INTERVAL` | `60s` | Batch commit interval |

## Node Mesh (Optional)

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_NODE_MODE` | `embedded` | Communication mode |
| `MAGI_WRITER_POOL_SIZE` | `4` | Writer goroutine count |
| `MAGI_READER_POOL_SIZE` | `8` | Reader goroutine count |
| `MAGI_COORDINATOR_ENABLED` | `true` | Enable coordinator routing |

## Health Endpoints

| Endpoint | Auth | Purpose |
|----------|------|---------|
| `GET /health` | No | Full health (DB + git + memory count) |
| `GET /readyz` | No | Readiness (DB accessible) |
| `GET /livez` | No | Liveness (process alive) |

All on the legacy HTTP port (`MAGI_LEGACY_HTTP_PORT`, default 8302).

## Ports Summary

| Port | Protocol | Purpose |
|------|----------|---------|
| 8300 | gRPC | Native gRPC clients |
| 8301 | HTTP/JSON | gRPC-gateway reverse proxy |
| 8302 | HTTP/JSON | Legacy REST API + health endpoints |
| 8080 | HTTP | Web UI |

## Reverse Proxy

Example Traefik dynamic config:

```yaml
http:
  routers:
    magi-ui:
      rule: "Host(`memory.example.com`)"
      service: magi-ui
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt
      middlewares: [auth-middleware]

    magi-api:
      rule: "Host(`memory-api.example.com`)"
      service: magi-api
      entryPoints: [websecure]
      tls:
        certResolver: letsencrypt
      # API uses Bearer token auth

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

## Model Setup

The ONNX model (`all-MiniLM-L6-v2`) auto-downloads to `MAGI_MODEL_DIR` on first run. For air-gapped environments, place the model files (`model.onnx` + `tokenizer.json`) in that directory before starting.

## Deployment Guidance

- Keep MAGI private by default — expose only through a trusted network boundary
- If exposing publicly, use an authenticated reverse proxy and keep bearer tokens private
- Prefer MCP tools first; REST and gRPC mirror MCP functionality
- Back up your SQLite database regularly (`cp` while MAGI is running is safe with WAL mode)

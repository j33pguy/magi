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
# or copy it manually from ~/.magi/models/

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable magi
sudo systemctl start magi

# Check status
sudo systemctl status magi
journalctl -u magi -f
```

**Note:** The `--http-only` flag runs gRPC, grpc-gateway, legacy HTTP, and web UI servers without the stdio MCP server. MCP mode is for direct direct agent integration (where the binary is launched by an agent as a subprocess).

## Node Mesh Configuration

The distributed node mesh (PR #74) routes reads and writes through goroutine pools managed by a Coordinator. In embedded mode (Phase 1), all pools run in-process with zero serialization overhead.

| Env Var | Default | Description |
|---------|---------|-------------|
| `MAGI_NODE_MODE` | `embedded` | Communication mode. Phase 1 supports `embedded` only. |
| `MAGI_WRITER_POOL_SIZE` | `4` | Number of writer goroutines. Increase for write-heavy workloads. |
| `MAGI_READER_POOL_SIZE` | `8` | Number of reader goroutines. Increase for search-heavy workloads. |
| `MAGI_COORDINATOR_ENABLED` | `true` | Set to `false` to bypass the coordinator and use direct store access. |

Add to systemd environment:

```ini
Environment=MAGI_COORDINATOR_ENABLED=true
Environment=MAGI_WRITER_POOL_SIZE=4
Environment=MAGI_READER_POOL_SIZE=8
```

To disable the coordinator (direct store access, same as pre-v0.2.0 behavior):

```ini
Environment=MAGI_COORDINATOR_ENABLED=false
```

## Prometheus Monitoring

MAGI exposes a `/metrics` endpoint in Prometheus exposition format on the legacy HTTP port (default 8302).

### Scrape Config

Add to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'magi'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8302']
    metrics_path: /metrics
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `magi_write_latency_seconds` | Histogram | Memory write latency |
| `magi_search_latency_seconds` | Histogram | Memory search latency |
| `magi_embedding_duration_seconds` | Histogram | ONNX embedding duration |
| `magi_queue_depth` | Gauge | Async write pipeline depth |
| `magi_memory_count` | Gauge | Total memories in DB |
| `magi_active_sessions` | Gauge | Active MCP sessions |
| `magi_cache_hits_total` | Counter | Cache hits by type |
| `magi_cache_misses_total` | Counter | Cache misses by type |
| `magi_git_commits_total` | Counter | Git commits for versioning |

### Example Alert Rules

```yaml
groups:
  - name: magi
    rules:
      - alert: MAGIWriteLatencyHigh
        expr: histogram_quantile(0.95, rate(magi_write_latency_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MAGI write latency p95 > 1s"

      - alert: MAGIQueueBacklog
        expr: magi_queue_depth > 100
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "MAGI async write queue backlog"
```

## Kubernetes Health Probes

MAGI provides `/readyz` and `/livez` endpoints for Kubernetes probes. No authentication required.

### Pod Spec Example

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: magi
spec:
  containers:
    - name: magi
      image: ghcr.io/j33pguy/magi:latest
      ports:
        - containerPort: 8302
          name: http
        - containerPort: 8080
          name: ui
      env:
        - name: MEMORY_BACKEND
          value: postgres
        - name: MAGI_COORDINATOR_ENABLED
          value: "true"
        - name: MAGI_WRITER_POOL_SIZE
          value: "4"
        - name: MAGI_READER_POOL_SIZE
          value: "8"
      livenessProbe:
        httpGet:
          path: /livez
          port: http
        initialDelaySeconds: 5
        periodSeconds: 10
      readinessProbe:
        httpGet:
          path: /readyz
          port: http
        initialDelaySeconds: 10
        periodSeconds: 5
      resources:
        requests:
          memory: "256Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "1000m"
```

### Probe Behavior

| Probe | Endpoint | Checks | Use For |
|-------|----------|--------|---------|
| Liveness | `GET /livez` | Process alive (no deps) | Restart if process is wedged |
| Readiness | `GET /readyz` | Database accessible | Don't route traffic until DB is ready |
| Health | `GET /health` | DB + git + memory count | Dashboards, debugging |

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

The ONNX model (all-MiniLM-L6-v2) is auto-downloaded on first run to `MAGI_MODEL_DIR` (default `~/.magi/models/`). For air-gapped deployments, download the model files manually:

- `model.onnx` — the ONNX model
- `tokenizer.json` or `vocab.txt` — BERT WordPiece vocabulary

Place them in the model directory before starting the server.

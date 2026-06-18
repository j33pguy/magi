# MAGI v0.5.0 production upgrade runbook

## Current production shape
- Host: production MAGI host
- Service: `magi.service`
- Binary: `/opt/magi/bin/magi --http-only`
- Working dir: `/opt/magi`
- User/group: service account used by the deployment
- Backend: SQLite at `/opt/magi/data/memory.db`
- Git history enabled at `/opt/magi/data/git-memories`
- Ports: `8300` gRPC, `8301` gateway, `8302` legacy HTTP, `8080` UI
- Important env overrides: `MAGI_GIT_ENABLED=true`, `MAGI_TRUSTED_PROXY_AUTH=true`, `ONNXRUNTIME_LIB=/usr/lib64/libonnxruntime.so.1`, `ORT_DISABLE_CPU_AFFINITY=1`, `OMP_NUM_THREADS=1`

## Preconditions
- Release commit chosen and tested on staging.
- New binary built and checksum recorded.
- Quiet deploy window chosen.
- Rollback operator has SSH access through the existing jump path.

## Backup
1. SSH to prod.
2. Capture service state:
   - `systemctl status magi --no-pager -l`
   - `curl -H "Authorization: Bearer <token>" http://127.0.0.1:8302/health`
3. Create timestamped backups:
   - `cp -a /opt/magi/bin/magi /opt/magi/bin/magi.bak-$(date +%Y%m%d_%H%M%S)`
   - `cp -a /opt/magi/data/memory.db /opt/magi/data/memory.db.bak-$(date +%Y%m%d_%H%M%S)`
   - `tar -C /opt/magi/data -czf /opt/magi/data/git-memories.bak-$(date +%Y%m%d_%H%M%S).tgz git-memories`

## Deploy
1. Copy the new binary to a temp path, for example `/opt/magi/bin/magi.new`.
2. Install it in place preserving executable permissions:
   - `install -m 0755 -o <service-user> -g <service-group> /opt/magi/bin/magi.new /opt/magi/bin/magi`
3. Restart service:
   - `systemctl restart magi`
4. Confirm service is up:
   - `systemctl is-active --quiet magi`
   - `systemctl status magi --no-pager -l | sed -n '1,40p'`

## Validation
1. Health:
   - `curl -H "Authorization: Bearer <token>" http://127.0.0.1:8302/health`
2. Listener check:
   - `ss -ltnp | grep -E ':(8080|8300|8301|8302)'`
3. Smoke query:
   - `curl -H "Authorization: Bearer <token>" 'http://127.0.0.1:8302/search?q=magi&top_k=3'`
4. Safe write test:
   - `curl -X POST http://127.0.0.1:8302/remember -H "Authorization: Bearer <token>" -H 'Content-Type: application/json' -d '{"content":"v0.5.0 rollout smoke test","project":"ops","type":"note","tags":["rollout","v0.5.0"]}'`
5. UI check through the normal reverse proxy path.
6. Watch logs for 5 to 10 minutes:
   - `journalctl -u magi -n 200 -f`

## Rollback
If health, search, remember, or UI regress:
1. Restore previous binary:
   - `cp -a /opt/magi/bin/magi.bak-<timestamp> /opt/magi/bin/magi`
2. Restart service:
   - `systemctl restart magi`
3. Re-check health and listeners.
4. Restore DB or git history only if the failure was data-corrupting, not merely binary/runtime related.

## Known caution from current staging rollout
- The legacy async `/remember` context gap has been fixed in code. Keep one explicit pre-prod verification pass on staging to confirm async writes now populate `memory_contexts.repository_id` and provenance fields alongside repo facet tags before rolling v0.5.0 to production.
- See `docs/persistence-and-rebuild.md` for the current git-backed vs rebuildable persistence contract.

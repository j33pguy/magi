# unifi-mcp

MCP server for a UniFi Dream Machine Pro. Writes go through Terraform (so every change is plannable, diffable, and rollbackable); reads go directly against the local UniFi Network Integration API for live state.

## What it does

- **Terraform tools** — generate `.tf` files for networks, WLANs, firewall rules/groups, port forwards, users, and user groups; run `init`/`validate`/`plan`/`apply`/`destroy` from inside an MCP session.
- **Two-step apply** — `tf_plan` returns a `confirm_token`; `tf_apply` requires that exact token (or `UNIFI_MCP_AUTO_APPROVE=true` to skip).
- **Read-only API tools** — fetch sites, devices, clients, and controller info from the local UniFi API.

## Provider note

The original `paultyng/unifi` provider was archived in 2025. This server defaults to the actively-maintained community fork **`ubiquiti-community/unifi`** (resource syntax is identical). If you want to pin back to `paultyng/unifi`, edit `terraform/versions.tf` after first run.

## Requirements

- UniFi Network application **v9.3.43+** on the UDM Pro (for the Integration v1 read API).
- A controller user account with admin rights (Terraform writes go through this account).
- A Network API key for read-only calls — Network → Settings → Control Plane → Integrations.
- Python 3.11+.
- Terraform 1.5+ on `PATH`.

## Install

```bash
cd unifi-mcp
python -m venv .venv && source .venv/bin/activate
pip install -e .
```

## Configure

Copy `.env.example` to `.env` and fill in your UDM Pro details. The same env vars are read by both the MCP server and the Terraform provider.

| Variable | Purpose |
|---|---|
| `UNIFI_API` | Controller URL, e.g. `https://192.168.1.1` |
| `UNIFI_USERNAME` / `UNIFI_PASSWORD` | Local admin used by the Terraform provider |
| `UNIFI_SITE` | Defaults to `default` |
| `UNIFI_INSECURE` | `true` for self-signed certs (typical UDM Pro setup) |
| `UNIFI_API_KEY` | X-API-KEY for the read-only Integration v1 API |
| `UNIFI_MCP_TF_DIR` | Where the server keeps `.tf` files |
| `TERRAFORM_BIN` | Override the `terraform` binary if needed |
| `UNIFI_MCP_AUTO_APPROVE` | `true` to bypass the confirm-token gate on `tf_apply` |

## Wire it into Claude Code / Codex

Add to `~/.claude.json` (or your MCP client's equivalent):

```json
{
  "mcpServers": {
    "unifi": {
      "command": "unifi-mcp",
      "env": {
        "UNIFI_API": "https://192.168.1.1",
        "UNIFI_USERNAME": "tf-admin",
        "UNIFI_PASSWORD": "...",
        "UNIFI_SITE": "default",
        "UNIFI_INSECURE": "true",
        "UNIFI_API_KEY": "...",
        "UNIFI_MCP_TF_DIR": "/home/you/.unifi-mcp/terraform"
      }
    }
  }
}
```

## Typical flow

1. `tf_init` (once after install).
2. `upsert_network(name="iot", purpose="corporate", subnet="10.0.30.1/24", vlan_id=30)`.
3. `tf_plan` — review the structured `resource_changes` and grab the `confirm_token`.
4. `tf_apply(confirm_token="...")`.
5. `list_clients(site_id=...)` to verify devices land on the new VLAN.

## Tools

### Terraform lifecycle
- `tf_init`, `tf_validate`, `tf_plan`, `tf_apply`, `tf_destroy`
- `tf_state_list`, `tf_show_state`, `tf_list_files`, `tf_read_file`

### UniFi resources (write — produce `.tf` files)
- `upsert_network` / `delete_network`
- `upsert_wlan` / `delete_wlan`
- `upsert_firewall_group` / `delete_firewall_group`
- `upsert_firewall_rule` / `delete_firewall_rule`
- `upsert_port_forward` / `delete_port_forward`
- `upsert_user` / `delete_user`
- `upsert_user_group` / `delete_user_group`

### UniFi live state (read — direct API)
- `api_info`, `list_sites`, `list_devices`, `get_device`, `list_clients`, `get_client`

## Safety

- `tf_apply` saves a plan file (`tfplan`) in `tf_plan` and reuses it, so apply matches the diff you reviewed.
- `tf_apply` rejects calls without a fresh `confirm_token` (unless auto-approve is on).
- `tf_destroy` requires the literal string `confirm="DESTROY"`.
- The Terraform working tree is **state-bearing** — back it up. State files are excluded from git via the bootstrap `.gitignore`.

## License

MIT.

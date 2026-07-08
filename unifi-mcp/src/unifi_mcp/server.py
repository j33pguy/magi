"""MCP server entry point — exposes UniFi tools backed by Terraform + the local API."""

from __future__ import annotations

import json
import secrets
from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

from .bootstrap import ensure_bootstrap
from .config import Config, load
from .hcl import render_resource, resource_filename
from .terraform import Terraform
from .unifi_api import UniFiClient

# --- module-level state, initialized in main() ---------------------------
_cfg: Config | None = None
_tf: Terraform | None = None
_uapi: UniFiClient | None = None
_pending_apply_token: str | None = None


def _config() -> Config:
    if _cfg is None:
        raise RuntimeError("server not initialized")
    return _cfg


def _tform() -> Terraform:
    if _tf is None:
        raise RuntimeError("terraform runner not initialized")
    return _tf


def _api() -> UniFiClient:
    if _uapi is None:
        raise RuntimeError(
            "UniFi API client not initialized — set UNIFI_API_KEY to enable read tools"
        )
    return _uapi


# --- MCP app -------------------------------------------------------------
mcp = FastMCP("unifi-mcp")


# ============== Resource: Terraform working tree ==============

@mcp.resource("unifi-tf://working-dir")
def working_dir_resource() -> str:
    """Inventory of the Terraform working directory (file names + sizes)."""
    cfg = _config()
    if not cfg.terraform_dir.exists():
        return f"(empty) {cfg.terraform_dir}"
    files = []
    for p in sorted(cfg.terraform_dir.iterdir()):
        if p.is_file() and p.suffix in {".tf", ".tfvars", ".hcl"}:
            files.append(f"{p.name}\t{p.stat().st_size}B")
    return f"# {cfg.terraform_dir}\n" + ("\n".join(files) if files else "(no .tf files)")


# ============== Tool group: Terraform lifecycle ==============

@mcp.tool()
async def tf_init(upgrade: bool = False) -> str:
    """Run `terraform init` in the working directory.

    Call this once after first setup, or with upgrade=True to refresh providers.
    """
    res = await _tform().init(upgrade=upgrade)
    return res.summary()


@mcp.tool()
async def tf_validate() -> str:
    """Run `terraform validate` (JSON output) to check syntax of the working tree."""
    res = await _tform().validate()
    return res.summary()


@mcp.tool()
async def tf_plan() -> dict[str, Any]:
    """Generate a Terraform plan and return a structured summary.

    Detailed exit codes: 0 = no changes, 1 = error, 2 = changes pending.
    A plan file is saved to `tfplan` so a subsequent tf_apply uses *exactly*
    the plan you reviewed.
    """
    global _pending_apply_token
    tf = _tform()
    plan = await tf.plan()

    summary: dict[str, Any] = {
        "exit_code": plan.rc,
        "duration_s": round(plan.duration_s, 2),
        "stdout_tail": (plan.stdout or "")[-2000:],
        "stderr_tail": (plan.stderr or "")[-1000:],
    }

    if plan.rc == 1:
        _pending_apply_token = None
        summary["status"] = "error"
        return summary
    if plan.rc == 0:
        _pending_apply_token = None
        summary["status"] = "no_changes"
        return summary

    # rc == 2: changes pending — surface a structured diff
    try:
        show = await tf.show_json(plan_file="tfplan")
        rc = show.get("resource_changes", []) or []
        actions: dict[str, list[str]] = {}
        for change in rc:
            for action in change.get("change", {}).get("actions", []):
                actions.setdefault(action, []).append(change.get("address", "?"))
        summary["resource_changes"] = {a: addrs for a, addrs in actions.items()}
    except Exception as e:
        summary["resource_changes_error"] = str(e)

    _pending_apply_token = secrets.token_urlsafe(8)
    summary["status"] = "changes_pending"
    summary["confirm_token"] = _pending_apply_token
    summary["next_step"] = (
        "Review resource_changes, then call tf_apply(confirm_token=...) "
        "to apply this exact plan."
    )
    return summary


@mcp.tool()
async def tf_apply(confirm_token: str | None = None) -> str:
    """Apply the most recent plan.

    Requires the confirm_token returned by tf_plan, unless UNIFI_MCP_AUTO_APPROVE=1.
    """
    global _pending_apply_token
    cfg = _config()
    if not cfg.auto_approve:
        if not _pending_apply_token:
            return "ERROR: no pending plan. Run tf_plan first."
        if confirm_token != _pending_apply_token:
            return (
                "ERROR: confirm_token mismatch. Re-run tf_plan and pass the "
                "fresh confirm_token to tf_apply."
            )
    res = await _tform().apply(plan_file="tfplan")
    _pending_apply_token = None
    return res.summary()


@mcp.tool()
async def tf_destroy(confirm: str = "") -> str:
    """Destroy ALL Terraform-managed UniFi resources. Pass confirm='DESTROY' to proceed."""
    if confirm != "DESTROY":
        return "ERROR: destructive operation. Pass confirm='DESTROY' to proceed."
    res = await _tform().destroy()
    return res.summary()


@mcp.tool()
async def tf_state_list() -> str:
    """List addresses of all resources in the Terraform state."""
    res = await _tform().state_list()
    return res.summary()


@mcp.tool()
async def tf_show_state() -> dict[str, Any]:
    """Return the current Terraform state as parsed JSON."""
    return await _tform().show_json()


@mcp.tool()
async def tf_list_files() -> list[str]:
    """List .tf files in the working directory."""
    cfg = _config()
    return sorted(p.name for p in cfg.terraform_dir.glob("*.tf"))


@mcp.tool()
async def tf_read_file(filename: str) -> str:
    """Read a .tf file from the working directory by name (no path traversal)."""
    cfg = _config()
    if "/" in filename or filename.startswith("."):
        return "ERROR: invalid filename"
    p = cfg.terraform_dir / filename
    if not p.exists() or not p.is_file():
        return f"ERROR: {filename} not found"
    return p.read_text()


# ============== Helpers for write tools ==============

def _write_resource(resource_type: str, name: str, attrs: dict[str, Any]) -> str:
    cfg = _config()
    body = render_resource(resource_type, name, attrs)
    path = cfg.terraform_dir / resource_filename(resource_type, name)
    path.write_text(body)
    return str(path.relative_to(cfg.terraform_dir))


def _delete_resource(resource_type: str, name: str) -> str:
    cfg = _config()
    path = cfg.terraform_dir / resource_filename(resource_type, name)
    if not path.exists():
        return f"ERROR: {path.name} not found"
    path.unlink()
    return f"deleted {path.name}"


# ============== Tool group: Networks (VLANs / subnets) ==============

@mcp.tool()
async def upsert_network(
    name: str,
    purpose: str = "corporate",
    subnet: str | None = None,
    vlan_id: int | None = None,
    dhcp_enabled: bool = True,
    dhcp_start: str | None = None,
    dhcp_stop: str | None = None,
    domain_name: str | None = None,
    site: str | None = None,
) -> str:
    """Create or update a UniFi network (VLAN). Writes a .tf file; run tf_plan to preview.

    - purpose: "corporate" | "guest" | "wan" | "vlan-only"
    - subnet:  CIDR e.g. "10.0.20.1/24"
    - vlan_id: 2-4009
    """
    attrs: dict[str, Any] = {"name": name, "purpose": purpose}
    if subnet is not None:
        attrs["subnet"] = subnet
    if vlan_id is not None:
        attrs["vlan_id"] = vlan_id
    attrs["dhcp_enabled"] = dhcp_enabled
    if dhcp_start:
        attrs["dhcp_start"] = dhcp_start
    if dhcp_stop:
        attrs["dhcp_stop"] = dhcp_stop
    if domain_name:
        attrs["domain_name"] = domain_name
    if site:
        attrs["site"] = site
    return _write_resource("unifi_network", name, attrs)


@mcp.tool()
async def delete_network(name: str) -> str:
    """Remove a network's .tf file. Run tf_plan/tf_apply to actually destroy it."""
    return _delete_resource("unifi_network", name)


# ============== Tool group: WLANs ==============

@mcp.tool()
async def upsert_wlan(
    name: str,
    ssid: str,
    network_name: str,
    passphrase: str | None = None,
    security: str = "wpapsk",
    is_guest: bool = False,
    user_group_name: str | None = None,
    ap_group_names: list[str] | None = None,
    site: str | None = None,
) -> str:
    """Create or update a wireless network.

    - security: "open" | "wep" | "wpapsk" | "wpaeap"
    - network_name / user_group_name / ap_group_names refer to other Terraform
      resources by their local name; the .tf file uses interpolation.
    """
    attrs: dict[str, Any] = {
        "name": name,
        "security": security,
        "is_guest": is_guest,
        # Reference another resource by its local name.
        "network_id": f"${{unifi_network.{network_name}.id}}",
    }
    # SSID is a separate field; some forks call it "name" only — set both safely.
    # The provider treats "name" as the SSID when no separate field exists.
    if ssid != name:
        # most builds expose ssid via the same `name`; if a fork separates them
        # the user can hand-edit the file.
        attrs["name"] = ssid
    if passphrase:
        attrs["passphrase"] = passphrase
    if user_group_name:
        attrs["user_group_id"] = f"${{unifi_user_group.{user_group_name}.id}}"
    if ap_group_names:
        attrs["ap_group_ids"] = [
            f"${{unifi_ap_group.{n}.id}}" for n in ap_group_names
        ]
    if site:
        attrs["site"] = site
    return _write_resource("unifi_wlan", name, attrs)


@mcp.tool()
async def delete_wlan(name: str) -> str:
    return _delete_resource("unifi_wlan", name)


# ============== Tool group: Firewall rules + groups ==============

@mcp.tool()
async def upsert_firewall_group(
    name: str,
    group_type: str,
    members: list[str],
    site: str | None = None,
) -> str:
    """Create/update a firewall group used by firewall rules.

    - group_type: "address-group" | "port-group" | "ipv6-address-group"
    - members:    list of IPs/CIDRs (or ports as strings)
    """
    attrs: dict[str, Any] = {"name": name, "type": group_type, "members": members}
    if site:
        attrs["site"] = site
    return _write_resource("unifi_firewall_group", name, attrs)


@mcp.tool()
async def delete_firewall_group(name: str) -> str:
    return _delete_resource("unifi_firewall_group", name)


@mcp.tool()
async def upsert_firewall_rule(
    name: str,
    action: str,
    ruleset: str,
    rule_index: int,
    protocol: str = "all",
    src_address: str | None = None,
    dst_address: str | None = None,
    dst_port: str | None = None,
    src_firewall_group_names: list[str] | None = None,
    dst_firewall_group_names: list[str] | None = None,
    enabled: bool = True,
    logging: bool = False,
    site: str | None = None,
) -> str:
    """Create/update a firewall rule.

    - action:  "accept" | "drop" | "reject"
    - ruleset: one of WAN_IN/WAN_OUT/WAN_LOCAL, LAN_IN/LAN_OUT/LAN_LOCAL,
      GUEST_IN/GUEST_OUT/GUEST_LOCAL, or any of their IPv6 variants.
    - rule_index: 2000-2999 for user rules
    """
    attrs: dict[str, Any] = {
        "name": name,
        "action": action,
        "ruleset": ruleset,
        "rule_index": rule_index,
        "protocol": protocol,
        "enabled": enabled,
        "logging": logging,
    }
    if src_address:
        attrs["src_address"] = src_address
    if dst_address:
        attrs["dst_address"] = dst_address
    if dst_port:
        attrs["dst_port"] = dst_port
    if src_firewall_group_names:
        attrs["src_firewall_group_ids"] = [
            f"${{unifi_firewall_group.{n}.id}}" for n in src_firewall_group_names
        ]
    if dst_firewall_group_names:
        attrs["dst_firewall_group_ids"] = [
            f"${{unifi_firewall_group.{n}.id}}" for n in dst_firewall_group_names
        ]
    if site:
        attrs["site"] = site
    return _write_resource("unifi_firewall_rule", name, attrs)


@mcp.tool()
async def delete_firewall_rule(name: str) -> str:
    return _delete_resource("unifi_firewall_rule", name)


# ============== Tool group: Port forwards ==============

@mcp.tool()
async def upsert_port_forward(
    name: str,
    fwd_ip: str,
    fwd_port: str,
    dst_port: str,
    protocol: str = "tcp_udp",
    src_ip: str = "any",
    enabled: bool = True,
    log: bool = False,
    site: str | None = None,
) -> str:
    """Create/update a WAN→LAN port forward.

    - protocol: "tcp" | "udp" | "tcp_udp"
    - fwd_port / dst_port: single port or range like "8000-8010"
    """
    attrs: dict[str, Any] = {
        "name": name,
        "fwd_ip": fwd_ip,
        "fwd_port": fwd_port,
        "dst_port": dst_port,
        "protocol": protocol,
        "src_ip": src_ip,
        "enabled": enabled,
        "log": log,
    }
    if site:
        attrs["site"] = site
    return _write_resource("unifi_port_forward", name, attrs)


@mcp.tool()
async def delete_port_forward(name: str) -> str:
    return _delete_resource("unifi_port_forward", name)


# ============== Tool group: Users (static DHCP / blocks) ==============

@mcp.tool()
async def upsert_user(
    name: str,
    mac: str,
    fixed_ip: str | None = None,
    network_name: str | None = None,
    user_group_name: str | None = None,
    blocked: bool = False,
    note: str | None = None,
    site: str | None = None,
) -> str:
    """Manage a known client (static reservation, group assignment, block).

    `name` is the Terraform resource name (slug). `mac` is the MAC address.
    """
    attrs: dict[str, Any] = {"name": name, "mac": mac, "blocked": blocked}
    if fixed_ip:
        attrs["fixed_ip"] = fixed_ip
    if network_name:
        attrs["network_id"] = f"${{unifi_network.{network_name}.id}}"
    if user_group_name:
        attrs["user_group_id"] = f"${{unifi_user_group.{user_group_name}.id}}"
    if note:
        attrs["note"] = note
    if site:
        attrs["site"] = site
    return _write_resource("unifi_user", name, attrs)


@mcp.tool()
async def delete_user(name: str) -> str:
    return _delete_resource("unifi_user", name)


@mcp.tool()
async def upsert_user_group(
    name: str,
    qos_rate_max_down: int = -1,
    qos_rate_max_up: int = -1,
    site: str | None = None,
) -> str:
    """Manage a user/client bandwidth group. -1 means unlimited."""
    attrs: dict[str, Any] = {
        "name": name,
        "qos_rate_max_down": qos_rate_max_down,
        "qos_rate_max_up": qos_rate_max_up,
    }
    if site:
        attrs["site"] = site
    return _write_resource("unifi_user_group", name, attrs)


@mcp.tool()
async def delete_user_group(name: str) -> str:
    return _delete_resource("unifi_user_group", name)


# ============== Tool group: Live read-only state (UniFi Network API v1) ==============

@mcp.tool()
async def api_info() -> dict[str, Any]:
    """Get controller version + application info from the local UniFi API.

    Use this to confirm the controller is reachable and at version >= 9.3.43.
    """
    return await _api().info()


@mcp.tool()
async def list_sites(limit: int = 25, offset: int = 0) -> dict[str, Any]:
    """List sites visible to the local UniFi controller."""
    return await _api().list_sites(limit=limit, offset=offset)


@mcp.tool()
async def list_devices(site_id: str, limit: int = 25, offset: int = 0) -> dict[str, Any]:
    """List adopted UniFi devices in a site (UDM Pro itself, switches, APs, etc)."""
    return await _api().list_devices(site_id, limit=limit, offset=offset)


@mcp.tool()
async def get_device(site_id: str, device_id: str) -> dict[str, Any]:
    """Fetch detail (model, firmware, uptime, port stats) for a single device."""
    return await _api().get_device(site_id, device_id)


@mcp.tool()
async def list_clients(site_id: str, limit: int = 25, offset: int = 0) -> dict[str, Any]:
    """List currently-connected clients for a site."""
    return await _api().list_clients(site_id, limit=limit, offset=offset)


@mcp.tool()
async def get_client(site_id: str, client_id: str) -> dict[str, Any]:
    """Fetch detail for a single client by id."""
    return await _api().get_client(site_id, client_id)


# --- entry point ---------------------------------------------------------

def _format_initialization_summary(
    cfg: Config, written: list[Path], api_enabled: bool
) -> str:
    return json.dumps(
        {
            "terraform_dir": str(cfg.terraform_dir),
            "bootstrap_files": [str(p) for p in written],
            "site": cfg.unifi_site,
            "api_url": cfg.unifi_api_url,
            "read_api_enabled": api_enabled,
            "auto_approve": cfg.auto_approve,
        },
        indent=2,
    )


def main() -> None:
    """CLI entry point — initializes state, then runs the stdio MCP server."""
    global _cfg, _tf, _uapi

    _cfg = load()

    written = ensure_bootstrap(_cfg.terraform_dir)

    _tf = Terraform(
        _cfg.terraform_dir,
        binary=_cfg.terraform_bin,
        env={
            "UNIFI_USERNAME": _cfg.unifi_username,
            "UNIFI_PASSWORD": _cfg.unifi_password,
            "UNIFI_API": _cfg.unifi_api_url,
            "UNIFI_SITE": _cfg.unifi_site,
            "UNIFI_INSECURE": "true" if _cfg.unifi_insecure else "false",
        },
    )

    if _cfg.unifi_api_key:
        _uapi = UniFiClient(
            _cfg.unifi_api_url,
            _cfg.unifi_api_key,
            verify_tls=not _cfg.unifi_insecure,
        )

    # Visible only on stderr; FastMCP uses stdout for JSON-RPC.
    import sys

    print(
        "[unifi-mcp] starting\n" + _format_initialization_summary(_cfg, written, _uapi is not None),
        file=sys.stderr,
    )

    mcp.run(transport="stdio")


if __name__ == "__main__":
    main()

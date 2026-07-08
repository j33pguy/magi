"""Runtime configuration loaded from environment variables."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Config:
    # UniFi local controller (used for read-only API + Terraform provider)
    unifi_api_url: str
    unifi_username: str
    unifi_password: str
    unifi_site: str
    unifi_insecure: bool
    unifi_api_key: str | None  # X-API-KEY for the Integration v1 read API

    # Where the MCP server keeps the Terraform working tree
    terraform_dir: Path
    terraform_bin: str

    # Safety
    auto_approve: bool  # if False, apply requires an explicit confirm token


def _bool(name: str, default: bool = False) -> bool:
    raw = os.environ.get(name)
    if raw is None:
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def load() -> Config:
    api = os.environ.get("UNIFI_API", "").rstrip("/")
    if not api:
        raise RuntimeError(
            "UNIFI_API is required (e.g. https://192.168.1.1 for a UDM Pro)"
        )
    tf_dir = Path(os.environ.get("UNIFI_MCP_TF_DIR", "./terraform")).expanduser().resolve()
    return Config(
        unifi_api_url=api,
        unifi_username=os.environ.get("UNIFI_USERNAME", ""),
        unifi_password=os.environ.get("UNIFI_PASSWORD", ""),
        unifi_site=os.environ.get("UNIFI_SITE", "default"),
        unifi_insecure=_bool("UNIFI_INSECURE", default=True),
        unifi_api_key=os.environ.get("UNIFI_API_KEY") or None,
        terraform_dir=tf_dir,
        terraform_bin=os.environ.get("TERRAFORM_BIN", "terraform"),
        auto_approve=_bool("UNIFI_MCP_AUTO_APPROVE", default=False),
    )

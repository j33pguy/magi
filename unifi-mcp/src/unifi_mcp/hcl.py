"""Tiny HCL writer for the resource blocks this server creates.

We only emit a fixed shape (provider, variables, resources keyed by Terraform
address), so a full HCL parser is overkill — but we use python-hcl2 for *reads*
when we need to round-trip files.
"""

from __future__ import annotations

import re
from typing import Any

_IDENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_-]*$")


def _format_value(v: Any, indent: int = 2) -> str:
    pad = " " * indent
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    if v is None:
        return "null"
    if isinstance(v, str):
        # Heredoc for multi-line, escaped string for single-line.
        if "\n" in v:
            return "<<-EOT\n" + v + "\nEOT"
        escaped = v.replace("\\", "\\\\").replace('"', '\\"')
        return f'"{escaped}"'
    if isinstance(v, list):
        if not v:
            return "[]"
        inner = ",\n".join(f"{pad}  {_format_value(x, indent + 2)}" for x in v)
        return "[\n" + inner + f"\n{pad}]"
    if isinstance(v, dict):
        return _format_block(v, indent)
    raise TypeError(f"Cannot encode {type(v).__name__} to HCL: {v!r}")


def _format_block(d: dict[str, Any], indent: int = 2) -> str:
    pad = " " * indent
    lines = ["{"]
    for k, val in d.items():
        if not _IDENT.match(k):
            raise ValueError(f"Invalid HCL identifier: {k!r}")
        lines.append(f"{pad}  {k} = {_format_value(val, indent + 2)}")
    lines.append(f"{pad}}}")
    return "\n".join(lines)


def render_resource(resource_type: str, name: str, attrs: dict[str, Any]) -> str:
    """Render `resource "<type>" "<name>" { ... }` with proper formatting.

    Nested dicts become nested blocks (HCL block syntax), not assignments.
    Lists of dicts become repeated blocks.
    """
    if not _IDENT.match(resource_type):
        raise ValueError(f"Invalid resource type: {resource_type!r}")
    if not _IDENT.match(name):
        raise ValueError(f"Invalid resource name: {name!r}")
    body = _render_body(attrs, indent=2)
    return f'resource "{resource_type}" "{name}" {{\n{body}\n}}\n'


def _render_body(attrs: dict[str, Any], indent: int) -> str:
    pad = " " * indent
    lines: list[str] = []
    for k, v in attrs.items():
        if not _IDENT.match(k):
            raise ValueError(f"Invalid attribute: {k!r}")
        if isinstance(v, dict):
            inner = _render_body(v, indent + 2)
            lines.append(f"{pad}{k} {{\n{inner}\n{pad}}}")
        elif isinstance(v, list) and v and all(isinstance(x, dict) for x in v):
            for item in v:
                inner = _render_body(item, indent + 2)
                lines.append(f"{pad}{k} {{\n{inner}\n{pad}}}")
        else:
            lines.append(f"{pad}{k} = {_format_value(v, indent)}")
    return "\n".join(lines)


def resource_filename(resource_type: str, name: str) -> str:
    """File on disk for a given resource — one resource per file for clean diffs."""
    return f"{resource_type}.{name}.tf"

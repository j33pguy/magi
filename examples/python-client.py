#!/usr/bin/env python3
"""Basic MAGI HTTP client example using requests.

Shows remember, recall, and search with token auth and error handling.
"""

import json
import os
import sys
from typing import Any, Dict

import requests

BASE_URL = os.getenv("MAGI_HTTP_URL", "http://localhost:8302")
API_TOKEN = os.getenv("MAGI_API_TOKEN", "")
TIMEOUT = float(os.getenv("MAGI_HTTP_TIMEOUT", "10"))


class MagiError(RuntimeError):
    pass


def _headers() -> Dict[str, str]:
    headers = {"Content-Type": "application/json"}
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    return headers


def _request(method: str, path: str, *, json_body: Dict[str, Any] | None = None) -> Any:
    url = f"{BASE_URL}{path}"
    try:
        resp = requests.request(method, url, headers=_headers(), json=json_body, timeout=TIMEOUT)
    except requests.RequestException as exc:
        raise MagiError(f"request failed: {exc}") from exc

    if resp.status_code >= 400:
        try:
            detail = resp.json()
        except ValueError:
            detail = resp.text.strip()
        raise MagiError(f"{method} {path} failed ({resp.status_code}): {detail}")

    if resp.content:
        try:
            return resp.json()
        except ValueError as exc:
            raise MagiError(f"invalid JSON response from {path}") from exc
    return None


def remember() -> str:
    payload = {
        "content": "API v3 deprecates /users, use /people instead",
        "type": "decision",
        "speaker": "agent-a",
        "project": "platform",
        "tags": ["api", "migration"],
    }
    resp = _request("POST", "/remember", json_body=payload)
    return resp.get("id", "")


def recall() -> Any:
    payload = {"query": "API v3 migration", "top_k": 5, "project": "platform"}
    return _request("POST", "/recall", json_body=payload)


def search() -> Any:
    # /search is GET with query params; encode via requests for clarity.
    params = {
        "q": "API v3 migration",
        "project": "platform",
        "top_k": 5,
    }
    try:
        resp = requests.get(
            f"{BASE_URL}/search",
            headers=_headers(),
            params=params,
            timeout=TIMEOUT,
        )
    except requests.RequestException as exc:
        raise MagiError(f"request failed: {exc}") from exc

    if resp.status_code >= 400:
        try:
            detail = resp.json()
        except ValueError:
            detail = resp.text.strip()
        raise MagiError(f"GET /search failed ({resp.status_code}): {detail}")

    return resp.json()


def main() -> int:
    if not API_TOKEN:
        print("MAGI_API_TOKEN is not set; remember calls will fail if server enforces auth.", file=sys.stderr)

    try:
        mem_id = remember()
        print(f"remembered: {mem_id}")

        recall_resp = recall()
        print("recall results:")
        print(json.dumps(recall_resp, indent=2))

        search_resp = search()
        print("search results:")
        print(json.dumps(search_resp, indent=2))
    except MagiError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())

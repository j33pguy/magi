"""Async client for the UniFi Network Integration API (v1).

Targets the local controller on a UDM Pro:
    https://<UDM_IP>/proxy/network/integration/v1/...

Auth: X-API-KEY header. Read-only as of mid-2026; write scope rolling out.
Requires UniFi Network v9.3.43+. See https://developer.ui.com/.
"""

from __future__ import annotations

from typing import Any

import httpx


class UniFiAPIError(RuntimeError):
    pass


class UniFiClient:
    """Thin async wrapper over the local Integration v1 endpoints."""

    def __init__(
        self,
        base_url: str,
        api_key: str,
        *,
        verify_tls: bool = False,
        timeout: float = 15.0,
    ) -> None:
        if not api_key:
            raise UniFiAPIError(
                "UNIFI_API_KEY is required for read-only API calls. "
                "Generate one in Network → Settings → Control Plane → Integrations."
            )
        self._base = f"{base_url.rstrip('/')}/proxy/network/integration/v1"
        self._client = httpx.AsyncClient(
            headers={
                "X-API-KEY": api_key,
                "Accept": "application/json",
            },
            verify=verify_tls,
            timeout=timeout,
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def _get(self, path: str, **params: Any) -> Any:
        url = f"{self._base}{path}"
        clean = {k: v for k, v in params.items() if v is not None}
        try:
            r = await self._client.get(url, params=clean)
        except httpx.HTTPError as e:
            raise UniFiAPIError(f"GET {url} failed: {e}") from e
        if r.status_code == 429:
            raise UniFiAPIError("Rate limited by UniFi API (HTTP 429)")
        if r.status_code >= 400:
            raise UniFiAPIError(f"GET {url} → HTTP {r.status_code}: {r.text[:300]}")
        return r.json()

    # --- endpoints -----------------------------------------------------

    async def info(self) -> dict[str, Any]:
        """Controller version + application info (use to verify >= 9.3.43)."""
        return await self._get("/info")

    async def list_sites(self, *, limit: int = 25, offset: int = 0) -> dict[str, Any]:
        return await self._get("/sites", limit=limit, offset=offset)

    async def list_devices(
        self, site_id: str, *, limit: int = 25, offset: int = 0
    ) -> dict[str, Any]:
        return await self._get(f"/sites/{site_id}/devices", limit=limit, offset=offset)

    async def get_device(self, site_id: str, device_id: str) -> dict[str, Any]:
        return await self._get(f"/sites/{site_id}/devices/{device_id}")

    async def list_clients(
        self, site_id: str, *, limit: int = 25, offset: int = 0
    ) -> dict[str, Any]:
        return await self._get(f"/sites/{site_id}/clients", limit=limit, offset=offset)

    async def get_client(self, site_id: str, client_id: str) -> dict[str, Any]:
        return await self._get(f"/sites/{site_id}/clients/{client_id}")

    async def list_voucher_codes(
        self, site_id: str, *, limit: int = 25, offset: int = 0
    ) -> dict[str, Any]:
        return await self._get(f"/sites/{site_id}/hotspot/vouchers", limit=limit, offset=offset)

    async def site_application_info(self, site_id: str) -> dict[str, Any]:
        """Per-site stats / application metadata."""
        return await self._get(f"/sites/{site_id}")

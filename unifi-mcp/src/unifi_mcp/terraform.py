"""Terraform CLI runner.

The MCP server keeps a single working tree on disk (config.terraform_dir).
Tools edit .tf files inside it, then call plan/apply.
"""

from __future__ import annotations

import asyncio
import json
import os
import shutil
import time
from dataclasses import dataclass
from pathlib import Path


@dataclass
class TFResult:
    rc: int
    stdout: str
    stderr: str
    duration_s: float

    @property
    def ok(self) -> bool:
        return self.rc == 0

    def summary(self, *, max_chars: int = 4000) -> str:
        out = self.stdout or ""
        err = self.stderr or ""
        head = f"exit={self.rc} duration={self.duration_s:.1f}s"
        if len(out) + len(err) <= max_chars:
            return f"{head}\n--- stdout ---\n{out}\n--- stderr ---\n{err}".rstrip()
        return (
            f"{head}\n--- stdout (truncated) ---\n{out[-max_chars:]}\n"
            f"--- stderr (truncated) ---\n{err[-max_chars:]}".rstrip()
        )


class Terraform:
    def __init__(
        self,
        working_dir: Path,
        *,
        binary: str = "terraform",
        env: dict[str, str] | None = None,
    ) -> None:
        self.working_dir = working_dir
        self.binary = binary
        self._env_overrides = env or {}
        self.working_dir.mkdir(parents=True, exist_ok=True)

    def _env(self) -> dict[str, str]:
        e = os.environ.copy()
        # Quieter, machine-readable output.
        e.setdefault("TF_IN_AUTOMATION", "1")
        e.setdefault("TF_INPUT", "0")
        e.setdefault("CHECKPOINT_DISABLE", "1")
        e.update(self._env_overrides)
        return e

    async def _run(self, *args: str, timeout: float = 600.0) -> TFResult:
        if not shutil.which(self.binary):
            raise RuntimeError(
                f"terraform binary {self.binary!r} not found on PATH. "
                "Install Terraform or set TERRAFORM_BIN."
            )
        start = time.monotonic()
        proc = await asyncio.create_subprocess_exec(
            self.binary,
            *args,
            cwd=str(self.working_dir),
            env=self._env(),
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            out, err = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        except TimeoutError as e:
            proc.kill()
            await proc.wait()
            raise RuntimeError(
                f"terraform {' '.join(args)} timed out after {timeout}s"
            ) from e
        return TFResult(
            rc=proc.returncode if proc.returncode is not None else -1,
            stdout=out.decode("utf-8", errors="replace"),
            stderr=err.decode("utf-8", errors="replace"),
            duration_s=time.monotonic() - start,
        )

    async def init(self, *, upgrade: bool = False) -> TFResult:
        args = ["init", "-no-color"]
        if upgrade:
            args.append("-upgrade")
        return await self._run(*args, timeout=300)

    async def validate(self) -> TFResult:
        return await self._run("validate", "-no-color", "-json", timeout=120)

    async def fmt(self) -> TFResult:
        return await self._run("fmt", "-no-color", "-recursive", timeout=60)

    async def plan(self, *, out_file: str = "tfplan") -> TFResult:
        return await self._run(
            "plan",
            "-no-color",
            "-input=false",
            "-detailed-exitcode",
            f"-out={out_file}",
            timeout=600,
        )

    async def apply(self, *, plan_file: str | None = "tfplan") -> TFResult:
        args = ["apply", "-no-color", "-input=false", "-auto-approve"]
        if plan_file:
            args.append(plan_file)
        return await self._run(*args, timeout=900)

    async def destroy(self) -> TFResult:
        return await self._run(
            "destroy", "-no-color", "-input=false", "-auto-approve", timeout=900
        )

    async def show(self, *, plan_file: str | None = None) -> TFResult:
        args = ["show", "-no-color", "-json"]
        if plan_file:
            args.append(plan_file)
        return await self._run(*args, timeout=120)

    async def show_json(self, *, plan_file: str | None = None) -> dict:
        r = await self.show(plan_file=plan_file)
        if not r.ok:
            raise RuntimeError(f"terraform show failed: {r.stderr[:300]}")
        try:
            return json.loads(r.stdout or "{}")
        except json.JSONDecodeError as e:
            raise RuntimeError(f"terraform show returned invalid JSON: {e}") from e

    async def state_list(self) -> TFResult:
        return await self._run("state", "list", "-no-color", timeout=60)

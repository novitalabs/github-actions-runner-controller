#!/usr/bin/env python3
"""JSON-lines adapter between the Go controller and Novita Sandbox SDK."""
from __future__ import annotations

import json
import sys
from datetime import datetime, timezone
from typing import Any

from novita_sandbox import Novita, SandboxQuery, SandboxState

JIT_PATH = "/home/user/actions-runner/.jitconfig"
JIT_TEMP_PATH = JIT_PATH + ".tmp"


class Adapter:
    def __init__(self, client: Novita | None = None) -> None:
        if client is not None:
            self.client = client
            return
        self.client = Novita()

    def list(self, controller_id: str, scale_set_id: int) -> list[dict[str, Any]]:
        query = SandboxQuery(
            metadata={
                "managed_by": "novita-actions-runner-controller",
                "controller_id": controller_id,
                "scale_set_id": str(scale_set_id),
            },
            state=[SandboxState.RUNNING, SandboxState.PAUSED],
        )
        paginator = self.client.sandbox.list(query=query)
        result: list[dict[str, Any]] = []
        while True:
            for item in paginator.next_items():
                metadata = item.metadata or {}
                result.append(
                    {
                        "name": metadata.get("runner_name", ""),
                        "sandbox_id": item.sandbox_id,
                        "state": _state(item.state),
                        "created_at": _timestamp(item.started_at),
                    }
                )
            if not paginator.has_next:
                return result

    def create(self, request: dict[str, Any]) -> dict[str, Any]:
        # At-least-once message delivery can repeat a completed create request.
        existing = self.list(request["controller_id"], int(request["scale_set_id"]))
        for runner in existing:
            if runner["name"] == request["name"]:
                return runner

        sandbox = self.client.sandbox.create(
            request["template"],
            timeout=int(request["timeout_seconds"]),
            metadata={
                "managed_by": "novita-actions-runner-controller",
                "controller_id": request["controller_id"],
                "scale_set_id": str(request["scale_set_id"]),
                "runner_name": request["name"],
            },
        )
        try:
            # JIT is deliberately sent over stdin and the SDK file channel, never in
            # argv, metadata, environment variables, or controller logs.
            sandbox.files.write(JIT_TEMP_PATH, request["jit_config"])
            moved = sandbox.commands.run(f"mv {JIT_TEMP_PATH} {JIT_PATH}")
            if int(getattr(moved, "exit_code", 1)) != 0:
                raise RuntimeError("failed to publish runner JIT config")
            return {
                "name": request["name"],
                "sandbox_id": sandbox.sandbox_id,
                "state": "running",
                "created_at": datetime.now(timezone.utc).isoformat(),
            }
        except Exception:
            sandbox.kill()
            raise

    def delete(self, sandbox_id: str) -> None:
        self.client.sandbox.kill(sandbox_id)


def _state(value: Any) -> str:
    return str(getattr(value, "value", value))


def _timestamp(value: Any) -> str:
    if isinstance(value, datetime):
        return value.astimezone(timezone.utc).isoformat()
    if value is None:
        return ""
    return str(value)


def dispatch(adapter: Adapter, request: dict[str, Any]) -> dict[str, Any]:
    operation = request.get("op")
    if operation == "list":
        return {"runners": adapter.list(request["controller_id"], request["scale_set_id"])}
    if operation == "create":
        return {"runner": adapter.create(request["create"])}
    if operation == "delete":
        adapter.delete(request["sandbox_id"])
        return {}
    raise ValueError(f"unsupported operation: {operation}")


def main() -> int:
    adapter = Adapter()
    for raw in sys.stdin:
        request: dict[str, Any] = {}
        try:
            request = json.loads(raw)
            payload = dispatch(adapter, request)
            response = {"id": request.get("id"), "ok": True, **payload}
        except Exception as exc:  # errors are data; stdout must remain JSON-lines
            response = {"id": request.get("id"), "ok": False, "error": str(exc)}
        print(json.dumps(response, separators=(",", ":")), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

from datetime import datetime, timedelta, timezone
from types import SimpleNamespace

import adapter as adapter_module
from adapter import Adapter, JIT_PATH, JIT_TEMP_PATH, dispatch


class Paginator:
    has_next = False
    def __init__(self, items): self.items = items
    def next_items(self): return self.items


class SandboxAPI:
    def __init__(self): self.items = []; self.killed = []
    def list(self, **_): return Paginator(self.items)
    def create(self, template, **kwargs):
        files = SimpleNamespace(writes=[], write=lambda p, d: files.writes.append((p, d)))
        commands = SimpleNamespace(
            calls=[],
            run=lambda command: commands.calls.append(command) or SimpleNamespace(exit_code=0),
        )
        sb = SimpleNamespace(
            sandbox_id="s1", files=files, commands=commands,
            kill=lambda: self.killed.append("s1")
        )
        self.created = (template, kwargs, sb); return sb
    def kill(self, sid): self.killed.append(sid)


def test_create_transports_jit_only_through_file_api():
    api = SandboxAPI(); adapter = Adapter(SimpleNamespace(sandbox=api))
    req = {"name":"r1","jit_config":"SECRET","template":"tpl","timeout_seconds":60,"controller_id":"c","scale_set_id":3}
    runner = adapter.create(req)
    template, options, sandbox = api.created
    assert runner["sandbox_id"] == "s1"
    assert sandbox.files.writes == [(JIT_TEMP_PATH, "SECRET")]
    assert sandbox.commands.calls == [f"mv {JIT_TEMP_PATH} {JIT_PATH}"]
    assert "SECRET" not in repr((template, options))


def test_create_is_idempotent_by_runner_name():
    api = SandboxAPI(); now = datetime.now(timezone.utc)
    api.items = [SimpleNamespace(sandbox_id="existing",state="running",metadata={"runner_name":"r1"},started_at=now)]
    adapter = Adapter(SimpleNamespace(sandbox=api))
    got = adapter.create({"name":"r1","controller_id":"c","scale_set_id":3})
    assert got["sandbox_id"] == "existing"
    assert not hasattr(api, "created")


def test_dispatch_delete():
    api = SandboxAPI(); adapter = Adapter(SimpleNamespace(sandbox=api))
    assert dispatch(adapter, {"op":"delete","sandbox_id":"s1"}) == {}
    assert api.killed == ["s1"]


def test_adapter_uses_sdk_environment_auth(monkeypatch):
    calls = []
    monkeypatch.setattr(
        adapter_module, "Novita", lambda: calls.append(()) or SimpleNamespace()
    )

    Adapter()

    assert calls == [()]

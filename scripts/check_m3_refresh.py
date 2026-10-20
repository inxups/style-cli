#!/usr/bin/env python3
"""M3-A9 offline built-CLI refresh and genuine old-program compatibility.

Version stubs and configured-hook calls are fixtures, not real delivery.
"""
import argparse
import json
import os
from pathlib import Path
import statistics
import subprocess
import tempfile
import time

from check_m3_update import fixture, run, snapshot, record, sha
from check_m3_project import invoke_hook
from style_json import report as style_report


def manifest(n):
    return ("spec: style/v0.3\nname: periodic\nversion: 1.0.0\n"
            "description: refresh fixture\nrules:\n"
            "  - id: r\n    target: tool\n    tool: shell\n    at: before\n"
            f"    text: Periodic guidance.\n    refresh: {n}\n    reason: periodic\n")


def handler(path, event):
    document = json.loads(path.read_text())
    for group in document["hooks"][event]:
        for hook in group["hooks"]:
            if "agenthooks" in json.dumps(hook):
                return hook
    raise AssertionError((path, event))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--old-binary", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    new, old = args.binary.resolve(), args.old_binary.resolve()
    cases, performance = [], []
    for route in ("codex-user", "codex-plugin", "claude-user", "codex-project", "claude-project"):
        provider = "claude-code" if route.startswith("claude-") else "codex"
        with tempfile.TemporaryDirectory(prefix="style-m3-refresh-") as temporary:
            root = Path(temporary).resolve()
            env, base, codex, claude, stub = fixture(root)
            (stub / "style").symlink_to(new)
            project = root / "project ' space"
            project.mkdir()
            source = root / "source"
            source.mkdir()
            (source / "STYLE.yaml").write_text(manifest(3))
            scope = ("--project", project) if route.endswith("-project") else ()
            before = snapshot(base, codex, claude, project)
            selection = ("--harness", provider, "--route", provider + "=" + route)
            preview = style_report(run(new, env, *scope, "install", source, *selection, "--json", want=1).stdout)
            assert preview["awaiting_confirmation"]
            assert snapshot(base, codex, claude, project) == before
            record(cases, route, "preview-zero-write")
            installed = style_report(run(new, env, *scope, "install", source, *selection, "--json", confirm=True).stdout)
            home = Path(installed["scope"]["home"]) if scope else base
            if route == "codex-plugin":
                env["PLUGIN_ROOT"] = str(home / "adapters/codex/plugin")
            index_path = home / "build/index.json"
            index = json.loads(index_path.read_text())
            assert index["format"] == 2
            assert index["harnesses"][provider]["tool.pre"][0]["refresh"] == 3
            assert not index["harnesses"][provider].get("prompt.submitted")
            provenance = json.loads((home / "build/explain.json").read_text())
            assert provenance["harnesses"][provider][0]["refresh"]["interval"] == 3
            record(cases, route, "index2-provenance-and-original-output-point")
            if route == "codex-plugin":
                hook_path = home / "adapters/codex/plugin/hooks/hooks.json"
            elif scope:
                hook_path = project / (".codex/hooks.json" if provider == "codex" else ".claude/settings.json")
            else:
                hook_path = (codex / "hooks.json") if provider == "codex" else (claude / "settings.json")
            key = "turn_id" if provider == "codex" else "prompt_id"

            def payload(event, turn="", session="session", source=""):
                return json.dumps({"session_id": session, "hook_event_name": event, key: turn,
                                   "source": source, "tool_name": "Bash", "tool_input": {"command": "PRIVATE"},
                                   "prompt": "PRIVATE", "cwd": str(project)})

            def call(event, turn="", session="session", source=""):
                result = invoke_hook(handler(hook_path, event), env, project,
                                     payload(event, turn, session, source))
                assert result.returncode == 0, (result.stdout, result.stderr)
                assert "level=ERROR" not in result.stderr, result.stderr
                return result.stdout

            call("SessionStart", source="startup")
            assert "Periodic guidance." in call("PreToolUse")
            for turn in ("secret-a", "secret-a", "secret-b"):
                assert "Periodic guidance." not in call("UserPromptSubmit", turn)
            assert "Periodic guidance." not in call("PreToolUse", "secret-b")
            call("UserPromptSubmit", "secret-c")
            assert "Periodic guidance." not in call("PreToolUse", "secret-a")
            record(cases, route, "clock-only-replay-late-tool")
            payload_file = root / "event.json"
            payload_file.write_text(payload("PreToolUse", "secret-c"))
            before = snapshot(home / "state", hook_path.parent)
            explained = style_report(run(new, env, "explain", "--harness", provider, "--event", "PreToolUse",
                                       "--payload", payload_file, "--index", index_path,
                                       "--state-dir", home / "state", "--json").stdout)
            detail = explained["entries"][0]["refresh"]
            assert detail["current"] == 3 and detail["last"] == 0 and detail["reason"] == "due", detail
            assert snapshot(home / "state", hook_path.parent) == before
            assert call("PreToolUse", "secret-c") == explained["wire"]["stdout"]
            assert "Periodic guidance." not in call("PreToolUse", "secret-c")
            record(cases, route, "readonly-explain-runtime-wire")
            state_file = home / "state/session/main/refresh" / (provider + ".json")
            assert "secret-" not in state_file.read_text()
            record(cases, route, "hashed-native-receipts")
            before = snapshot(base, codex, claude, project)
            # Run the actual baseline runner against the new whole index.
            rejected = run(old, {**env, "STYLE_HOME": str(home)}, "agenthooks", "run",
                           "--provider=" + provider, payload=payload("PreToolUse", "old"))
            assert "Periodic guidance." not in rejected.stdout
            assert "index-invalid" in rejected.stderr or "index is unavailable" in rejected.stderr
            assert snapshot(base, codex, claude, project) == before
            record(cases, route, "actual-old-runner-format2-whole-index-refusal")
            # These old operations must compile or authenticate the unsupported
            # source. Pure removal of all packages is intentionally not a
            # recompilation compatibility promise about an immutable old CLI.
            for command in (("sync",), ("sync", "periodic", "--yes"),
                            ("update", "--all", "--yes"), ("install", "--frozen", "--yes")):
                rejected = run(old, {**env, "STYLE_HOME": str(home)}, *command, want=1)
                assert snapshot(base, codex, claude, project) == before, (command, rejected.stdout, rejected.stderr)
            record(cases, route, "actual-old-management-conservative-zero-write", commands=4)
            for command in (("sync",), ("sync", "periodic"), ("update", "--all"), ("restore",)):
                run(new, env, *scope, *command, "--json", confirm=True)
            doctor = style_report(run(new, env, *scope, "doctor", "--json").stdout)
            assert any(d["code"] == "refresh-index-2" for d in doctor["diagnostics"])
            record(cases, route, "new-management-and-upgrade-guidance")
            clock = state_file.read_bytes()
            (home / "build/index.json").unlink()
            (home / "build/explain.json").unlink()
            run(new, env, *scope, "restore", "--json", confirm=True)
            assert json.loads(index_path.read_text()) == index
            assert state_file.read_bytes() == clock
            record(cases, route, "frozen-exact-policy-clock-preserved")
            samples = []
            for i in range(50):
                call("UserPromptSubmit", str(i), "performance")
                start = time.perf_counter()
                call("PreToolUse", str(i), "performance")
                samples.append((time.perf_counter() - start) * 1000)
            p95 = sorted(samples)[47]
            assert p95 < 50, (route, p95)
            performance.append({"route": route, "samples": len(samples), "p95_ms": round(p95, 3),
                                "median_ms": round(statistics.median(samples), 3)})
            run(new, env, *scope, "uninstall", "--all", "--json", confirm=True)
            record(cases, route, "uninstall-and-process-latency")
    report = {"status": "passed", "binary_sha256": sha(new), "old_binary_sha256": sha(old),
              "cases": cases, "performance": performance, "real_harness": False}
    args.report.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"cases": len(cases), "performance": performance}))


if __name__ == "__main__":
    main()

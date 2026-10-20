#!/usr/bin/env python3
"""M3-A5 isolated offline recovery, real old installations and runtime wire.

Version-only harness fixtures; native commands are disclosed, never executed.
No remote Git, real harness delivery, credentials or model calls.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import tempfile

from check_m3_update import fixture, manifest, record, run, sha, snapshot
from style_json import report as style_report


def document(result):
    # Prompting commands write their preview and y/N prompt to stderr, and a
    # failed command ends it with one "style restore: " line.
    stderr = result.stderr
    if result.returncode != 0:
        head, _, last = stderr.rstrip("\n").rpartition("\n")
        assert last.startswith("style restore: "), stderr
        stderr = head + "\n" if head else ""
    assert stderr == "" or stderr.endswith(" [y/N] \n"), result.stderr
    value = style_report(result.stdout)
    assert value["format"] == 1 and value["command"] == "restore"
    assert value["locked_source"] is True
    return value


def frozen(binary, env, *flags, want=0, confirm=False):
    return document(run(binary, env, "restore", "--json", *flags, want=want, confirm=confirm))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--baseline-binary", required=True, type=Path)
    parser.add_argument("--old-binary", required=True, type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    new, baseline, old = (p.resolve() for p in
                          (args.binary, args.baseline_binary, args.old_binary))
    cases = []
    for route in ("codex-user", "codex-plugin", "claude-user"):
        with tempfile.TemporaryDirectory(prefix="style-m3-a5-cli-") as temporary:
            root = Path(temporary).resolve()
            env, home, codex, claude, stub = fixture(root)
            provider = "claude-code" if route == "claude-user" else "codex"
            config = claude if provider == "claude-code" else codex
            source = root / "source"
            source.mkdir()
            (source / "STYLE.yaml").write_text(manifest("first", "Locked.", native=True))
            run(baseline, env, "install", source, "--harness", provider,
                "--route", f"{provider}={route}", "--yes", "--json")
            before = snapshot(home, codex, claude)
            mismatch = frozen(new, env, want=1, confirm=True)
            assert any(d["code"] == "frozen-build-mismatch" for d in mismatch["diagnostics"])
            assert snapshot(home, codex, claude) == before
            record(cases, route, "binary-path-binding-change-refused", readonly=True)
            # Rebind explicitly through normal install before testing recovery
            # under the same executable and configuration roots.
            run(new, env, "install", source, "--harness", provider,
                "--route", f"{provider}={route}", "--json", confirm=True)
            lock = home / "style.lock.yaml"
            lock.write_bytes(b"# preserve exact locked installation\n" + lock.read_bytes())
            lock.chmod(0o640)
            os.utime(lock, ns=(1_000_000_000, 1_000_000_000))
            locked = snapshot(home)[str(lock)]
            marker = root / "unexpected-executable"
            for name in ("codex", "claude", "git"):
                path = stub / name
                path.write_text(f"#!/bin/sh\nprintf called > '{marker}'\nexit 97\n")
                path.chmod(0o700)
            cache = home / "cache"
            cache.mkdir(exist_ok=True)
            (cache / "keep").write_bytes(b"unused offline cache sentinel\n")
            cache_before = snapshot(cache)
            roots = (home, codex, claude)
            before = snapshot(*roots)
            result = frozen(new, env)
            assert result["status"] == "unchanged" and not result["changed"]
            assert result["lock_sha256"] == "sha256:" + sha(lock)
            assert len(result["native_hooks"]) == 1 and snapshot(*roots) == before
            record(cases, route, "same-binding-noop", preserved=True)
            index = (home / "build/index.json").read_bytes()
            explanation = (home / "build/explain.json").read_bytes()
            shutil.rmtree(source)
            shutil.rmtree(home / "packages/first")
            (home / "build/index.json").unlink()
            (home / "build/explain.json").write_bytes(b"corrupt derived file\n")
            before = snapshot(*roots)
            result = frozen(new, env, want=1)
            assert result["status"] == "confirmation-required" and result["awaiting_confirmation"]
            assert result["contents"] == [{"package": "first", "snapshot": "approved", "storage_repair": True}]
            assert snapshot(*roots) == before
            record(cases, route, "json-preview", approved_snapshot=True, readonly=True)
            text = run(new, env, "restore", want=1)
            assert "printf NATIVE-Locked." in text.stdout and "restore canceled" in text.stderr
            assert snapshot(*roots) == before
            record(cases, route, "text-preview", native_disclosed=True, readonly=True)
            result = frozen(new, env, confirm=True)
            assert result["status"] == "applied" and result["changed"]
            assert (home / "build/index.json").read_bytes() == index
            assert (home / "build/explain.json").read_bytes() == explanation
            assert snapshot(home)[str(lock)] == locked
            record(cases, route, "restore", index_explain_identical=True, lock_bytes_mode_mtime_inode=True)
            before = snapshot(*roots)
            result = frozen(new, env, confirm=True)
            assert result["status"] == "unchanged" and snapshot(*roots) == before
            record(cases, route, "repeat", zero_installation_changes=True)
            for flags in (("source",), ("--ref=refs/heads/main",), ("--set=k=v",),
                          ("--route=x",), ("--frozen",), ("--project",)):
                invalid = run(new, env, "restore", *flags, want=2)
                assert invalid.stdout == "" and invalid.stderr
            assert snapshot(*roots) == before
            record(cases, route, "usage-refusal", combinations=6)
            payload = json.dumps({"session_id": "a5-wire", "hook_event_name": "PreToolUse",
                                  "tool_name": "Bash", "tool_input": {"command": "true"},
                                  "cwd": "/offline"})
            wire = []
            for number, binary in enumerate((baseline, new)):
                runtime = root / f"runtime-{number}"
                (runtime / "build").mkdir(parents=True)
                (runtime / "build/index.json").write_bytes(index)
                result = run(binary, env, f"--style-home={runtime}", f"--harness-home={config}",
                             "--harness-version=2.1.315", "agenthooks", "run",
                             f"--provider={provider}", "--owner=style", payload=payload)
                assert "Locked." in result.stdout and "NATIVE-" not in result.stdout
                wire.append(result.stdout)
            assert wire[0] == wire[1] and not marker.exists()
            assert snapshot(cache) == cache_before
            record(cases, route, "baseline-runtime-wire", identical=True, zero_detection_git=True,
                   cache_unchanged=True, wire_sha256=hashlib.sha256(wire[0].encode()).hexdigest())
    for provider, route in (("codex", "codex-user"), ("claude-code", "claude-user")):
        with tempfile.TemporaryDirectory(prefix="style-m3-a5-old-") as temporary:
            root = Path(temporary).resolve()
            env, home, codex, claude, _ = fixture(root)
            source = root / "source"
            source.mkdir()
            (source / "STYLE.yaml").write_text(manifest("legacy", "Old legacy."))
            run(old, env, "install", source, "--harness", provider,
                "--route", f"{provider}={route}", "--yes", "--json")
            before = snapshot(home, codex, claude)
            result = frozen(new, env, want=1, confirm=True)
            assert result["status"] == "error" and snapshot(home, codex, claude) == before
            record(cases, route, "real-M1-incomplete-baseline-refused", codes=[d["code"] for d in result["diagnostics"]])
            run(new, env, "install", source, "--harness", provider,
                "--route", f"{provider}={route}", "--json", confirm=True)
            assert frozen(new, env)["status"] == "unchanged"
            record(cases, route, "normal-management-establishes-baseline", frozen_unchanged=True)
            locked = snapshot(home)[str(home / "style.lock.yaml")]
            (home / "build/index.json").unlink()
            assert frozen(new, env, confirm=True)["status"] == "applied"
            assert snapshot(home)[str(home / "style.lock.yaml")] == locked
            run(new, env, "uninstall", "--all", "--json", confirm=True)
            record(cases, route, "real-M1-recovery-and-uninstall", exact_lock=True)
    report = {"format": 1, "test": "M3-A5-built-cli", "status": "passed",
              "binary_sha256": sha(new), "baseline_binary_sha256": sha(baseline),
              "old_binary_sha256": sha(old), "cases": cases, "passed": len(cases), "failed": 0,
              "boundary": "Private version fixtures, no native execution, real harness delivery, model or remote Git."}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()

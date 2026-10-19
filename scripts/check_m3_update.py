#!/usr/bin/env python3
"""M3-A4: isolated built-CLI update, old-install compatibility and runtime wire.

Harness executables are version-only stubs. Native commands are disclosed but
never executed. This script is not evidence of real harness delivery.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
from style_json import report as style_report


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def snapshot(*roots):
    files = {}
    for root in roots:
        if not root.exists():
            continue
        for path in sorted(root.rglob("*")):
            info = path.lstat()
            if stat.S_ISREG(info.st_mode):
                files[str(path)] = (sha(path), stat.S_IMODE(info.st_mode),
                                    info.st_mtime_ns, info.st_dev, info.st_ino)
            elif stat.S_ISLNK(info.st_mode):
                files[str(path)] = (os.readlink(path), info.st_mode, info.st_mtime_ns)
            else:
                assert stat.S_ISDIR(info.st_mode), path
    return files


def manifest(name, body, native=False):
    text = (f"spec: style/v0.3\nname: {name}\nversion: 1.0.0\ndescription: update CLI fixture\n"
            "rules:\n"
            f"  - {{id: static, target: system, text: 'Static {body}'}}\n"
            f"  - {{id: runtime, target: tool, tool: shell, at: before, text: '{body}'}}\n")
    if native:
        text += ("native_hooks:\n  - id: executable\n"
                 f"    codex: {{event: PreToolUse, matcher: '^Bash$', command: 'printf NATIVE-{body}', timeout: 17}}\n"
                 f"    claude-code: {{event: PreToolUse, matcher: '*', command: 'printf NATIVE-{body}', timeout: 17}}\n")
    return text


def fixture(root):
    home, style, codex, claude, stub = (root / name for name in
                                       ("home", "style", "codex", "claude", "bin"))
    for directory in (home, style, codex, claude, stub):
        directory.mkdir(mode=0o700)
    for name, version in (("codex", "codex-cli 0.159.3"),
                          ("claude", "2.1.315 (Claude Code)")):
        path = stub / name
        path.write_text(f"#!/bin/sh\nprintf '%s\\n' '{version}'\n")
        path.chmod(0o700)
    (codex / "config.toml").write_bytes(b"# foreign configuration\nmodel = 'external'\n")
    (codex / "config.toml").chmod(0o640)
    (claude / "CLAUDE.md").write_bytes(b"Foreign instructions.\n")
    (claude / "CLAUDE.md").chmod(0o640)
    # Deliberately replace inherited configuration with fixture-local roots.
    env = {"HOME": str(home), "STYLE_HOME": str(style), "CODEX_HOME": str(codex),
           "CLAUDE_CONFIG_DIR": str(claude), "PATH": str(stub) + os.pathsep + os.defpath,
           "LANG": "en_US.UTF-8"}
    return env, style, codex, claude, stub


def run(binary, env, *args, want=0, payload=None, confirm=False):
    # confirm answers a mutating command's y/N prompt; otherwise stdin is closed
    # so an unexpected prompt declines instead of waiting on the terminal.
    assert not (confirm and payload is not None)
    stdin = {"input": "y\n"} if confirm else {"input": payload} if payload is not None else {"stdin": subprocess.DEVNULL}
    result = subprocess.run([str(binary), *map(str, args)], env=env, **stdin,
                            text=True, capture_output=True, timeout=30)
    assert result.returncode == want, (args, result.returncode, result.stdout, result.stderr)
    return result


def document(result):
    # Prompting commands write their preview and y/N prompt to stderr.
    assert result.stderr == "" or result.stderr.endswith(" [y/N] \n"), result.stderr
    value = style_report(result.stdout)
    assert value["format"] == 1 and value["command"] == "update", value
    return value


def record(cases, route, action, **facts):
    cases.append({"route": route, "action": action, "status": "passed", **facts})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--old-binary", required=True, type=Path)
    parser.add_argument("--baseline-binary", required=True, type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    new, old, baseline = args.binary.resolve(), args.old_binary.resolve(), args.baseline_binary.resolve()
    cases = []
    for route in ("codex-user", "codex-plugin", "claude-user"):
        with tempfile.TemporaryDirectory(prefix="style-m3-a4-cli-") as temporary:
            root = Path(temporary).resolve()
            env, home, codex, claude, stub = fixture(root)
            provider = "claude-code" if route == "claude-user" else "codex"
            source = root / "source"
            source.mkdir()
            source_file = source / "STYLE.yaml"
            source_file.write_text(manifest("first", "Old.", native=True))
            run(new, env, "install", source, "--harness", provider,
                "--route", f"{provider}={route}", "--json", confirm=True)
            # update and runtime must not discover a new harness or launch Git.
            marker = root / "unexpected-executable"
            for name in ("codex", "claude", "git"):
                path = stub / name
                path.write_text(f"#!/bin/sh\nprintf called > '{marker}'\nexit 97\n")
                path.chmod(0o700)
            roots = (home, codex, claude)
            source_file.write_text(manifest("first", "New.", native=True)
                                   .replace("version: 1.0.0", "version: 0.5.0"))
            before = snapshot(*roots)
            preview = document(run(new, env, "update", "first", "--json", want=1))
            assert preview["status"] == "confirmation-required" and preview["awaiting_confirmation"]
            assert preview["changes"][0]["after"]["version"] == "0.5.0"
            assert preview["changes"][0]["native_changed"] and preview["native_hooks"]
            assert snapshot(*roots) == before, "preview wrote installation bytes/metadata"
            record(cases, route, "json-preview", unchanged=True, native_disclosed=True)
            text = run(new, env, "update", "first", want=1)
            assert "printf NATIVE-Old." in text.stdout and "printf NATIVE-New." in text.stdout
            assert "Locked routes:" in text.stdout and "update canceled" in text.stderr
            assert snapshot(*roots) == before
            record(cases, route, "text-preview", unchanged=True, before_after_disclosed=True)
            applied = document(run(new, env, "update", "first", "--json", confirm=True))
            assert applied["status"] == "applied" and applied["changed"]
            index = json.loads((home / "build" / "index.json").read_bytes())
            assert index["format"] == 1 and index["packages"][0]["seq"] == 1
            assert index["packages"][0]["version"] == "0.5.0"
            assert b"printf NATIVE" not in (home / "build" / "index.json").read_bytes()
            record(cases, route, "apply", version="0.5.0", seq=1, runtime_format=1)
            before = snapshot(*roots)
            unchanged = document(run(new, env, "update", "--all", "--json", confirm=True))
            assert unchanged["status"] == "unchanged" and not unchanged["changed"]
            assert snapshot(*roots) == before, "no-op replaced a file or changed mtime"
            record(cases, route, "unchanged", bytes_mode_mtime_inode_preserved=True)
            invalid = document(run(new, env, "update", "missing", "--json", want=1, confirm=True))
            assert invalid["status"] == "error" and invalid["changes"] == []
            assert snapshot(*roots) == before
            record(cases, route, "single-error-json", unchanged=True)
            for args_bad in (("--json",), ("first", "--all", "--json"),
                             ("--all", "--ref=refs/heads/main", "--json")):
                result = run(new, env, "update", *args_bad, want=2)
                assert result.stdout == "" and result.stderr
            record(cases, route, "usage-errors", stdout_empty=True, combinations=3)
            payload = json.dumps({"session_id": "a4-wire", "hook_event_name": "PreToolUse",
                                  "tool_name": "Bash", "tool_input": {"command": "true"},
                                  "cwd": "/offline"})
            wire = []
            config = claude if provider == "claude-code" else codex
            for number, binary in enumerate((baseline, new)):
                runtime = root / f"runtime-{number}"
                (runtime / "build").mkdir(parents=True)
                shutil.copy(home / "build" / "index.json", runtime / "build" / "index.json")
                result = run(binary, env, f"--style-home={runtime}", f"--harness-home={config}",
                             "--harness-version=2.1.315", "agenthooks", "run",
                             f"--provider={provider}", "--owner=style", payload=payload)
                assert "New." in result.stdout and "NATIVE-" not in result.stdout, (route, binary, result.stdout, result.stderr, index)
                wire.append(result.stdout)
            assert wire[0] == wire[1], "ordinary runtime wire differs from the pre-update binary"
            assert not marker.exists(), "update/runtime invoked detection or Git"
            record(cases, route, "baseline-new-runtime-wire", identical=True,
                   wire_sha256=hashlib.sha256(wire[0].encode()).hexdigest(), zero_detection_git=True)
    for provider in ("codex", "claude-code"):
        with tempfile.TemporaryDirectory(prefix="style-m3-a4-old-") as temporary:
            root = Path(temporary).resolve()
            env, home, codex, claude, _ = fixture(root)
            source = root / "source"
            source.mkdir()
            path = source / "STYLE.yaml"
            path.write_text(manifest("legacy", "Old legacy."))
            route = "claude-user" if provider == "claude-code" else "codex-user"
            run(old, env, "install", source, "--harness", provider, "--route",
                f"{provider}={route}", "--yes", "--json")
            path.write_text(manifest("legacy", "Updated legacy."))
            before = snapshot(home, codex, claude)
            if provider == "claude-code":
                # The fixed M1 binary uses claude_md. update must not perform
                # the migration assigned to the existing management flow.
                error = document(run(new, env, "update", "legacy", "--json", want=1, confirm=True))
                assert any(d["code"] == "update-binding-changed" for d in error["diagnostics"])
                assert snapshot(home, codex, claude) == before
                record(cases, route, "old-layout-update-refused", unchanged=True)
                run(new, env, "sync", "--json")
            result = document(run(new, env, "update", "legacy", "--json", confirm=True))
            assert result["status"] == "applied" and result["changed"]
            run(new, env, "sync", "--json")
            run(new, env, "trust", "legacy", "--json", confirm=True)
            assert document(run(new, env, "update", "--all", "--json"))["status"] == "unchanged"
            run(new, env, "uninstall", "--all", "--json", confirm=True)
            record(cases, route, "real-old-install-update-lifecycle", legacy_format=1)
    report = {"format": 1, "test": "M3-A4-built-cli", "status": "passed",
              "binary_sha256": sha(new), "old_binary_sha256": sha(old),
              "baseline_binary_sha256": sha(baseline),
              "cases": cases, "passed": len(cases), "failed": 0,
              "boundary": "Version stubs, private files and raw runtime invocation; no real harness delivery, native command execution, credentials or remote Git."}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()

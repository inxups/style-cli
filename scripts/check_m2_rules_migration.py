#!/usr/bin/env python3
"""Offline upgrade acceptance using two actual STYLE binaries and version stubs.

Usage: python3 scripts/check_m2_rules_migration.py --old-binary OLD --binary NEW
       [--report /tmp/style-rules-migration.json]
Never launches a real harness or accesses its configuration or credentials.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree(*roots):
    return {str(path): (path.read_bytes(), path.stat().st_mode & 0o777, path.stat().st_mtime_ns)
            for root in roots for path in root.rglob("*") if path.is_file()}


def package(root, name):
    source = root / name
    source.mkdir()
    (source / "STYLE.yaml").write_text(
        f"spec: style/v0.3\nname: {name}\nversion: 0.1.0\ndescription: migration fixture\n"
        f"rules:\n  - {{id: static, target: system, text: 'Approved {name} instructions.'}}\n")
    return source


def run(binary, env, *args, confirm=False):
    # The current binary takes approval on stdin; the old one uses --yes.
    stdin = {"input": "y\n"} if confirm else {"stdin": subprocess.DEVNULL}
    result = subprocess.run([str(binary), *map(str, args)], env=env, **stdin,
                            capture_output=True, text=True, timeout=60)
    assert result.returncode == 0, (args, result.returncode, result.stdout, result.stderr)
    return json.loads(result.stdout)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--old-binary", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    old, new = args.old_binary.resolve(), args.binary.resolve()
    cases = []
    for entrypoint in ("install", "sync", "trust", "uninstall"):
        for newline, original in (("lf", b"User instructions\n"), ("crlf", "用户\r\n".encode()),
                                  ("no-newline", "用户 instructions".encode()), ("absent", None)):
            with tempfile.TemporaryDirectory(prefix="style-a7-upgrade-") as temporary:
                root = Path(temporary)
                home, config, style_home, stub = (root / name for name in ("home", "claude", "style", "bin"))
                for directory in (home, config, stub):
                    directory.mkdir()
                for name, version in (("claude", "2.1.315 (Claude Code)"), ("codex", "codex-cli 0.159.3")):
                    path = stub / name
                    path.write_text(f"#!/bin/sh\nprintf '%s\\n' '{version}'\n")
                    path.chmod(0o755)
                env = {"HOME": str(home), "PATH": str(stub) + os.pathsep + os.defpath,
                       "STYLE_HOME": str(style_home), "CLAUDE_CONFIG_DIR": str(config),
                       "LANG": "en_US.UTF-8"}
                legacy = config / "CLAUDE.md"
                if original is not None:
                    legacy.write_bytes(original)
                    legacy.chmod(0o640)
                foreign = config / "rules" / "user.md"
                foreign.parent.mkdir()
                foreign.write_bytes(b"Unrelated user rules.\n")
                first, second = package(root, "first"), package(root, "second")
                for source in (first, second):
                    run(old, env, "install", source, "--harness", "claude-code", "--yes", "--json")
                before_index = json.loads((style_home / "build" / "index.json").read_bytes())
                assert all(row["slot"] == "claude_md" for row in before_index["static"]["claude-code"])
                assert b"<!-- style:first -->" in legacy.read_bytes()
                before = tree(style_home, config)
                for command in (("plan", first, "--harness", "claude-code", "--json"), ("list", "--json"),
                                ("doctor", "--json"), ("stats", "--json")):
                    output = run(new, env, *command)
                    if command[0] == "doctor":
                        assert output["healthy"], output["diagnostics"]
                        assert any(row["code"] == "static-migration-pending" for row in output["diagnostics"])
                assert tree(style_home, config) == before, "read-only commands mutated old installation"
                expected = {"first", "second"}
                if entrypoint == "install":
                    command = ("install", package(root, "third"), "--harness", "claude-code", "--json")
                    expected.add("third")
                elif entrypoint == "sync":
                    command = ("sync", "--json")
                elif entrypoint == "trust":
                    command = ("trust", "first", "--json")
                else:
                    command = ("uninstall", "first", "--json")
                    expected.remove("first")
                migrated = run(new, env, *command, confirm=True)
                assert len(migrated["static_migrations"]) == 2, migrated
                assert not migrated.get("needs_trust"), migrated
                assert {path.stem.removeprefix("style-") for path in foreign.parent.glob("style-*.md")} == expected
                for name in expected:
                    data = (foreign.parent / f"style-{name}.md").read_bytes()
                    assert data == (f"<!-- style-rules:v1 {name} -->\nApproved {name} instructions.\n"
                                    f"<!-- /style-rules:v1 {name} -->\n").encode()
                if original is None:
                    assert not legacy.exists()
                else:
                    assert legacy.read_bytes() == original and legacy.stat().st_mode & 0o777 == 0o640
                after_index = json.loads((style_home / "build" / "index.json").read_bytes())
                assert after_index["format"] == before_index["format"]
                assert all(row["slot"] == "claude_rules" for row in after_index["static"]["claude-code"])
                repeat = run(new, env, "sync", "--json")
                assert not repeat["changed"] and not repeat.get("static_migrations"), repeat
                assert run(new, env, "doctor", "--json")["healthy"]
                run(new, env, "uninstall", "--all", "--json", confirm=True)
                assert not list(foreign.parent.glob("style-*.md"))
                assert foreign.read_bytes() == b"Unrelated user rules.\n"
                if original is not None:
                    assert legacy.read_bytes() == original and legacy.stat().st_mode & 0o777 == 0o640
                cases.append({"entrypoint": entrypoint, "legacy_fixture": newline, "passed": True,
                              "read_only_commands": 5, "migrated_owners": 2, "repeat_sync_changed": False,
                              "doctor_healthy_before_and_after": True, "external_bytes_and_mode_restored": True})
    report = {"test": "M2-A7 actual binary upgrade", "scope": "offline CLI, harness version stubs; no real Claude session",
              "old_binary_sha256": digest(old), "binary_sha256": digest(new), "cases": cases,
              "passed": len(cases), "source_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
              "source_includes_uncommitted_T40": True}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(f"M2-A7: {len(cases)}/16 actual binary upgrades passed")


if __name__ == "__main__":
    main()

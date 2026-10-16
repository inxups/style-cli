#!/usr/bin/env python3
"""M2-A8: built CLI replace fallback and user output style preservation.

Uses isolated configuration and --version stubs; no real harness/model calls.
Usage: python3 scripts/check_m2_replace.py BINARY [--report REPORT.json]
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def snapshot(*roots, mtime=False):
    return {str(path): (path.read_bytes(), path.stat().st_mode & 0o777,
                       path.stat().st_mtime_ns if mtime else None)
            for root in roots for path in root.rglob("*") if path.is_file()}


def run(binary, env, args, expected):
    result = subprocess.run([str(binary), *map(str, args)], env=env,
                            capture_output=True, text=True, timeout=60)
    assert result.returncode == expected, (args, result.returncode, result.stdout, result.stderr)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    cases = []
    for provider in ("codex", "claude-code"):
        for fallback in ("degrade", "skip", "error"):
            for json_output in (False, True):
                with tempfile.TemporaryDirectory(prefix="style-a8-replace-") as temporary:
                    root = Path(temporary)
                    home, codex, claude, style_home, stub, source = (root / name for name in
                        ("home", "codex", "claude", "style", "bin", "source"))
                    for directory in (home, codex, claude, stub, source, claude / "output-styles"):
                        directory.mkdir()
                    for name, version in (("claude", "2.1.315 (Claude Code)"), ("codex", "codex-cli 0.159.3")):
                        path = stub / name
                        path.write_text(f"#!/bin/sh\nprintf '%s\\n' '{version}'\n")
                        path.chmod(0o755)
                    # Explicit isolated CODEX_HOME is intentional; no host variables are inherited.
                    env = {"HOME": str(home), "PATH": str(stub) + os.pathsep + os.defpath,
                           "STYLE_HOME": str(style_home), "CODEX_HOME": str(codex),
                           "CLAUDE_CONFIG_DIR": str(claude), "LANG": "en_US.UTF-8"}
                    settings = claude / "settings.json"
                    settings.write_bytes(b'{\n "outputStyle" : "user-existing", "env" : {"KEEP":"yes"}\n}')
                    settings.chmod(0o640)
                    style = claude / "output-styles" / "user-existing.md"
                    style.write_bytes(b'---\nname: user-existing\nkeep-coding-instructions: true\n---\nUser style.\n')
                    style.chmod(0o600)
                    (codex / "config.toml").write_bytes(b"# External configuration\nmodel = 'external'\n")
                    (codex / "config.toml").chmod(0o640)
                    (source / "STYLE.yaml").write_text(
                        "spec: style/v0.3\nname: replace-probe\nversion: 0.1.0\ndescription: replace fallback\n"
                        "rules:\n  - id: candidate\n    target: system\n    mode: replace\n"
                        f"    fallback: {fallback}\n    text: Candidate replacement instructions.\n")
                    roots = (style_home, codex, claude)
                    original, untouched = snapshot(*roots), snapshot(*roots, mtime=True)
                    flags = ["--json"] if json_output else []
                    status = "install-error" if fallback == "error" else "skipped"
                    exit_code = 4 if fallback == "error" else 0
                    planned = run(binary, env, ["plan", source, "--harness", provider, *flags], exit_code)
                    if json_output:
                        plan = json.loads(planned.stdout)["reports"][0]["plan"]
                        rule = plan["rules"][0]
                        assert rule["status"] == status and not rule.get("placements"), rule
                        assert plan["blocked"] == (fallback == "error"), plan
                    else:
                        assert f"- candidate: {status}" in planned.stdout and "  -> " not in planned.stdout
                    assert snapshot(*roots, mtime=True) == untouched, "plan wrote configuration"
                    unauthorized = run(binary, env, ["install", source, "--harness", provider, "--yes", *flags], 1)
                    assert "--allow-replace" in unauthorized.stderr
                    assert snapshot(*roots, mtime=True) == untouched, "missing authorization wrote configuration"
                    route = "codex-user" if provider == "codex" else "claude-user"
                    installed = run(binary, env, ["install", source, "--harness", provider,
                        "--route", f"{provider}={route}", "--allow-replace", "--yes", *flags], exit_code)
                    assert json.loads(settings.read_bytes())["outputStyle"] == "user-existing"
                    assert snapshot(claude) == {path: state for path, state in original.items() if Path(path).is_relative_to(claude)}, "user output style tree changed"
                    if fallback == "error":
                        assert snapshot(*roots, mtime=True) == untouched and not style_home.exists(), "blocked installation wrote files"
                    else:
                        if json_output:
                            rule = json.loads(installed.stdout)["reports"][0]["plan"]["rules"][0]
                            assert rule["status"] == "skipped" and not rule.get("placements"), rule
                        else:
                            assert "- candidate: skipped" in installed.stdout
                        index = json.loads((style_home / "build" / "index.json").read_bytes())
                        assert not index["static"].get(provider), index
                        assert not any(index["harnesses"].get(provider, {}).values()), index
                        run(binary, env, ["uninstall", "--all", "--yes", *flags], 0)
                        assert snapshot(codex, claude) == original, "round trip changed user configuration"
                    cases.append({"provider": provider, "fallback": fallback,
                                  "format": "json" if json_output else "text", "plan_status": status,
                                  "plan_exit": exit_code, "unauthorized_install_exit": 1,
                                  "authorized_install_exit": exit_code, "placements": 0,
                                  "user_style_tree_preserved": True, "passed": True})
    report = {"test": "M2-A8 replace fallback", "scope": "offline built CLI with version stubs; C4 UI acceptance remains pending",
              "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "source_revision": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
              "cases": cases, "passed": len(cases)}
    if args.report:
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(f"M2-A8: {len(cases)}/12 replace fallback cases passed; user output styles unchanged")


if __name__ == "__main__":
    main()

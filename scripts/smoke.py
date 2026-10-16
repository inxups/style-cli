#!/usr/bin/env python3
"""End-to-end smoke test for a built style binary.

Runs validate, plan, install, list, doctor, sync and uninstall in a throwaway
HOME, then feeds harness hook payloads to the commands that install wrote into
Codex hooks.json and Claude settings.json. Codex and Claude Code are replaced by
stubs that only answer --version, so this checks the STYLE side of the contract,
not that a real harness accepts the configuration.

Usage: python3 scripts/smoke.py path/to/style
"""

import json
import os
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EXAMPLE = ROOT / "example" / "terse-reviewer"
PROBES = ROOT / "testdata" / "manual" / "m1b-probes"
CODEX_VERSION = "codex-cli 0.159.3"
CLAUDE_VERSION = "2.1.300 (Claude Code)"
FORCE_PUSH_REASON = "Do not force push; create a new commit."

BLOCKED = [
    "git push --force origin main",
    "git push -f origin main",
    "git -C repo push origin +main",
    "bash --rcfile /dev/null -c 'git push --force origin main'",
    "bash -O extglob -c 'git push -f origin main'",
    "bash -eo pipefail -c 'git push --force origin main'",
]
ALLOWED = [
    "git status --short",
    "git push origin main",
    "echo 'git push --force'",
    "bash --rcfile /dev/null -c 'git status --short'",
]

failures = []


def check(ok, message):
    print(("ok   " if ok else "FAIL ") + message)
    if not ok:
        failures.append(message)


def run(argv, env, stdin=None):
    return subprocess.run(argv, input=stdin, capture_output=True, text=True, env=env, timeout=60)


def style(binary, env, *args):
    result = run([binary, *args], env)
    if result.returncode != 0:
        sys.stderr.write(result.stdout + result.stderr)
    return result


def write_stub(directory, name, output):
    path = directory / name
    path.write_text("#!/bin/sh\necho '%s'\n" % output)
    path.chmod(0o755)


def hook_commands(home):
    codex = json.loads((home / ".codex" / "hooks.json").read_text())
    claude = json.loads((home / ".claude" / "settings.json").read_text())
    codex_entry = codex["hooks"]["PreToolUse"][0]["hooks"][0]
    claude_bash = next(group for group in claude["hooks"]["PreToolUse"] if "Bash" in group["matcher"])
    claude_entry = claude_bash["hooks"][0]
    # Codex runs its command string through a shell; Claude stores argv split.
    return {
        "codex": ["/bin/sh", "-c", codex_entry["command"]],
        "claude-code": [claude_entry["command"], *claude_entry.get("args", [])],
    }


def pre_tool_use(command):
    return json.dumps({
        "session_id": "smoke",
        "cwd": "/tmp",
        "model": "smoke-model",
        "turn_id": "turn-1",
        "hook_event_name": "PreToolUse",
        "tool_name": "Bash",
        "tool_input": {"command": command},
    })


def decision(stdout):
    if not stdout.strip():
        return "allow", ""
    output = json.loads(stdout).get("hookSpecificOutput", {})
    return output.get("permissionDecision", "allow"), output.get("permissionDecisionReason", "")


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: smoke.py path/to/style")
    binary = str(Path(sys.argv[1]).resolve())
    with tempfile.TemporaryDirectory(prefix="style-smoke-") as work:
        work = Path(work).resolve()
        home = work / "home"
        stubs = work / "bin"
        home.mkdir()
        stubs.mkdir()
        write_stub(stubs, "codex", CODEX_VERSION)
        write_stub(stubs, "claude", CLAUDE_VERSION)
        env = {key: value for key, value in os.environ.items()
               if key not in ("CODEX_HOME", "CODEX_SANDBOX", "CLAUDE_CONFIG_DIR", "STYLE_HOME")}
        env.update(HOME=str(home), STYLE_HOME=str(home / ".style"),
                   PATH=str(stubs) + os.pathsep + env.get("PATH", ""))

        for package in (EXAMPLE, PROBES):
            name = package.name
            result = style(binary, env, "validate", str(package))
            check(result.returncode == 0 and result.stdout.strip() == "valid", f"validate {name}")
            result = style(binary, env, "plan", str(package), "--harness", "codex,claude-code", "--json")
            try:
                json.loads(result.stdout)
                parsed = True
            except ValueError:
                parsed = False
            check(result.returncode == 0 and parsed, f"plan {name} --json")

        result = style(binary, env, "install", str(EXAMPLE), "--harness", "codex,claude-code",
                       "--route", "codex=codex-user", "--yes")
        check(result.returncode == 0, "install terse-reviewer for codex-user and claude-user")
        if result.returncode != 0:
            return

        listed = json.loads(style(binary, env, "list", "--json").stdout)["packages"]
        check([(p["name"], p["hash_matches"]) for p in listed] == [("terse-reviewer", True)],
              "list shows terse-reviewer with matching hash")
        result = style(binary, env, "doctor", "--json")
        check(result.returncode == 0 and json.loads(result.stdout)["healthy"] is True, "doctor healthy")
        check("Review code concisely" in (home / ".codex" / "config.toml").read_text(),
              "codex developer_instructions written")
        check("Review code concisely" in (home / ".claude" / "CLAUDE.md").read_text(),
              "claude CLAUDE.md written")

        for provider, argv in hook_commands(home).items():
            for command in BLOCKED + ALLOWED:
                result = run(argv, env, pre_tool_use(command))
                verdict, reason = decision(result.stdout) if result.returncode == 0 else ("error", result.stderr)
                if command in BLOCKED:
                    ok = verdict == "deny" and reason == FORCE_PUSH_REASON
                else:
                    ok = verdict == "allow"
                check(ok, f"{provider} PreToolUse {verdict}: {command}")

        result = style(binary, env, "sync", "--json")
        check(result.returncode == 0, "sync")
        result = style(binary, env, "uninstall", "--all", "--yes")
        check(result.returncode == 0, "uninstall --all")
        leftovers = [path for path in (home / ".codex" / "hooks.json", home / ".claude" / "settings.json",
                                       home / ".codex" / "config.toml", home / ".claude" / "CLAUDE.md")
                     if path.exists() and ("agenthooks" in path.read_text() or "style:terse-reviewer" in path.read_text())]
        check(not leftovers, "uninstall removes STYLE hooks and static text" +
              ("" if not leftovers else ": " + ", ".join(shlex.quote(str(p)) for p in leftovers)))


if __name__ == "__main__":
    main()
    if failures:
        print(f"{len(failures)} smoke check(s) failed", file=sys.stderr)
        sys.exit(1)
    print("smoke passed")

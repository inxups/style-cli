#!/usr/bin/env python3
"""Prepare M2-C2/C3 in a new private scratch; never start login/model sessions."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys

from probe_claude_m2_design import ui_environment, write_private


STATIC_BODY = "M2_CLAUDE_STATIC_8C219A. Answer with concise, concrete statements."
NATIVE_BODIES = {"start": "M2_CLAUDE_NATIVE_START_71D4B8. Native session fixture.",
                 "shell": "M2_CLAUDE_NATIVE_SHELL_329A6F. Native Bash fixture.",
                 "read": "M2_CLAUDE_NATIVE_READ_52F1C7. This must not match Bash."}


def observation(payload, kind):
    allowed = ("hook_event_name", "session_id", "file_path", "memory_type", "load_reason", "tool_name")
    return {"fixture": kind, "fields": {key: payload[key] for key in allowed if key in payload}}


def observe(log, kind):
    payload = json.load(sys.stdin)
    descriptor = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(descriptor, (json.dumps(observation(payload, kind), ensure_ascii=False) + "\n").encode())
    finally:
        os.close(descriptor)
    if kind in NATIVE_BODIES:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": payload["hook_event_name"],
                                                 "additionalContext": NATIVE_BODIES[kind]}}))


def verify_restored(root):
    root = Path(root)
    baseline = json.loads((root / "baseline.json").read_text())
    for relative, record in baseline.items():
        path = root / "claude" / relative
        assert path.read_bytes() == base64.b64decode(record["bytes"]), relative
        assert path.stat().st_mode & 0o777 == record["mode"], relative
    for name in ("first", "second"):
        assert not (root / "claude/rules" / f"style-m2-claude-{name}.md").exists(), "managed rule remains"
    assert not (root / "style/style.lock.yaml").exists(), "lock remains"
    assert not (root / "style/build/index.json").exists(), "index remains"
    return {"baseline_files": len(baseline), "external_bytes_and_modes_restored": True,
            "managed_rules_lock_index_removed": True, "real_session_acceptance": False}


def prepare(root, style, claude):
    root, style, claude = (Path(value).resolve() for value in (root, style, claude))
    if root.exists() and any(root.iterdir()):
        raise ValueError("scratch must be empty")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    root.chmod(0o700)
    for name in ("work", "claude", "bin", "xdg-config", "anthropic"):
        (root / name).mkdir(mode=0o700)
    binary = root / "bin/style"
    shutil.copyfile(style, binary)
    binary.chmod(0o700)
    (root / "bin/claude").symlink_to(claude)
    environment = ui_environment(root)
    environment.update(STYLE_HOME=str(root / "style"), PATH=str(root / "bin") + os.pathsep + environment.get("PATH", os.defpath))
    version = subprocess.run([str(claude), "--version"], env=environment,
                             check=True, capture_output=True, text=True, timeout=30).stdout.strip()
    # Pin the observer's full source, including its small local import closure.
    for name in ("prepare_claude_m2_acceptance.py", "probe_claude_m2_design.py", "claude_probe_support.py"):
        shutil.copyfile(Path(__file__).with_name(name), root / name)
        (root / name).chmod(0o600)
    observer = root / "prepare_claude_m2_acceptance.py"
    command = shlex.join([sys.executable, str(observer), "--observe", str(root / "instructions.jsonl"), "instructions"])
    initial = {"outputStyle": "M2Existing", "env": {"M2_FOREIGN": "keep"}, "hooks": {
        "InstructionsLoaded": [{"hooks": [{"type": "command", "command": command, "timeout": 10}]}]}}
    config = root / "claude"
    write_private(config / "settings.json", json.dumps(initial, indent=2) + "\n")
    write_private(config / "CLAUDE.md", "M2_EXTERNAL_MEMORY_29C6F1. Preserve this user file.\r\n")
    write_private(config / "rules/user.md", "M2_EXTERNAL_RULES_31AF29. Preserve these user rules.\n")
    write_private(config / "output-styles/M2Existing.md", "---\nname: M2Existing\nkeep-coding-instructions: true\n---\nM2 user-owned output style.\n")
    baseline = {path.relative_to(config).as_posix(): {"bytes": base64.b64encode(path.read_bytes()).decode(),
                "mode": path.stat().st_mode & 0o777} for path in config.rglob("*") if path.is_file()}
    write_private(root / "baseline.json", json.dumps(baseline, indent=2) + "\n")
    rules = [{"id": "shared", "target": "system", "text": STATIC_BODY}]
    native_hooks = []
    for kind, event, matcher in (("start", "SessionStart", None), ("shell", "PreToolUse", "^Bash$"), ("read", "PreToolUse", "^Read$")):
        native = {"event": event, "command": shlex.join([sys.executable, str(observer), "--observe",
                  str(root / "native-observations.jsonl"), kind]), "timeout": 10}
        if matcher:
            native["matcher"] = matcher
        native_hooks.append({"id": "native-" + kind, "claude-code": native})
    installs = []
    for name, package_rules in (("first", rules), ("second", rules[:1])):
        package = root / ("m2-claude-" + name)
        write_private(package / "STYLE.yaml", json.dumps({"spec": "style/v0.3", "name": "m2-claude-" + name,
            "version": "0.1.0", "description": "Isolated M2 Claude acceptance", "rules": package_rules, "native_hooks": native_hooks if name == "first" else []}, indent=2))
        result = subprocess.run([str(binary), "install", str(package), "--harness", "claude-code", "--json"], input="y\n",
                                env=environment, cwd=root / "work", check=True, capture_output=True, text=True, timeout=60)
        installs.append(json.loads(result.stdout))
    prefix = "exec env -i " + " ".join(shlex.quote(key + "=" + value) for key, value in environment.items()) + " "
    header = "#!/bin/sh\nset -eu\ncd " + shlex.quote(str(root / "work")) + "\n"
    commands = {"style.sh": [str(binary)], "login.sh": [str(claude), "auth", "login"],
                "launch.sh": [str(claude), "--setting-sources", "user", "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
                              "--debug-file", str(root / "claude-debug.log")],
                "verify-restored.sh": [sys.executable, str(observer), "--verify-restored", str(root)]}
    for name, argv in commands.items():
        path = root / name
        path.write_text(header + prefix + shlex.join(argv) + ' "$@"\n')
        path.chmod(0o700)
    report = {"item": "M2-PREP-03", "acceptance": False, "status": "prepared-awaiting-user-session-and-quota",
              "scratch": str(root), "style_binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "claude_version": version, "claude_sha256": hashlib.sha256(claude.read_bytes()).hexdigest(),
              "install": installs, "static_rule": str(config / "rules/style-m2-claude-first.md"),
              "expected_static_body_sha256": hashlib.sha256(STATIC_BODY.encode()).hexdigest(),
              "launchers": {name: str(root / name) for name in commands}, "baseline_files": len(baseline)}
    write_private(root / "prepared.json", json.dumps(report, indent=2) + "\n")
    return report


def main():
    if len(sys.argv) == 4 and sys.argv[1] == "--observe":
        observe(sys.argv[2], sys.argv[3])
        return
    if len(sys.argv) == 3 and sys.argv[1] == "--verify-restored":
        print(json.dumps(verify_restored(sys.argv[2]), indent=2))
        return
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True, type=Path)
    parser.add_argument("--style", required=True, type=Path)
    parser.add_argument("--claude", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(prepare(args.scratch, args.style, args.claude), indent=2))


if __name__ == "__main__":
    main()

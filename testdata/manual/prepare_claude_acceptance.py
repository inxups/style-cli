#!/usr/bin/env python3
"""Prepare isolated user-operated Claude acceptance; this is not a real pass."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys


REPO = Path(__file__).resolve().parents[2]


def prepare(root, style, claude=None):
    root, style = Path(root).resolve(), Path(style).resolve()
    if root.exists() and any(root.iterdir()):
        raise ValueError("scratch must be empty")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    root.chmod(0o700)
    config, work, store = (root / name for name in ("claude", "work", "style"))
    config.mkdir()
    work.mkdir()
    binary = root / "bin/style"
    binary.parent.mkdir()
    shutil.copyfile(style, binary)
    binary.chmod(0o700)
    environment = {key: value for key, value in os.environ.items()
                   if key not in ("CODEX_HOME", "CODEX_SANDBOX", "STYLE_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_EFFORT")}
    environment.update(STYLE_HOME=str(store), CLAUDE_CONFIG_DIR=str(config))
    claude_command = str(Path(claude).resolve()) if claude else "claude"
    if claude:
        (binary.parent / "claude").symlink_to(claude_command)
        environment["PATH"] = str(binary.parent) + os.pathsep + environment.get("PATH", "")
    records = []
    for package in (REPO / "example/terse-reviewer", REPO / "testdata/manual/m1b-probes"):
        command = [str(binary), "install", str(package), "--harness", "claude-code", "--yes", "--json"]
        result = subprocess.run(command, cwd=work, env=environment, capture_output=True, text=True, check=True)
        records.append(json.loads(result.stdout))

    package = root / "boundary"
    package.mkdir()
    rules = []
    for event in ("session.start", "prompt.submitted", "skill.explicit", "tool.pre", "tool.post", "tool.error"):
        marker = event.replace(".", "_").upper()
        rule = {"id": event.replace(".", "-"), "target": "event", "event": event,
                "action": "warn", "repeat": "every", "text": "M1_FINAL_CLAUDE_USER_" + marker,
                "model_text": "M1_FINAL_CLAUDE_MODEL_" + marker}
        if event == "skill.explicit":
            rule.pop("event")
            rule.update(target="skill", skill="style-boundary", at="after")
        if event.startswith("tool."):
            rule["when"] = {"conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_CLAUDE_BOUNDARY"}]}
        rules.append(rule)
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "claude-boundary", "version": "0.1.0",
        "description": "User-operated isolated Claude visibility probes", "rules": rules}, indent=2))
    result = subprocess.run([str(binary), "install", str(package), "--harness", "claude-code", "--yes", "--json"],
                            cwd=work, env=environment, capture_output=True, text=True, check=True)
    records.append(json.loads(result.stdout))
    settings = config / "settings.json"
    data = json.loads(settings.read_text())
    observer = REPO / "testdata/manual/claude_hook_observer.py"
    events = ("SessionStart", "UserPromptSubmit", "UserPromptExpansion", "PreToolUse", "PostToolUse", "PostToolUseFailure", "SubagentStart", "PostModelSwitch")
    for event in events:
        data.setdefault("hooks", {}).setdefault(event, []).append({"hooks": [{"type": "command", "command": sys.executable,
            "args": [str(observer), str(root / "hook-facts.jsonl")], "timeout": 10}]})
    settings.write_text(json.dumps(data, indent=2) + "\n")
    settings.chmod(0o600)
    # The launcher selects only user settings. A project skill is intentionally
    # outside that source selection, so install the probe in the user root.
    skill = config / "skills/style-boundary/SKILL.md"
    skill.parent.mkdir(parents=True)
    skill.write_text("---\nname: style-boundary\ndescription: M1 isolated explicit skill probe\n---\nReply only STYLE_CLAUDE_SKILL_OK. Do not use tools.\n")
    hot = root / "hot-update"
    hot.mkdir()
    (hot / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "claude-hot-update", "version": "0.1.0",
        "description": "New Read subscription installed while the UI session is running", "rules": [
            {"id": "read", "target": "tool", "names": ["Read"], "at": "after", "action": "warn",
             "text": "M1_FINAL_CLAUDE_HOT_READ_USER", "model_text": "M1_FINAL_CLAUDE_HOT_READ_MODEL"}]}, indent=2))
    (work / "probe-fixture.txt").write_text("STYLE_CLAUDE_HOT_FIXTURE\n")
    script = "#!/bin/sh\nset -eu\n" + "\n".join("export " + key + "=" + shlex.quote(value) for key, value in
        (("STYLE_HOME", str(store)), ("CLAUDE_CONFIG_DIR", str(config))))
    script += "\nunset CODEX_HOME CODEX_SANDBOX CLAUDE_EFFORT\ncd " + shlex.quote(str(work))
    script += '\nexec ' + shlex.quote(claude_command) + ' --setting-sources user --debug-file ' + shlex.quote(str(root / "claude-debug.log")) + ' --effort high "$@"\n'
    launcher = root / "launch.sh"
    launcher.write_text(script)
    launcher.chmod(0o700)
    hot_script = root / "install-hot.sh"
    hot_script.write_text("#!/bin/sh\nset -eu\n" + "\n".join("export " + key + "=" + shlex.quote(value) for key, value in
        (("STYLE_HOME", str(store)), ("CLAUDE_CONFIG_DIR", str(config)))) + "\nexec " + shlex.quote(str(binary)) +
        " install " + shlex.quote(str(hot)) + " --harness claude-code --yes --json\n")
    hot_script.chmod(0o700)
    report = {"item": "M1-FINAL-06", "status": "prepared-awaiting-real-user-session", "scratch": str(root),
              "style_binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "install": records,
              "launcher": str(launcher), "observer_log": str(root / "hook-facts.jsonl"),
              "auth_command": "env CLAUDE_CONFIG_DIR=" + shlex.quote(str(config)) + " claude auth login"}
    (root / "prepared.json").write_text(json.dumps(report, indent=2))
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True, type=Path)
    parser.add_argument("--style", required=True, type=Path)
    parser.add_argument("--claude", type=Path, help="Pin a native CLI version for repeatable acceptance")
    args = parser.parse_args()
    print(json.dumps(prepare(args.scratch, args.style, args.claude), indent=2))


if __name__ == "__main__":
    main()

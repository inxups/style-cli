#!/usr/bin/env python3
"""Observe actual thinking request settings and native hook effort separately."""

import argparse
import datetime
import json
from pathlib import Path
import shlex
import sys

from claude_live_probe import LiveClaudeProbe
from claude_probe_support import digest


def run(probe, report):
    package = probe.root / "thinking-package"
    package.mkdir()
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "claude-thinking", "version": "0.1.0",
        "description": "Observe native effort independently of thinking on/off", "rules": [
            {"id": "effort", "target": "event", "event": "tool.pre", "text": 'M1_FINAL_CLAUDE_EFFORT={{thinking_effort | default: "unknown"}}'}]}))
    report["install"] = probe.install(package)
    runner, log = probe.root / "observer.py", probe.root / "hook-effort.jsonl"
    runner.write_text('''import json,os,sys
p=json.load(sys.stdin)
keys=("hook_event_name","session_id","effort","tool_name")
with open(sys.argv[1],"a") as log: log.write(json.dumps({"payload_keys":sorted(p),"fields":{k:p[k] for k in keys if k in p},"CLAUDE_EFFORT":os.environ.get("CLAUDE_EFFORT"),"MAX_THINKING_TOKENS":os.environ.get("MAX_THINKING_TOKENS")})+"\\n")
''')
    path = probe.config / "settings.json"
    settings = json.loads(path.read_text())
    command = shlex.join([sys.executable, "-I", str(runner), str(log)])
    settings["hooks"]["PreToolUse"].append({"matcher": "Bash", "hooks": [{"type": "command", "command": command, "timeout": 10}]})
    checks = []
    for name, disabled in (("thinking-default", False), ("thinking-disabled-request", True)):
        if disabled:
            settings["alwaysThinkingEnabled"] = False
            probe.live_env["MAX_THINKING_TOKENS"] = "0"
        path.write_text(json.dumps(settings))
        call = probe.invoke_live(name, "Use Bash exactly once to execute `printf STYLE_OFF_PROBE_OK`, then reply DONE.",
            extra=("--allowedTools", "Bash", "--permission-mode", "dontAsk"), tools="Bash", markers=("M1_FINAL_CLAUDE_EFFORT=",))
        assert call["exit"] == 0 and not call["result"].get("is_error") and call["native_requests"], call
        facts = [json.loads(line) for line in log.read_text().splitlines()]
        facts = [row for row in facts if row["fields"].get("session_id") == call["session_id"]]
        assert facts
        state = json.loads((probe.store / "state" / call["session_id"] / "session.json").read_text())
        checks.append({"name": name, "disabled_settings_requested": disabled, "native_hook_facts": facts,
                       "native_request_settings": [{key: row[key] for key in ("id", "thinking", "output_config", "model")}
                                                   for row in call["native_requests"]], "state": state})
    disabled_requests = checks[-1]["native_request_settings"]
    disabled_observed = all(row["thinking"] is None or row["thinking"].get("type") == "disabled" for row in disabled_requests)
    report.update(checks=checks, passed=True, thinking_disabled_observed=disabled_observed,
                  acceptance="Observe the actual boundary; retain M1's existing no-none fallback. A requested setting is not proof that thinking was disabled.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ("style", "claude", "auth-config", "scratch"):
        parser.add_argument("--" + flag, required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = LiveClaudeProbe(args.scratch, args.style, args.claude, args.auth_config)
    report = {"date": datetime.datetime.now(datetime.timezone.utc).date().isoformat(), "item": "M1-FINAL-06-U9",
              "passed": False, "harness_version": probe.version, "style_binary_sha256": digest(probe.style.read_bytes())}
    try:
        run(probe, report)
    finally:
        report["calls"] = probe.calls
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

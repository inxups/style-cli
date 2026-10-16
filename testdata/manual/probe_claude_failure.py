#!/usr/bin/env python3
"""Verify native failure payloads and STYLE injection without extending M1 DSL."""

import argparse
import datetime
import json
from pathlib import Path
import shlex
import sys

from claude_probe_support import ClaudeProbe, body_observation, digest


MARKER = "M1_FINAL_CLAUDE_ERROR_RULE_MATCHED"


def run(probe, report):
    package = probe.root / "failure-package"
    package.mkdir()
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "claude-native-failure", "version": "0.1.0",
        "description": "Actual PostToolUseFailure with an existing input condition", "rules": [
            {"id": "failure", "target": "event", "event": "tool.error", "text": MARKER,
             "when": {"conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_CLAUDE_BOUNDARY"}]}}]}))
    report["install"] = probe.install(package)
    observer = probe.root / "observe_failure.py"
    log = probe.root / "native-failure.jsonl"
    observer.write_text('''import json,sys
p=json.load(sys.stdin)
keys=("hook_event_name","session_id","tool_name","error","tool_error","is_interrupt","effort")
with open(sys.argv[1],"a") as log: log.write(json.dumps({"payload_keys":sorted(p),"fields":{k:p[k] for k in keys if k in p}})+"\\n")
''')
    settings_path = probe.config / "settings.json"
    settings = json.loads(settings_path.read_text())
    settings["hooks"]["PostToolUseFailure"].append({"matcher": "Bash", "hooks": [{"type": "command",
        "command": shlex.join([sys.executable, "-I", str(observer), str(log)]), "timeout": 10}]})
    settings_path.write_text(json.dumps(settings))

    def response(request):
        if any(block.get("type") == "tool_result" for message in request["messages"]
               for block in message.get("content", []) if isinstance(block, dict)):
            return [{"type": "text", "text": "STYLE_LOCAL_STUB_DONE"}]
        return [{"type": "tool_use", "id": "toolu_style_failure", "name": "Bash",
                 "input": {"command": "exit 7 # STYLE_CLAUDE_BOUNDARY"}}]

    probe.responder = response
    call = probe.invoke("native-failure", "Run the harmless exit-code fixture once.",
                        extra=("--allowedTools", "Bash", "--permission-mode", "dontAsk"), tools="Bash")
    assert call["exit"] == 0 and call["result"]["subtype"] == "success", call
    facts = [json.loads(line) for line in log.read_text().splitlines()]
    assert len(facts) == 1 and facts[0]["fields"]["error"] == "Exit code 7" and "tool_error" not in facts[0]["fields"]
    observations = [{"index": index, **body_observation(probe.requests[index]["body"], MARKER)} for index in call["request_indices"]]
    assert any(row["full_body_in_request"] for row in observations)
    stats_path = probe.store / "state" / call["session_id"] / "main/stats.jsonl"
    stats = [json.loads(line) for line in stats_path.read_text().splitlines()]
    assert len(stats) == 1 and stats[0]["entry"] == "claude-native-failure/failure@tool.error" and stats[0]["result"] == "output"
    report.update(native_facts=facts, request_observations=observations, stats=stats, passed=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True)
    parser.add_argument("--claude", help="Pin an exact native CLI binary")
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = ClaudeProbe(args.scratch, args.style, args.claude)
    report = {"date": datetime.datetime.now(datetime.timezone.utc).date().isoformat(), "item": "M1-FINAL-06-FIX",
              "harness_version": probe.version, "style_binary_sha256": digest(probe.style.read_bytes()), "passed": False,
              "transport": "real native Claude CLI and Bash; loopback-only scripted Messages responses, no model inference",
              "limits": "Synthetic stub usage is excluded; actual native hook payload, request and STYLE stats are checked"}
    try:
        run(probe, report)
    finally:
        report.update(calls=probe.calls, requests=probe.report_requests(), unexpected_requests=probe.unexpected)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

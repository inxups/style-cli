#!/usr/bin/env python3
"""Observe native raw Claude hook transport against a local protocol stub."""

import argparse
import datetime
import json
from pathlib import Path
import shlex
import sys

from claude_probe_support import ClaudeProbe, body_observation, digest


COMMAND = "printf STYLE_RAW_CLAUDE_TOOL_OK"


def scripted_response(request):
    if any(block.get("type") == "tool_result" for message in request.get("messages", [])
           for block in message.get("content", []) if isinstance(block, dict)):
        return [{"type": "text", "text": "STYLE_LOCAL_STUB_DONE"}]
    return [{"type": "tool_use", "id": "toolu_style_fixture", "name": "Bash", "input": {"command": COMMAND}}]


def raw_body(name, size):
    start, end = "M1_FINAL_RAW_CLAUDE_" + name + "_BEGIN\n", "\nM1_FINAL_RAW_CLAUDE_" + name + "_END"
    middle = ("alpha beta gamma delta epsilon zeta eta theta " * (size // 40 + 1))[:size - len(start) - len(end)]
    return start + middle + end


def run(probe, report):
    runner = probe.root / "raw_hook.py"
    runner.write_text('''import hashlib, json, pathlib, sys
payload=json.load(sys.stdin)
body=pathlib.Path(sys.argv[1]).read_text()
output=json.dumps({"hookSpecificOutput":{"hookEventName":payload["hook_event_name"],"additionalContext":body}})
fields=("hook_event_name","session_id","source","model","agent_id","tool_name","effort")
with open(sys.argv[2],"a") as log:
 log.write(json.dumps({"fields":{k:payload[k] for k in fields if k in payload},"payload_keys":sorted(payload),
 "body_chars":len(body),"body_sha256":hashlib.sha256(body.encode()).hexdigest(),
 "stdout_sha256":hashlib.sha256((output+"\\n").encode()).hexdigest()})+"\\n")
print(output)
''')
    checks = []
    cases = [("session-" + str(size), "SessionStart", size) for size in (9999, 10000, 10001, 12000, 50000)]
    cases += [("prompt-12000", "UserPromptSubmit", 12000), ("pre-12000", "PreToolUse", 12000),
              ("post-12000", "PostToolUse", 12000)]
    for name, event, size in cases:
        body = raw_body(name, size)
        assert len(body) == size
        body_path, log = probe.root / (name + "-body.txt"), probe.root / (name + "-hook.jsonl")
        body_path.write_text(body)
        command = shlex.join([sys.executable, "-I", str(runner), str(body_path), str(log)])
        group = {"hooks": [{"type": "command", "command": command, "timeout": 10}]}
        if event in ("PreToolUse", "PostToolUse"):
            group["matcher"] = "Bash"
        (probe.config / "settings.json").write_text(json.dumps({"hooks": {event: [group]}}))
        needs_tool = event in ("PreToolUse", "PostToolUse")
        probe.responder = scripted_response if needs_tool else lambda request: [{"type": "text", "text": "STYLE_LOCAL_STUB_DONE"}]
        call = probe.invoke(name, "Run the fixed harmless fixture command." if needs_tool else "Reply OK.",
                            extra=("--allowedTools", "Bash", "--permission-mode", "dontAsk") if needs_tool else (),
                            tools="Bash" if needs_tool else "")
        assert call["exit"] == 0 and call["result"]["subtype"] == "success", call
        facts = [json.loads(line) for line in log.read_text().splitlines()]
        assert len(facts) == 1 and facts[0]["body_chars"] == size
        requests = [{"index": index, **body_observation(probe.requests[index]["body"], body, probe.root)} for index in call["request_indices"]]
        observed = any(row["full_body_in_request"] for row in requests)
        outcome = "accepted-full" if observed else "file-preview" if any(row["outcome"] == "file-preview" for row in requests) else "not-found-or-invalid-artifact"
        checks.append({"name": name, "event": event, "raw_body_chars": size, "body_sha256": digest(body.encode()),
                       "raw_body_path": str(body_path), "style_installed": False, "hook_facts": facts,
                       "native_cli_completed": True, "outcome": outcome,
                       "request_observations": requests})
        print(name + ": " + checks[-1]["outcome"], flush=True)
    report["native_checks"] = checks
    report["passed"] = all(check["outcome"] in ("accepted-full", "file-preview") for check in checks)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--claude", help="Pin an exact native CLI binary")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = ClaudeProbe(args.scratch, claude=args.claude)
    report = {"date": datetime.datetime.now(datetime.timezone.utc).date().isoformat(),
              "item": "M1-FINAL-08-CLAUDE-LOCAL", "harness_version": probe.version, "passed": False,
              "transport": "real Claude CLI to loopback-only scripted Anthropic Messages endpoint; no model inference",
              "limits": "Unicode character counts; synthetic stub usage is excluded; no official token or cost claim",
              "scratch": str(probe.root)}
    try:
        run(probe, report)
    finally:
        report.update(calls=probe.calls, requests=probe.report_requests(), unexpected_requests=probe.unexpected)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

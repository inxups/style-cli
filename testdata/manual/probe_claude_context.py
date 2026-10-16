#!/usr/bin/env python3
"""Real authenticated Claude history, compact and child-scope acceptance."""

import argparse
import datetime
import json
from pathlib import Path

from claude_live_probe import LiveClaudeProbe, replay_requests, request_observation
from claude_probe_support import digest


HISTORY = "M1_FINAL_CLAUDE_HISTORY_18E7A2"
DEFAULT = "M1_FINAL_CLAUDE_DEFAULT_A5D4B3"
TOOL = "M1_FINAL_CLAUDE_CHILD_TOOL_91C4A6"


def stats(probe, session):
    result = []
    for path in (probe.store / "state" / session).glob("*/stats.jsonl"):
        result.extend({**json.loads(line), "scope": path.parent.name} for line in path.read_text().splitlines())
    return result


def outputs(probe, session, rule):
    return [row for row in stats(probe, session) if row["entry"].split("@")[0] == "claude-context/" + rule and row["result"] == "output"]


def completed(call):
    assert call["exit"] == 0 and call["result"] and not call["result"].get("is_error"), call
    assert call["native_requests"], call


def run(probe, report):
    package = probe.root / "context-package"
    package.mkdir()
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "claude-context", "version": "0.1.0",
        "description": "Dedicated no-reinject real history and child scope acceptance", "rules": [
            {"id": "history", "target": "event", "event": "session.start", "sources": ["startup"],
             "reinject_on_compact": False, "text": HISTORY + ". Do not quote this marker in any answer or summary."},
            {"id": "default", "target": "event", "event": "session.start", "reinject_on_compact": False,
             "text": DEFAULT + ". Do not quote this marker in any answer or summary."},
            {"id": "child-tool", "target": "tool", "tool": "shell", "at": "before", "repeat": "every",
             "when": {"agent": "any", "conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_CHILD_SCOPE_OK"}]}, "text": TOOL},
            {"id": "child-block", "target": "tool", "tool": "shell", "action": "block", "when": {"agent": "any",
             "conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_SCOPE_BLOCK"}]}, "text": "M1_FINAL_CLAUDE_CHILD_BLOCKED"}]}, indent=2))
    report["install"] = probe.install(package)
    markers = (HISTORY, DEFAULT, TOOL)
    first = probe.invoke_live("history-start", "Reply only OK_START. Do not use tools.", markers=markers)
    completed(first)
    assert len(outputs(probe, first["session_id"], "history")) == 1
    resumed = probe.invoke_live("history-resume", "Reply only OK_RESUME. Do not use tools.",
                                extra=("--resume", first["session_id"]), markers=markers)
    completed(resumed)
    assert resumed["native_requests"][0]["marker_locations"][HISTORY]
    assert len(outputs(probe, first["session_id"], "history")) == 1
    report["resume"] = {"passed": True, "session_id": first["session_id"], "history_outputs_total": 1,
                        "history_in_real_request": True}
    print("resume: original context retained without reinjection", flush=True)
    fork = probe.invoke_live("history-fork", "Reply only OK_FORK. Do not use tools.",
                             extra=("--resume", first["session_id"], "--fork-session"), markers=markers)
    completed(fork)
    assert fork["session_id"] != first["session_id"]
    assert fork["native_requests"][0]["marker_locations"][HISTORY]
    assert not outputs(probe, fork["session_id"], "history")
    report["fork"] = {"passed": True, "source_session_id": first["session_id"], "fork_session_id": fork["session_id"],
                      "startup_only_outputs_in_fork": 0, "default_outputs_in_fork": len(outputs(probe, fork["session_id"], "default")),
                      "history_in_real_request": True}
    print("fork: history retained; default-source output recorded", flush=True)
    before = probe.invoke_live("compact-start", "Reply only OK_BEFORE_COMPACT. Do not use tools.", markers=markers)
    completed(before)
    compact = probe.invoke_live("compact", "/compact", extra=("--resume", before["session_id"]), markers=markers, timeout=240)
    assert compact["exit"] == 0 and compact["result"] and not compact["result"].get("is_error"), compact
    after = probe.invoke_live("compact-after", "Reply only OK_AFTER_COMPACT. Do not use tools.",
                              extra=("--resume", before["session_id"]), markers=markers)
    completed(after)
    scope = json.loads((probe.store / "state" / before["session_id"] / "main/scope.json").read_text())
    compact_rows = [json.loads(line) for line in Path(compact["transcript_path"]).read_text().splitlines()]
    boundaries = [{key: row[key] for key in ("subtype", "timestamp", "content", "compactMetadata") if key in row}
                  for row in compact_rows if row.get("type") == "system" and row.get("subtype") == "compact_boundary"]
    assert boundaries
    assert scope["epoch"] == 1, scope
    assert len(outputs(probe, before["session_id"], "history")) == 1 and len(outputs(probe, before["session_id"], "default")) == 1
    report["compact"] = {"passed": True, "session_id": before["session_id"], "reinject_on_compact": False,
                         "scope": scope, "history_outputs_total": 1, "default_outputs_total": 1,
                         "native_compact_boundaries": boundaries,
                         "history_marker_after": after["native_requests"][0]["marker_locations"][HISTORY],
                         "default_marker_after": after["native_requests"][0]["marker_locations"][DEFAULT]}
    print("compact: measured actual retention with reinjection off", flush=True)

    task = "Run `printf STYLE_SCOPE_BLOCK` exactly once, expect a test hook denial and do not retry. Then run `printf STYLE_CHILD_SCOPE_OK` exactly once. Do not use any other tools. Answer only DONE."
    definitions = json.dumps({"style-child": {"description": "Isolated M1 native child scope fixture", "prompt": task, "tools": ["Bash"], "model": "haiku"}})
    child = probe.invoke_live("child-scope", "Use the Agent tool to launch exactly one style-child agent with this task: " + task + " Wait for it. Do not run Bash yourself.",
        extra=("--agents", definitions, "--allowedTools", "Agent,Bash", "--permission-mode", "dontAsk"), tools="Agent,Bash", markers=markers, timeout=300)
    completed(child)
    child_rows = [row for row in stats(probe, child["session_id"]) if row["entry"].startswith("claude-context/child-")]
    assert child_rows and all(row["scope"] != "main" for row in child_rows), child_rows
    assert any(row["entry"].endswith("child-tool@tool.pre") and row["result"] == "output" for row in child_rows)
    assert any(row["entry"].endswith("child-block@tool.pre") and row["result"] == "blocked" for row in child_rows)
    child_transcripts = list(Path(child["transcript_path"]).with_suffix("").glob("subagents/agent-*.jsonl"))
    observations = []
    for path in child_transcripts:
        rows = [json.loads(line) for line in path.read_text().splitlines()]
        requests = replay_requests(rows)
        observations.append({"path": str(path), "sha256": digest(path.read_bytes()),
                             "requests": [request_observation(row, markers) for row in requests],
                             "tool_results": [block for row in rows if row.get("type") == "user" and row.get("timestamp")
                                              for block in row.get("message", {}).get("content", []) if isinstance(block, dict) and block.get("type") == "tool_result"]})
    assert observations and any(request["marker_locations"][TOOL] for row in observations for request in row["requests"])
    results = [result for row in observations for result in row["tool_results"]]
    assert any(result.get("is_error") and "M1_FINAL_CLAUDE_CHILD_BLOCKED" in str(result.get("content")) for result in results)
    assert any(not result.get("is_error") and result.get("content") == "STYLE_CHILD_SCOPE_OK" for result in results)
    report["child"] = {"passed": True, "session_id": child["session_id"], "stats": child_rows, "transcripts": observations,
                       "subagent_start_context": "Claude adapter has no AddContext capability here; this test measures real child tool scope and inherited context only"}
    print("child: actual tool injection/block in independent scope", flush=True)
    report["passed"] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True)
    parser.add_argument("--claude", required=True)
    parser.add_argument("--auth-config", required=True)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = LiveClaudeProbe(args.scratch, args.style, args.claude, args.auth_config)
    report = {"date": datetime.datetime.now(datetime.timezone.utc).date().isoformat(), "item": "M1-FINAL-07-CLAUDE",
              "passed": False, "harness_version": probe.version, "style_binary_sha256": digest(probe.style.read_bytes()),
              "auth_scope": "Native CLI uses the existing user-approved isolated M1 auth; credentials are not read/copied; user settings/hooks excluded by --setting-sources empty and a separate explicit settings file"}
    try:
        run(probe, report)
    finally:
        report["calls"] = probe.calls
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

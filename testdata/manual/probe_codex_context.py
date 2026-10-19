#!/usr/bin/env python3
"""Measure native Codex history and child scope, with compact reinjection off."""

import argparse
import json
from pathlib import Path
import time

from codex_probe_support import AppServer, NativeProbe, digest


HISTORY = "M1_FINAL_HISTORY_7C942E"
DEFAULT = "M1_FINAL_DEFAULT_SOURCE_914B2A"
CHILD = "M1_FINAL_CHILD_CONTEXT_4129AC"
TOOL = "M1_FINAL_CHILD_TOOL_CONTEXT_7284BC"


def child_tool_results(rows):
    allowed, blocked = [], False
    for row in rows:
        payload = row.get("payload", {})
        if row.get("type") != "response_item" or payload.get("type") not in ("function_call_output", "custom_tool_call_output"):
            continue
        output = payload.get("output", "")
        texts = [part.get("text", "") for part in output if isinstance(part, dict)] if isinstance(output, list) else [output]
        for text in texts:
            blocked = blocked or "Command blocked by PreToolUse hook: M1_FINAL_CHILD_BLOCKED" in text
            try:
                value = json.loads(text)
            except (ValueError, TypeError):
                continue
            if isinstance(value, dict) and value.get("output") == "STYLE_CHILD_SCOPE_OK":
                allowed.append({"exit_code": value.get("exit_code"), "output": value["output"]})
    return {"allowed_printf_results": allowed, "blocked_printf_hook_result": blocked}


def fixture(probe):
    package = probe.root / "context-probe"
    package.mkdir()
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "context-probe", "version": "0.1.0",
        "description": "Isolated history retention and child scope probes", "rules": [
            {"id": "history", "target": "event", "event": "session.start", "sources": ["startup"],
             "reinject_on_compact": False, "text": HISTORY + ". Preserve this instruction in context; do not quote it in your answers."},
            {"id": "default-source", "target": "event", "event": "session.start", "reinject_on_compact": False,
             "text": DEFAULT + ". Do not quote this marker in answers."},
            {"id": "child", "target": "event", "event": "subagent.start", "text": CHILD + ". Do not quote this marker in answers."},
            {"id": "child-tool", "target": "tool", "tool": "shell", "at": "before", "repeat": "every", "reason": "Observe each child scope independently",
             "when": {"agent": "any", "conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_CHILD_SCOPE_OK"}]}, "text": TOOL},
            {"id": "child-block", "target": "tool", "tool": "shell", "action": "block", "when": {"agent": "any",
                "conditions": [{"field": "tool.input.command", "op": "contains", "pattern": "STYLE_SCOPE_BLOCK"}]}, "text": "M1_FINAL_CHILD_BLOCKED"}]}, indent=2))
    probe.evidence_markers = (HISTORY, DEFAULT, CHILD, TOOL)
    return probe.style_command("install", str(package), "--harness", "codex", "--route", "codex=codex-user", confirm=True)


def output_rows(call, rule):
    return [row for row in call["stats"] if row["entry"].split("@")[0] == "context-probe/" + rule and row["result"] == "output"]


def run(probe, report):
    report["install"] = fixture(probe)
    first = probe.invoke("history-start", "Reply only OK_START. Do not use tools.")
    assert first["exit"] == 0 and len(output_rows(first, "history")) == 1
    resumed = probe.invoke("history-resume", "Reply only OK_RESUME. Do not use tools.", resume=first["session_id"])
    assert resumed["exit"] == 0 and len(output_rows(resumed, "history")) == 1
    assert resumed["requests"][0]["marker_locations"][HISTORY]
    report["resume"] = {"passed": True, "session_id": first["session_id"], "history_outputs_total": 1,
                        "history_in_resumed_request": True}
    print("resume: history retained, no reinjection", flush=True)

    server = AppServer(probe)
    try:
        start, events = len(probe.requests), len(server.events)
        fork = server.rpc("thread/fork", {"threadId": first["session_id"], "cwd": str(probe.work),
                                          "approvalPolicy": "never", "sandbox": "danger-full-access"})["thread"]["id"]
        server.turn(fork, "Reply only OK_FORK. Do not use tools.")
        fork_call = probe.capture("history-fork", fork, server.events[events:], start)
        assert fork_call["requests"][0]["marker_locations"][HISTORY]
        assert not output_rows(fork_call, "history")
        report["fork"] = {"passed": True, "source_session_id": first["session_id"], "fork_session_id": fork,
                          "history_retained_without_reinjection": True,
                          "default_source_outputs": len(output_rows(fork_call, "default-source"))}
        print("fork: history retained; measured default source output", flush=True)

        start, events = len(probe.requests), len(server.events)
        thread = server.rpc("thread/start", {"cwd": str(probe.work), "approvalPolicy": "never", "sandbox": "danger-full-access"})["thread"]["id"]
        server.turn(thread, "Reply only OK_BEFORE_COMPACT. Do not use tools.")
        before = len(probe.requests)
        server.rpc("thread/compact/start", {"threadId": thread})
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            message = server.next()
            if message.get("method") == "turn/completed" and message.get("params", {}).get("threadId") == thread:
                break
        else:
            raise RuntimeError("compact did not complete")
        after = len(probe.requests)
        server.turn(thread, "Reply only OK_AFTER_COMPACT. Do not use tools.")
        compact = probe.capture("history-compact-no-reinject", thread, server.events[events:], start)
        assert len(output_rows(compact, "history")) == 1 and len(output_rows(compact, "default-source")) == 1
        scope = json.loads((probe.store / "state" / thread / "main/scope.json").read_text())
        assert scope["epoch"] == 1
        report["compact"] = {"passed": True, "session_id": thread, "reinject_on_compact": False, "epoch": scope["epoch"],
                             "history_outputs_total": 1, "default_source_outputs_total": 1,
                             "compact_request_count": after - before,
                             "history_marker_locations_after_compact": probe.requests[after]["marker_locations"][HISTORY],
                             "default_marker_locations_after_compact": probe.requests[after]["marker_locations"][DEFAULT]}
        print("compact: measured retention with reinjection disabled", flush=True)
    finally:
        report["app_server_notifications"] = server.events
        server.close()

    prompt = """This is an isolated M1 child scope acceptance test. Use the native spawn_agent tool to create two direct children, one with fork_turns=false and one with fork_turns=true, naming them fresh_child and fork_child. Give each this exact task: first run `printf STYLE_SCOPE_BLOCK` exactly once (a harmless print expected to be denied by a test hook), then run `printf STYLE_CHILD_SCOPE_OK` exactly once; do not retry a denied command. The fresh_child must also spawn one grandchild with fork_turns=false and task run `printf STYLE_SCOPE_BLOCK` once then `printf STYLE_CHILD_SCOPE_OK` once; wait for it. Do not let the fork_child spawn further children. Wait for both children and answer only DONE. Do not run any shell tools yourself. Use fork_turns with the value supported by your native schema (false/true or none/all)."""
    nested = probe.invoke("child-scopes", prompt, ["-c", "features.multi_agent=true", "-c", "agents.max_depth=3", "-c", "agents.max_threads=6"], timeout=600)
    report["child_run"] = {"exit": nested["exit"], "session_id": nested["session_id"]}
    scopes = []
    for path in sorted((probe.store / "state").rglob("stats.jsonl")):
        rows = [json.loads(line) for line in path.read_text().splitlines()]
        if any(row.get("entry", "").startswith("context-probe/child") for row in rows):
            scopes.append({"path": str(path.relative_to(probe.store)), "agent_id": path.parent.name, "rows": rows,
                           "scope": json.loads(path.with_name("scope.json").read_text())})
    rollouts = []
    spawn_options = []
    for path in sorted((probe.home / "sessions").rglob("*.jsonl")):
        rows = [json.loads(line) for line in path.read_text().splitlines()]
        for row in rows:
            payload = row.get("payload", {})
            if payload.get("type") == "function_call" and payload.get("name") == "spawn_agent":
                arguments = json.loads(payload["arguments"])
                spawn_options.append({key: arguments[key] for key in ("task_name", "fork_turns") if key in arguments})
        meta = next((row["payload"] for row in rows if row.get("type") == "session_meta"), {})
        contexts = [row for row in rows if row.get("type") == "response_item" and row.get("payload", {}).get("role") == "developer"
                    and "hooks.additional_context" in json.dumps(row)]
        if "subagent" in json.dumps(meta.get("source")):
            rollouts.append({"session_id": meta.get("id"), "source": meta.get("source"), "hook_context": contexts,
                             "tool_results": child_tool_results(rows)})
    report["child_scopes"], report["child_rollouts"] = scopes, rollouts
    report["native_spawn_options"], report["calls"] = spawn_options, probe.calls
    verify_children(report)
    report["child_run"]["passed"] = True
    print("children: fresh, fork and nested context/tool/block scopes verified", flush=True)
    report["passed"] = True


def verify_children(report):
    scopes, rollouts = report["child_scopes"], report["child_rollouts"]
    assert report["child_run"]["exit"] == 0 and len(rollouts) == 3 and len(scopes) == 3
    for scope in scopes:
        assert Path(scope["path"]).parent.name != "main"
        for suffix, result in (("child@subagent.start", "output"), ("child-tool@tool.pre", "output"), ("child-block@tool.pre", "blocked")):
            assert any(row["entry"] == "context-probe/" + suffix and row["result"] == result for row in scope["rows"]), scope["path"]
    for rollout in rollouts:
        assert CHILD in json.dumps(rollout["hook_context"]) and TOOL in json.dumps(rollout["hook_context"])
        if "tool_results" in rollout:
            assert rollout["tool_results"]["blocked_printf_hook_result"]
            assert rollout["tool_results"]["allowed_printf_results"] == [{"exit_code": 0, "output": "STYLE_CHILD_SCOPE_OK"}]
    paths = {row["source"]["subagent"]["thread_spawn"]["agent_path"] for row in rollouts}
    assert paths == {"/root/fresh_child", "/root/fork_child", "/root/fresh_child/grandchild"}
    if report.get("native_spawn_options"):
        assert {row["task_name"]: row["fork_turns"] for row in report["native_spawn_options"]} == {
            "fresh_child": "none", "fork_child": "all", "grandchild": "none"}
    parent = next(call for call in report.get("calls", []) if call["name"] == "child-scopes") if report.get("calls") else None
    if parent:
        assert not any(row["entry"].startswith("context-probe/child") for row in parent["stats"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style")
    parser.add_argument("--scratch")
    parser.add_argument("--verify-report", type=Path, help="Recheck retained real observations after a collector-only failure")
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if args.verify_report:
        report = json.loads(args.verify_report.read_text())
        verify_children(report)
        assert all(report[key]["passed"] for key in ("resume", "fork", "compact"))
        report["collector_continuation"] = {"source_sha256": digest(args.verify_report.read_bytes()),
            "reason": "Original collector read agent_id from scope.json, whose schema omits it; corrected validator derives scope from the persisted directory path. No native runs repeated or changed."}
        report["child_run"]["passed"] = True
        report["passed"] = True
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("Retained real observations verified; collector continuation recorded")
        return
    if not args.style or not args.scratch:
        parser.error("native execution requires --style and --scratch")
    probe = NativeProbe(args.scratch, args.style)
    report = {"date": "2026-10-16", "item": "M1-FINAL-07-CODEX", "passed": False,
              "style_binary_sha256": digest(Path(args.style).read_bytes()), "harness_version": probe.native_command("--version")}
    try:
        run(probe, report)
    finally:
        report["calls"] = probe.calls
        args.output.write_text(json.dumps(probe.sanitize(report), ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

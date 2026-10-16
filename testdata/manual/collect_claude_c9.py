#!/usr/bin/env python3
"""Cross-check C9 command-hook timings against timestamped native tool rows."""

import argparse
from collections import Counter
import datetime
import hashlib
import json
from pathlib import Path
import shlex

from measure_claude_hooks import COMPLETION, collect


def workload(rows, expected=50):
    calls, results = {}, {}
    ignored_snapshots = 0
    for row in rows:
        content = row.get("message", {}).get("content", [])
        if not isinstance(content, list):
            continue
        for block in content:
            if not isinstance(block, dict) or block.get("type") not in ("tool_use", "tool_result"):
                continue
            if not row.get("timestamp") or row.get("type") not in ("assistant", "user"):
                ignored_snapshots += 1
                continue
            at = row["timestamp"]
            if block["type"] == "tool_use":
                parts = shlex.split(block.get("input", {}).get("command", ""))
                # Both escaped transcript spellings occurred in retained
                # native fixtures. Neither permits a loop or multiple calls.
                valid = block.get("name") == "Bash" and parts in (["printf", "STYLE_M1B_C9\\n"], ["printf", "STYLE_M1B_C9\\\\n"])
                value = {"id": block["id"], "start": at, "assistant_message_id": row["message"].get("id"),
                         "fixed_printf": valid, "command": block.get("input", {}).get("command")}
                calls.setdefault(block["id"], value)
            else:
                value = {"end": at, "is_error": block.get("is_error", False),
                         "marker_output": isinstance(block.get("content"), str) and block["content"].strip() == "STYLE_M1B_C9"}
                results.setdefault(block["tool_use_id"], value)
    ordered = sorted(calls.values(), key=lambda row: row["start"])
    complete = [{**call, **results.get(call["id"], {})} for call in ordered]
    overlaps = []
    for previous, current in zip(complete, complete[1:]):
        if previous.get("end") and current["start"] < previous["end"]:
            overlaps.append({"previous": previous["id"], "next": current["id"]})
    valid = (len(calls) == expected and set(calls) == set(results) and not overlaps
             and all(row["fixed_printf"] and not row.get("is_error", True) and row.get("marker_output")
                     and row["end"] >= row["start"] for row in complete))
    return {"passed": valid, "expected_calls": expected, "unique_calls": len(calls), "unique_results": len(results),
            "assistant_messages_with_tools": len({row["assistant_message_id"] for row in complete}),
            "ignored_non_timed_snapshots": ignored_snapshots, "overlaps": overlaps, "calls": complete}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True, type=Path)
    parser.add_argument("--transcript", required=True, type=Path)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--completion-output", required=True, type=Path)
    args = parser.parse_args()
    raw = args.log.read_bytes()
    rows = [json.loads(line) for line in args.transcript.read_text().splitlines()]
    timing = collect(raw, str(args.binary), 100, 50)
    work = workload(rows)
    versions = sorted({row["version"] for row in rows if row.get("version")})
    models = Counter(row.get("params", {}).get("model") for row in rows if row.get("type") == "api-request")
    completions = [line for line in raw.decode().splitlines()
                   if (match := COMPLETION.fullmatch(line)) and match["command"].startswith(str(args.binary) + " ")
                   and "--owner=style" in match["command"]]
    args.completion_output.write_text("\n".join(completions) + "\n")
    report = {"date": datetime.datetime.now(datetime.timezone.utc).date().isoformat(), "item": "M1-FINAL-06-C9",
              "passed": timing["status"] == "within-budget" and work["passed"],
              "style_binary_sha256": hashlib.sha256(args.binary.read_bytes()).hexdigest(),
              "native_versions": versions, "request_models": dict(models), "workload": work, "timing": timing,
              "all_slow_samples_ge_50ms": [row for row in timing["samples"] if row["duration_ms"] >= 50],
              "transcript_sha256": hashlib.sha256(args.transcript.read_bytes()).hexdigest(),
              "completion_sha256": hashlib.sha256(args.completion_output.read_bytes()).hexdigest(),
              "limits": "Actual user-operated native session; all STYLE completions retained, including startup and model switch. Non-timestamped API snapshots are not new tool executions."}
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"passed": report["passed"], "workload_passed": work["passed"], "calls": work["unique_calls"],
                      "overlaps": len(work["overlaps"]), "timing": timing["overall"], "native_versions": versions}))
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())

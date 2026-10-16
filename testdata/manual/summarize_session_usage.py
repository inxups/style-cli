#!/usr/bin/env python3
"""Separate real session usage from STYLE rule estimates without copying text."""

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def summarize_stats(rows):
    rules = {}
    for row in rows:
        rule = row["entry"].split("@")[0]
        total = rules.setdefault(rule, {"outcome_counts": Counter(), "inject_outputs": 0, "warn_outputs": 0,
                                       "output_est_tokens": 0, "blocked_est_tokens": 0, "unique_output_bodies": set()})
        result = row["result"]
        total["outcome_counts"][result] += 1
        if result == "output":
            total["inject_outputs"] += row["action"] == "inject"
            total["warn_outputs"] += row["action"] == "warn"
            total["output_est_tokens"] += row.get("est_tokens", 0)
            total["unique_output_bodies"].add(row.get("body_sha256"))
        elif result == "blocked":
            total["blocked_est_tokens"] += row.get("est_tokens", 0)
    for total in rules.values():
        total["unique_output_bodies"] = len(total["unique_output_bodies"])
        outcomes = total["outcome_counts"]
        attempts = outcomes["output"] + outcomes["dedup"]
        total["dedup_fraction_of_output_or_dedup"] = outcomes["dedup"] / attempts if attempts else None
    outcomes = Counter(row["result"] for row in rows)
    attempts = outcomes["output"] + outcomes["dedup"]
    return {"rows": len(rows), "outcome_counts": outcomes,
            "dedup_fraction_of_output_or_dedup": outcomes["dedup"] / attempts if attempts else None,
            "inject_outputs": sum(rule["inject_outputs"] for rule in rules.values()),
            "warn_outputs": sum(rule["warn_outputs"] for rule in rules.values()),
            "output_est_tokens": sum(rule["output_est_tokens"] for rule in rules.values()), "rules": rules}


CAMEL = {"inputTokens": "input_tokens", "cachedInputTokens": "cached_input_tokens", "cacheWriteInputTokens": "cache_write_input_tokens",
         "outputTokens": "output_tokens", "reasoningOutputTokens": "reasoning_output_tokens", "totalTokens": "total_tokens"}


def codex_usage(call):
    snapshots = [row["params"]["tokenUsage"]["total"] for row in call["events"] if row.get("method") == "thread/tokenUsage/updated"]
    if snapshots:
        return {"source": "last app-server cumulative total; never sum cumulative snapshots", "snapshots": len(snapshots),
                "usage": {CAMEL[key]: value for key, value in snapshots[-1].items() if key in CAMEL}}
    turns = [row["usage"] for row in call["events"] if row.get("type") == "turn.completed" and "usage" in row]
    if not turns:
        return {"source": "no completed-turn usage", "usage": None}
    return {"source": "last exec cumulative session usage; never sum completed-turn snapshots", "snapshots": len(turns),
            "usage": {key: value for key, value in turns[-1].items() if key in CAMEL.values() and type(value) is int}}


def codex_report(path):
    source = json.loads(path.read_text())
    calls = source["calls"]
    grouped = {}
    for call in calls:
        grouped.setdefault(call["session_id"], []).append(call)
    sessions, all_stats = [], []
    totals, coverage = Counter(), Counter()
    for session_id, session_calls in grouped.items():
        # Each call records an append-only stats snapshot, including earlier
        # resumed turns. Fail closed if a later snapshot lost earlier rows.
        rows = []
        usage = {"source": "no completed-turn usage", "usage": None}
        for call in session_calls:
            assert call["stats"][:len(rows)] == rows, "session stats are not append-only"
            rows = call["stats"]
            snapshot = codex_usage(call)
            if snapshot["usage"] is not None:
                usage = snapshot
        all_stats.extend(rows)
        if usage["usage"] is not None:
            totals.update(usage["usage"])
            coverage.update(usage["usage"].keys())
        requests = [row for call in session_calls for row in call["requests"]]
        sessions.append({"session_id": session_id, "cases": [call["name"] for call in session_calls],
                         "native_exits": [call["exit"] for call in session_calls],
                         "observed_http_requests": len(requests), "models": sorted({row["model"] for row in requests}),
                         "style": summarize_stats(rows), **usage})
    return {"source": path.name, "source_sha256": sha(path), "provider": "codex", "route": source["route"],
            "scope": "Real acceptance sessions using the configured custom Responses provider; resume grouped by session ID, final cumulative usage/stats only. HTTP count includes observed attempts. Native usage may omit failed or compact requests; not official billing.",
            "sessions": sessions, "session_count": len(sessions), "observed_http_requests": sum(row["observed_http_requests"] for row in sessions),
            "sessions_with_usage": sum(row["usage"] is not None for row in sessions),
            "reported_usage": {key: value for key, value in totals.items() if coverage[key] == sum(row["usage"] is not None for row in sessions)},
            "usage_field_session_counts": coverage, "style": summarize_stats(all_stats)}


def claude_report(transcript, stats_path):
    rows = [json.loads(line) for line in transcript.read_text().splitlines()]
    selected, snapshots = {}, Counter()
    for row in rows:
        message = row.get("message")
        if row.get("type") != "assistant" or not isinstance(message, dict) or not message.get("usage"):
            continue
        if message.get("model") == "<synthetic>":
            continue
        key = row.get("requestId") or message.get("id")
        if not key:
            continue
        snapshots[key] += 1
        value = message["usage"]
        allowed = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens")
        usage = {key: value.get(key, 0) for key in allowed}
        usage.update({key: value.get("cache_creation", {}).get(key, 0) for key in ("ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens")})
        selected[key] = {"request_id": key, "message_id": message.get("id"), "model": message.get("model"), "at": row.get("timestamp"), "usage": usage}
    totals, models = Counter(), {}
    for row in selected.values():
        totals.update(row["usage"])
        models.setdefault(row["model"], Counter()).update(row["usage"])
    # Claude input_tokens excludes the two cache categories. TTL breakdowns
    # are subsets of cache creation and must not be added a second time.
    totals["total_input_including_cache"] = sum(totals[key] for key in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"))
    stats = [json.loads(line) for line in stats_path.read_text().splitlines()]
    return {"source": transcript.name, "source_sha256": sha(transcript), "stats_sha256": sha(stats_path), "provider": "claude-code",
            "scope": "Whole retained main-session transcript; usage-bearing responses only, no HTTP retry count or separate subagent transcript coverage.",
            "usage_snapshot_rows": sum(snapshots.values()), "observed_usage_response_requests": len(selected),
            "duplicate_snapshot_rows_removed": sum(snapshots.values()) - len(selected),
            "requests": list(selected.values()), "reported_usage": totals, "reported_usage_by_model": models, "style": summarize_stats(stats)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex-report", action="append", type=Path, default=[])
    parser.add_argument("--claude-transcript", action="append", type=Path, default=[])
    parser.add_argument("--claude-stats", action="append", type=Path, default=[])
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if len(args.claude_transcript) != len(args.claude_stats):
        parser.error("each Claude transcript needs its matching stats path")
    report = {"item": "M1-FINAL-10-U-T4", "date": "2026-10-16", "datasets": [codex_report(path) for path in args.codex_report] +
              [claude_report(transcript, stats) for transcript, stats in zip(args.claude_transcript, args.claude_stats)],
              "limits": "Rule estimates are not exact rule tokens and are not subtracted from session usage. Acceptance workload is not a representative pricing benchmark. Defaults remain 60 requests/3 triggers and budgets 400/1200/600."}
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    for dataset in report["datasets"]:
        print(dataset["provider"], dataset.get("route", dataset["source"]), "requests", dataset.get("observed_http_requests", dataset.get("observed_usage_response_requests")),
              "inject", dataset["style"]["inject_outputs"], "warn", dataset["style"]["warn_outputs"], "dedup", dataset["style"]["dedup_fraction_of_output_or_dedup"])


if __name__ == "__main__":
    main()

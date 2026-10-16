#!/usr/bin/env python3
"""Summarize real STYLE command-hook timings from Codex OTLP/HTTP JSON trace exports."""

import argparse
import hashlib
import json
import math
from datetime import datetime, timezone
from pathlib import Path

from measure_claude_hooks import summarize


SPAN = "codex.hooks.command"


def attributes(span):
    values = {}
    for item in span.get("attributes", []):
        value = item.get("value", {})
        values[item["key"]] = next(iter(value.values()), None) if value else None
    return values


def iso(nanos):
    return datetime.fromtimestamp(nanos / 1e9, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")


def parse_time(value):
    if value is None:
        return None
    return int(datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1e9)


def overlaps(samples):
    ordered = sorted(samples, key=lambda sample: sample["start_ns"])
    count = 0
    for previous, current in zip(ordered, ordered[1:]):
        if current["start_ns"] < previous["end_ns"]:
            count += 1
    return count


def collect(raw, source, gate_event, minimum, threshold, since=None, until=None):
    samples = []
    malformed = 0
    for line in raw.decode("utf-8", errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            malformed += 1
            continue
        body = record.get("body") or {}
        for resource in body.get("resourceSpans", []):
            for scope in resource.get("scopeSpans", []):
                for span in scope.get("spans", []):
                    if span.get("name") != SPAN:
                        continue
                    attrs = attributes(span)
                    if attrs.get("hook.handler_type") != "command" or attrs.get("hook.source") != source:
                        continue
                    try:
                        start, end = int(span["startTimeUnixNano"]), int(span["endTimeUnixNano"])
                    except (KeyError, TypeError, ValueError):
                        malformed += 1
                        continue
                    if (since is not None and start < since) or (until is not None and start > until):
                        continue
                    samples.append({
                        "at": iso(start),
                        "event": attrs.get("hook.event_name"),
                        "outcome": attrs.get("hook.command_outcome"),
                        "trace_id": span.get("traceId"),
                        "duration_ms": round((end - start) / 1e6, 3),
                        "start_ns": start,
                        "end_ns": end,
                    })
    samples.sort(key=lambda sample: sample["start_ns"])
    gate = [sample for sample in samples if sample["event"] == gate_event]
    gate_summary = summarize(gate)
    errors = sum(sample["outcome"] != "completed" for sample in samples)
    if malformed:
        status = "malformed-export"
    elif errors:
        status = "hook-errors"
    elif len(gate) < minimum:
        status = "insufficient-samples"
    elif gate_summary["p95_ms"] >= threshold:
        status = "over-budget"
    else:
        status = "within-budget"
    return {
        "source_sha256": hashlib.sha256(raw).hexdigest(),
        "measurement": "Codex codex.hooks.command span duration exported by the harness; includes process spawn and I/O; no matched samples discarded",
        "percentile": "nearest rank: sorted[ceil(0.95 * n) - 1]",
        "hook_source": source,
        "gate_event": gate_event,
        "minimum_samples": minimum,
        "threshold_ms": threshold,
        "status": status,
        "hook_errors": errors,
        "malformed_records": malformed,
        "gate": gate_summary,
        "gate_overlapping_pairs": overlaps(gate),
        "overall": summarize(samples),
        "by_event": {
            event: summarize([sample for sample in samples if sample["event"] == event])
            for event in sorted({sample["event"] for sample in samples if sample["event"]})
        },
        "samples": [{key: sample[key] for key in ("at", "event", "outcome", "trace_id", "duration_ms")} for sample in samples],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("export", type=Path, help="JSONL file with one OTLP/HTTP JSON request per line")
    parser.add_argument("--source", default="user", help="hook.source to include: user (codex-user) or plugin (codex-plugin)")
    parser.add_argument("--event", default="PreToolUse", help="event whose p95 decides the status")
    parser.add_argument("--since", help="ISO-8601 UTC start bound for span start times")
    parser.add_argument("--until", help="ISO-8601 UTC end bound for span start times")
    parser.add_argument("--min-samples", type=int, default=100)
    parser.add_argument("--threshold-ms", type=float, default=150)
    args = parser.parse_args()
    if args.min_samples < 1 or not math.isfinite(args.threshold_ms) or args.threshold_ms <= 0:
        parser.error("sample count and threshold must be positive")
    report = collect(
        args.export.read_bytes(), args.source, args.event, args.min_samples, args.threshold_ms,
        parse_time(args.since), parse_time(args.until),
    )
    report["source"] = str(args.export.resolve())
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()

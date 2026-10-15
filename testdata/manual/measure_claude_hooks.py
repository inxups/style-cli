#!/usr/bin/env python3
"""Summarize real STYLE command-hook timings from a Claude debug log."""

import argparse
import hashlib
import json
import math
import re
import statistics
from pathlib import Path


COMPLETION = re.compile(
    r"^(?P<at>\S+) \[DEBUG\] Hooks: (?P<event>.+?) "
    r"\[(?P<command>.+)\] finished with status (?P<exit>-?\d+) "
    r"\((?P<ms>\d+(?:\.\d+)?)ms\)$"
)


def summarize(samples):
    values = sorted(sample["duration_ms"] for sample in samples)
    if not values:
        return {"count": 0, "median_ms": None, "p95_ms": None, "max_ms": None}
    return {
        "count": len(values),
        "median_ms": statistics.median(values),
        "p95_ms": values[(len(values) * 95 + 99) // 100 - 1],
        "max_ms": values[-1],
    }


def collect(raw, binary, minimum, threshold):
    samples = []
    unparsed = 0
    for line in raw.decode("utf-8", errors="replace").splitlines():
        if "[DEBUG] Hooks: " not in line or "finished with status" not in line:
            continue
        if not re.search(r"(?:^|\s)--owner=style(?:\s|\])", line):
            continue
        match = COMPLETION.fullmatch(line)
        if not match:
            unparsed += 1
            continue
        if binary and not match["command"].startswith(binary + " "):
            continue
        samples.append({
            "at": match["at"],
            "event": match["event"],
            "exit_code": int(match["exit"]),
            "duration_ms": float(match["ms"]),
        })
    overall = summarize(samples)
    errors = sum(sample["exit_code"] != 0 for sample in samples)
    if unparsed:
        status = "unparsed-completions"
    elif errors:
        status = "hook-errors"
    elif len(samples) < minimum:
        status = "insufficient-samples"
    elif overall["p95_ms"] >= threshold:
        status = "over-budget"
    else:
        status = "within-budget"
    return {
        "source_sha256": hashlib.sha256(raw).hexdigest(),
        "measurement": "Claude-reported command-hook duration; includes harness timing boundaries; no matched samples discarded",
        "percentile": "nearest rank: sorted[ceil(0.95 * n) - 1]",
        "minimum_samples": minimum,
        "threshold_ms": threshold,
        "status": status,
        "hook_errors": errors,
        "unparsed_STYLE_completions": unparsed,
        "overall": overall,
        "by_event": {
            event: summarize([sample for sample in samples if sample["event"] == event])
            for event in sorted({sample["event"] for sample in samples})
        },
        "samples": samples,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("log", type=Path)
    parser.add_argument("--binary", help="Only include commands starting with this installed binary path")
    parser.add_argument("--min-samples", type=int, default=100)
    parser.add_argument("--threshold-ms", type=float, default=50)
    args = parser.parse_args()
    if args.min_samples < 1 or not math.isfinite(args.threshold_ms) or args.threshold_ms <= 0:
        parser.error("sample count and threshold must be positive")
    report = collect(args.log.read_bytes(), args.binary, args.min_samples, args.threshold_ms)
    report["source"] = str(args.log.resolve())
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()

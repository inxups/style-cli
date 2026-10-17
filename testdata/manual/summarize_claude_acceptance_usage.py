#!/usr/bin/env python3
"""Read only hash-pinned acceptance transcripts; export whitelisted usage."""

import argparse
from collections import Counter
import json
from pathlib import Path
import re

from calibrate_tokens import sha


FIELDS = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens")


def transcript_refs(value):
    if isinstance(value, dict):
        path = value.get("path")
        if isinstance(path, str) and re.fullmatch(r"[0-9a-f-]{36}\.jsonl", Path(path).name):
            yield path, value["sha256"]
        for child in value.values():
            yield from transcript_refs(child)
    elif isinstance(value, list):
        for child in value:
            yield from transcript_refs(child)


def summarize(paths):
    refs, sources = {}, []
    for path in paths:
        raw = path.read_bytes()
        report = json.loads(raw)
        if report.get("passed") is not True:
            raise ValueError("Acceptance source has not passed")
        sources.append({"path": str(path), "sha256": sha(raw)})
        for transcript, digest in transcript_refs(report):
            if transcript in refs and refs[transcript] != digest:
                raise ValueError("Conflicting transcript hashes")
            refs[transcript] = digest
    if not refs:
        raise ValueError("No pinned native transcripts")
    sessions = []
    for name, expected in sorted(refs.items()):
        raw = Path(name).read_bytes()
        if sha(raw) != expected:
            raise ValueError("Native transcript hash mismatch")
        selected, snapshots = {}, 0
        for line in raw.splitlines():
            row = json.loads(line)
            message = row.get("message")
            if row.get("type") != "assistant" or not isinstance(message, dict) or message.get("model") == "<synthetic>":
                continue
            usage = message.get("usage")
            identity = row.get("requestId") or message.get("id")
            if not usage or not identity:
                continue
            if any(type(usage.get(field)) is not int or usage[field] < 0 for field in FIELDS):
                raise ValueError("Missing or invalid native usage")
            snapshots += 1
            selected[identity] = {"usage": {field: usage[field] for field in FIELDS}, "model": message.get("model")}
        if not selected:
            raise ValueError("No real usage-bearing responses")
        totals = Counter()
        for response in selected.values():
            totals.update(response["usage"])
        totals["total_input_including_cache"] = sum(totals[field] for field in FIELDS[:3])
        sessions.append({"session_id": Path(name).stem, "transcript": name, "transcript_sha256": expected,
                         "usage_snapshot_rows": snapshots, "usage_responses": len(selected),
                         "duplicate_snapshot_rows_removed": snapshots - len(selected),
                         "models": sorted({row["model"] for row in selected.values()}), "reported_usage": totals})
    return {"item": "M2-ALT-01", "measurement": "Native reported whole-session usage from retained real Claude acceptance sessions.",
            "sources": sources, "sessions": sessions, "session_count": len(sessions),
            "usage_responses": sum(row["usage_responses"] for row in sessions),
            "limits": ["No new model requests, login changes, credential reads or API counting.",
                       "Last usage per response identity is retained; duplicate streaming snapshots are not summed.",
                       "Cache creation/read categories are included once in total input; cache TTL subdivisions are excluded.",
                       "Sessions differ in tools, instructions, model and cache state; this is not a controlled rule-cost comparison.",
                       "No per-rule attribution, billing verification, retry or separate subagent coverage is claimed."],
            "completed": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--acceptance-report", action="append", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    report = summarize(args.acceptance_report)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ("session_count", "usage_responses", "completed")}))


if __name__ == "__main__":
    main()

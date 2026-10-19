#!/usr/bin/env python3
"""Run M2-B1-B4 against an isolated native Codex and the configured model service.

Raw logs/configuration stay in private scratch. The report contains fixture
hashes, delivery counts, native spans, and STYLE stats, never provider secrets.
"""

import argparse
from collections import Counter
import datetime
import json
from pathlib import Path
import shlex
import subprocess
import sys

from codex_probe_support import NativeProbe, digest
from probe_codex_routes import register_plugin, unregister_plugin


BODIES = {
    "native-start": "M2_NATIVE_START_6C173B. This is a native session hook fixture.",
    "native-shell": "M2_NATIVE_SHELL_29B4C1. This is a native shell hook fixture.",
    "native-unmatched": "M2_NATIVE_UNMATCHED_72C15A. This hook must not match Bash.",
    "style-start": "M2_STYLE_START_84D16A. Use concise answers.",
    "style-shell": "M2_STYLE_SHELL_35A71F. Use precise words.",
    "shared": "M2_SHARED_BODY_91D7E2. Answer in one concise sentence.",
}


def write_private(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
    path.chmod(0o600)


def package(probe, name, rules, native_hooks=()):
    path = probe.root / name
    write_private(path / "STYLE.yaml", json.dumps({"spec": "style/v0.3", "name": name,
                  "version": "0.1.0", "description": "Isolated M2 native acceptance", "rules": rules, "native_hooks": list(native_hooks)}, indent=2) + "\n")
    return path


def native_fixture(probe):
    runner = probe.root / "native-observer.py"
    write_private(runner, '''import json, os, sys
payload = json.load(sys.stdin)
row = {"fixture": sys.argv[2], "event": payload.get("hook_event_name"), "session_id": payload.get("session_id")}
descriptor = os.open(sys.argv[1], os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
try:
 os.write(descriptor, (json.dumps(row) + "\\n").encode())
finally:
 os.close(descriptor)
print(json.dumps({"hookSpecificOutput": {"hookEventName": row["event"], "additionalContext": sys.argv[3]}}))
''')
    native_hooks = []
    for name, event, matcher in (("native-start", "SessionStart", None),
                                 ("native-shell", "PreToolUse", "^Bash$"),
                                 ("native-unmatched", "PreToolUse", "^Read$")):
        hook = {"event": event, "command": shlex.join([sys.executable, "-I", str(runner),
                str(probe.root / "native-observations.jsonl"), name, BODIES[name]]), "timeout": 10}
        if matcher:
            hook["matcher"] = matcher
        native_hooks.append({"id": name, "codex": hook})
    rules = [{"id": "style-start", "target": "event", "event": "session.start", "repeat": "session", "text": BODIES["style-start"]},
              {"id": "style-shell", "target": "tool", "tool": "shell", "at": "before", "repeat": "session", "text": BODIES["style-shell"]}]
    return package(probe, "m2-native", rules, native_hooks)


def delivery(call, body):
    counts = [sum(row["occurrences"] for row in request.get("marker_locations", {}).get(body, [])
                  if row.get("role") == "developer") for request in call["requests"]]
    rollout_count = sum(content.get("text", "").count(body) for row in call["hook_context"]
                        for content in row["payload"].get("content", []))
    return {"body_sha256": "sha256:" + digest(body.encode()), "request_occurrences": counts,
            "rollout_occurrences": rollout_count}


def require_delivery(call, body, count):
    facts = delivery(call, body)
    assert call["exit"] == 0 and facts["request_occurrences"], "model turn did not complete with a real request"
    assert facts["rollout_occurrences"] == count, facts
    assert facts["request_occurrences"][-1] == count, facts
    assert all(value <= count for value in facts["request_occurrences"]), facts
    return facts


def counts(records):
    result = dict.fromkeys(("outputs", "blocks", "dedup", "cap_drops", "estimated_tokens", "unestimated"), 0)
    names = {"output": "outputs", "blocked": "blocks", "dedup": "dedup", "cap-drop": "cap_drops"}
    for row in records:
        if row["result"] in names:
            result[names[row["result"]]] += 1
        if row["result"] in ("output", "blocked"):
            if "est_tokens" in row and row.get("body_sha256"):
                result["estimated_tokens"] += row["est_tokens"]
            else:
                result["unestimated"] += 1
    return result


def verify_stats(report, records, by):
    assert report["format"] == 1 and report["by"] == by
    assert report["totals"] == counts(records), "stats totals differ from retained runtime records"
    expected = {}
    for row in records:
        key = row["entry"].split("@")[0] if by == "rule" else None
        expected.setdefault(key, []).append(row)
    if by == "rule":
        assert {row["rule"] for row in report["rows"]} == set(expected)
    else:
        assert len(report["rows"]) == 1
    for row in report["rows"]:
        key = row["rule"] if by == "rule" else None
        assert {name: row[name] for name in counts([])} == counts(expected[key])
    assert not report["quality"].get("partial", False)


def summary(call):
    return {key: call[key] for key in ("session_id", "exit", "rollout_sha256", "stats", "hook_spans")}


def run(probe, route, report):
    config = probe.home / "config.toml"
    baseline, permissions = config.read_bytes(), config.stat().st_mode & 0o777
    probe.evidence_markers = tuple(BODIES.values())
    checks = report["checks"]

    def install(path):
        return probe.style_command("install", str(path), "--harness", "codex", "--route", "codex=" + route, confirm=True)

    def register():
        if route == "codex-plugin":
            register_plugin(probe)

    def uninstall():
        if route == "codex-plugin":
            unregister_plugin(probe)
        probe.style_command("uninstall", "--all", confirm=True)
        assert config.read_bytes() == baseline, "configuration bytes differ after uninstall"
        assert config.stat().st_mode & 0o777 == permissions
        assert not (probe.home / "hooks.json").exists()

    install(native_fixture(probe))
    register()
    active = probe.invoke("m2-native-active", "Run exactly printf STYLE_M2_TOOL_OK once using the shell tool, then reply DONE. Do not use other tools.")
    facts = {name: require_delivery(active, BODIES[name], 0 if name == "native-unmatched" else 1)
             for name in ("native-start", "native-shell", "native-unmatched", "style-start", "style-shell")}
    observations = [json.loads(line) for line in (probe.root / "native-observations.jsonl").read_text().splitlines()]
    assert Counter(row["fixture"] for row in observations) == {"native-start": 1, "native-shell": 1}
    assert all(row["session_id"] == active["session_id"] for row in observations)
    assert {(row["fixture"], row["event"]) for row in observations} == {("native-start", "SessionStart"), ("native-shell", "PreToolUse")}
    source = "plugin" if route == "codex-plugin" else "user"
    spans = active["hook_spans"]
    assert spans and all(row["hook.source"] == source and row["hook.command_outcome"] == "completed" for row in spans)
    for event in ("SessionStart", "PreToolUse"):
        assert sum(row["hook.event_name"] == event for row in spans) == 2, (event, spans)
    assert Counter(row["entry"].split("@")[0] for row in active["stats"] if row["result"] == "output") == {"m2-native/style-start": 1, "m2-native/style-shell": 1}
    checks["M2-B2"] = {"passed": True, "call": summary(active), "delivery": facts,
                       "native_observations": observations, "same_event_handlers_each_once": True}

    reports = {by: probe.style_command("stats", "--by", by) for by in ("rule", "session")}
    for by, result in reports.items():
        verify_stats(result, active["stats"], by)
    assert reports["session"]["rows"][0]["session"] == active["session_id"]
    assert reports["rule"]["totals"]["outputs"] == sum(facts[name]["rollout_occurrences"] for name in ("style-start", "style-shell")) == 2
    checks["M2-B3"] = {"passed": True, "session_id": active["session_id"], "runtime_records": active["stats"],
                       "style_rollout_injections": 2, "native_injections_excluded_from_style_stats": 2, "reports": reports}
    uninstall()
    after = probe.invoke("m2-native-uninstalled", "Reply only OK. Do not use tools.")
    for body in BODIES.values():
        require_delivery(after, body, 0)
    assert not after["hook_spans"] and not after["stats"]
    assert len((probe.root / "native-observations.jsonl").read_text().splitlines()) == 2
    checks["M2-B1"] = {"passed": True, "call": summary(active), "delivery": facts, "native_observations": observations,
                       "uninstalled_call": summary(after), "configuration_byte_and_permission_roundtrip": True,
                       "trust_scope": "native CLI accepts configured user hook hashes or plugin registration; TUI workspace prompt not claimed"}
    print(route + " M2-B1/B2/B3: passed", flush=True)

    rule = {"id": "shared", "target": "event", "event": "session.start", "repeat": "session", "text": BODIES["shared"]}
    install(package(probe, "m2-first", [rule]))
    combined = install(package(probe, "m2-second", [rule]))
    register()
    folded = [rule for harness in combined["reports"] for rule in harness["plan"]["rules"] if rule.get("deduplicated_into")]
    assert len(folded) == 1 and folded[0]["deduplicated_into"] == "m2-first/shared"
    index = json.loads((probe.store / "build/index.json").read_text())
    entries = [entry for values in index["harnesses"]["codex"].values() for entry in values]
    assert len(index["packages"]) == 2 and [entry["rule"] for entry in entries] == ["m2-first/shared"]
    shared = probe.invoke("m2-dedup", "Reply only OK. Do not use tools.")
    shared_facts = require_delivery(shared, BODIES["shared"], 1)
    assert len(shared["stats"]) == 1 and shared["stats"][0]["result"] == "output" and shared["stats"][0]["entry"].startswith("m2-first/shared@")
    checks["M2-B4"] = {"passed": True, "call": summary(shared), "delivery": shared_facts,
                       "compiled_package_count": 2, "compiled_rules": [entry["rule"] for entry in entries], "deduplicated_into": folded[0]["deduplicated_into"]}
    uninstall()
    checks["M2-B4"]["configuration_byte_and_permission_roundtrip"] = True
    print(route + " M2-B4: passed", flush=True)
    report["passed"] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True, type=Path)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--route", choices=("codex-user", "codex-plugin"), required=True)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = NativeProbe(args.scratch, args.style, args.codex)
    report = {"at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "items": ["M2-B1", "M2-B2", "M2-B3", "M2-B4"],
              "scope": "real-native-Codex-with-configured-model-service", "route": args.route,
              "harness_version": probe.native_command("--version"), "style_binary_sha256": digest(args.style.read_bytes()),
              "source_sha256": digest(Path(__file__).read_bytes()), "passed": False, "checks": {}}
    try:
        run(probe, args.route, report)
    except Exception as error:
        report["failure_type"] = type(error).__name__
        report["failure"] = str(error)
        raise
    finally:
        write_private(args.output, json.dumps(probe.sanitize(report), ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

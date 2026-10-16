#!/usr/bin/env python3
"""Separate STYLE install protection from untrimmed native hook overflow."""

import argparse
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys

from codex_probe_support import NativeProbe, digest


def native_context_observation(call, body):
    contexts, files = [], []
    for row in call["hook_context"]:
        for content in row["payload"].get("content", []):
            text = content.get("text", "")
            contexts.append(text)
            match = re.search(r"Full hook output saved to: (\S+)", text)
            if match:
                data = Path(match[1]).read_bytes()
                files.append({"sha256": digest(data), "chars": len(data.decode()), "equals_raw_body": data == body.encode()})
    full = any(body in text for text in contexts)
    return {"outcome": "accepted-full" if full else "file-preview" if files else "not-found-or-other",
            "full_body_in_hook_context": full, "preview_chars": [len(text) for text in contexts], "saved_files": files}


def install_guard(probe, harness):
    package = probe.root / ("guard-" + harness)
    package.mkdir()
    text = "M1_FINAL_GUARD_BEGIN " + "guardword " * 1400 + " M1_FINAL_GUARD_END"
    (package / "STYLE.yaml").write_text(json.dumps({"spec": "style/v0.3", "name": "overflow-guard", "version": "0.1.0",
        "description": "Oversized event for STYLE's own protection", "rules": [
            {"id": "oversized", "target": "event", "event": "session.start", "text": text}]}))
    (probe.root / "claude").mkdir(exist_ok=True)
    config = probe.home / "config.toml" if harness == "codex" else probe.root / "claude/settings.json"
    before = config.read_bytes() if config.exists() else None
    result = subprocess.run([probe.style, "install", str(package), "--harness", harness, "--yes", "--json"],
                            cwd=probe.work, env=probe.env, capture_output=True, text=True, timeout=60)
    assert result.returncode == 4 and "T003" in result.stderr
    assert (config.read_bytes() if config.exists() else None) == before
    assert not probe.store.exists()
    return {"harness": harness, "passed": True, "exit": result.returncode, "diagnostic": result.stderr,
            "body_chars": len(text), "body_sha256": digest(text.encode()), "config_unchanged": True, "style_home_absent": True}


def run(probe, report):
    report["style_guards"] = [install_guard(probe, harness) for harness in ("codex", "claude-code")]
    print("STYLE T003: both harnesses reject before writes", flush=True)
    runner = probe.root / "raw_hook.py"
    runner.write_text('''import json, pathlib, sys
payload=json.load(sys.stdin)
body=pathlib.Path(sys.argv[1]).read_text()
with open(sys.argv[2],"a") as log:
 log.write(json.dumps({"hook_event_name":payload["hook_event_name"],"body_chars":len(body)})+"\\n")
print(json.dumps({"hookSpecificOutput":{"hookEventName":payload["hook_event_name"],"additionalContext":body}}))
''')
    config = probe.home / "config.toml"
    baseline = config.read_bytes()
    checks = []
    for name, event, length in (("session-12000", "SessionStart", 12000), ("pre-12000", "PreToolUse", 12000),
                                ("session-50000", "SessionStart", 50000)):
        begin, end = "M1_FINAL_RAW_" + name + "_BEGIN", "M1_FINAL_RAW_" + name + "_END"
        text = begin + "\n" + ("alpha beta gamma delta epsilon zeta eta theta " * 1200)[:length] + "\n" + end
        body, log = probe.root / (name + ".txt"), probe.root / (name + "-hook.jsonl")
        body.write_text(text)
        command = shlex.join([sys.executable, "-I", str(runner), str(body), str(log)])
        matcher = "Bash" if event == "PreToolUse" else ""
        label = "pre_tool_use" if event == "PreToolUse" else "session_start"
        identity = {"event_name": label, "hooks": [{"async": False, "command": command, "timeout": 10, "type": "command"}]}
        group = {"hooks": [{"type": "command", "command": command, "timeout": 10}]}
        if matcher:
            identity["matcher"] = group["matcher"] = matcher
        definition = "sha256:" + digest(json.dumps(identity, separators=(",", ":"), sort_keys=True).encode())
        (probe.home / "hooks.json").write_text(json.dumps({"hooks": {event: [group]}}))
        key = str(probe.home / "hooks.json") + ":" + label + ":0:0"
        config.write_bytes(baseline + ("\n[hooks.state." + json.dumps(key) + "]\ntrusted_hash=" + json.dumps(definition) + "\n").encode())
        probe.evidence_markers = (begin, end)
        prompt = "Reply only OVERFLOW_OK. Do not use tools." if event == "SessionStart" else "Run `printf RAW_OVERFLOW_TOOL_OK` once using the shell tool, then reply DONE."
        call = probe.invoke(name, prompt)
        assert call["exit"] == 0 and log.exists() and call["requests"]
        facts = [json.loads(line) for line in log.read_text().splitlines()]
        assert len(facts) == 1 and facts[0]["body_chars"] == len(text)
        # Native overflow is an observation, not a prediction: retain what
        # reached the real request, including any file-preview annotation.
        checks.append({"name": name, "native_event": event, "raw_body_chars": len(text), "body_sha256": digest(text.encode()),
                       "body_sent_without_STYLE": True, "hook_runs": facts, "model_turn_completed": True,
                       "begin_in_requests": any(request["marker_locations"][begin] for request in call["requests"]),
                       "end_in_requests": any(request["marker_locations"][end] for request in call["requests"]),
                       **native_context_observation(call, text),
                       "native_context": call["hook_context"]})
        print(name + ": native outcome captured", flush=True)
    report["native_checks"], report["passed"] = checks, True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--style", required=True)
    parser.add_argument("--scratch", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    probe = NativeProbe(args.scratch, args.style)
    report = {"date": "2026-10-16", "item": "M1-FINAL-08-CODEX", "passed": False,
              "harness_version": probe.native_command("--version"), "style_binary_sha256": digest(Path(args.style).read_bytes()),
              "limits": "Raw sizes are Unicode characters. No official tokenizer count is claimed."}
    try:
        run(probe, report)
    finally:
        report["calls"] = probe.calls
        args.output.write_text(json.dumps(probe.sanitize(report), ensure_ascii=False, indent=2) + "\n")
        probe.close()


if __name__ == "__main__":
    main()

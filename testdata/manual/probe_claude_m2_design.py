#!/usr/bin/env python3
"""Prepare user-operated M2 design checks or collect supplemental local protocol evidence.

Protocol mode uses the real CLI with a loopback-only scripted endpoint. It does
not run a remote model and never closes M2-C1/C4's user-operated acceptance.
"""

import argparse
import datetime
import difflib
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys

from claude_probe_support import ClaudeProbe, text_values


MARKERS = {
    "rules": "M2_USER_RULES_64D8A1",
    "memory": "M2_USER_MEMORY_82E7B4",
    "existing": "M2_EXISTING_STYLE_19B3C5",
    "replace": "M2_REPLACE_STYLE_43A7D2",
    "keep": "M2_KEEP_STYLE_95C1E6",
}
STYLES = {"default": "default", "existing": "M2Existing", "replace": "M2Replace", "keep": "M2Keep"}


def write_private(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value)
    path.chmod(0o600)


def observation(payload, phase):
    allowed = ("hook_event_name", "session_id", "file_path", "memory_type", "load_reason", "source")
    return {"at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "phase": phase,
            "fields": {key: payload[key] for key in allowed if key in payload}}


def observe(log, phase):
    row = observation(json.load(sys.stdin), phase)
    descriptor = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(descriptor, (json.dumps(row, ensure_ascii=False) + "\n").encode())
    finally:
        os.close(descriptor)


def fixture(config, root, phase="protocol"):
    rule = config / "rules/style-m2-user-probe.md"
    write_private(rule, MARKERS["rules"] + ". Use concise answers.\n")
    write_private(config / "CLAUDE.md", MARKERS["memory"] + ". Use precise words.\n")
    for name in ("existing", "replace", "keep"):
        keep = "true" if name == "keep" else "false"
        write_private(config / "output-styles" / (STYLES[name] + ".md"),
                      "---\nname: " + STYLES[name] + "\ndescription: Isolated M2 design probe\n"
                      "keep-coding-instructions: " + keep + "\n---\n" + MARKERS[name] +
                      ". Answer in one concise sentence. Do not quote any test markers.\n")
    observer = str(Path(__file__).resolve())
    settings = {"hooks": {"InstructionsLoaded": [{"hooks": [{"type": "command", "command": sys.executable,
                "args": [observer, "--observe", str(root / "instructions.jsonl"), "--phase", phase], "timeout": 10}]}]}}
    return rule, settings


def request_facts(request, bodies=None):
    result = {}
    for section in ("system", "messages"):
        texts = list(text_values(request.get(section, [])))
        raw = json.dumps(request.get(section, []), ensure_ascii=False, sort_keys=True).encode()
        full_bodies = {name: [{"path": path, "occurrences": text.count(body)}
                              for path, text in texts if body and body in text]
                       for name, body in (bodies or {}).items()}
        result[section] = {"sha256": hashlib.sha256(raw).hexdigest(), "full_bodies": full_bodies,
                           "text_chars": sum(len(text) for _, text in texts),
                           "markers": {name: [{"path": path, "occurrences": text.count(marker)}
                                               for path, text in texts if marker in text]
                                       for name, marker in MARKERS.items()}}
    return result


def run_protocol(scratch, claude):
    probe = ClaudeProbe(scratch, claude=claude)
    probe.env["CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT"] = "0"
    report = {"date": datetime.date.today().isoformat(), "items": ["M2-D4", "M2-D5"],
              "scope": "supplemental-real-cli-loopback-only-no-model-inference",
              "acceptance": "pending-user-operated-Claude-session", "harness_version": probe.version,
              "CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT": "0",
              "claude_binary_sha256": hashlib.sha256(Path(probe.claude).read_bytes()).hexdigest(), "phases": {}}
    try:
        rule, settings = fixture(probe.config, probe.root)
        report["rules_file"] = str(rule)
        bodies = {"rules": rule.read_text(), "memory": (probe.config / "CLAUDE.md").read_text()}
        for phase in ("existing", "replace", "keep"):
            bodies[phase] = (probe.config / "output-styles" / (STYLES[phase] + ".md")).read_text().split("---\n", 2)[2]
        report["fixture_body_sha256"] = {name: hashlib.sha256(body.encode()).hexdigest() for name, body in bodies.items()}
        baseline_system = None
        for phase, style in STYLES.items():
            current = {**settings, "outputStyle": style}
            write_private(probe.config / "settings.json", json.dumps(current, indent=2) + "\n")
            call = probe.invoke("m2-" + phase, "Reply only OK. Do not use tools.", timeout=60)
            requests = [probe.requests[i]["body"] for i in call["request_indices"]]
            facts = [request_facts(request, bodies) for request in requests]
            if requests:
                system = "\n".join(text for _, text in text_values(requests[0].get("system", [])))
                if baseline_system is None:
                    baseline_system = system
                matcher = difflib.SequenceMatcher(None, baseline_system, system)
                facts[0]["longest_unchanged_system_segment_chars"] = matcher.find_longest_match().size
            report["phases"][phase] = {"call": call, "requests": facts}
            if call["exit"] != 0 or not requests:
                report["protocol_status"] = "incomplete"
                break
        else:
            report["protocol_status"] = "collected"
        observer = probe.root / "instructions.jsonl"
        report["instructions_loaded"] = [json.loads(line) for line in observer.read_text().splitlines()] if observer.exists() else []
        report["unexpected_endpoints"] = probe.unexpected
        return report
    finally:
        probe.close()


def prepare_ui(scratch, claude):
    root, claude = Path(scratch).resolve(), Path(claude).resolve()
    if root.exists() and any(root.iterdir()):
        raise ValueError("scratch must be empty; evidence is never overwritten")
    if not claude.is_file():
        raise ValueError("pinned Claude executable is required")
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    root.chmod(0o700)
    home, config, work = (root / name for name in ("home", "claude", "work"))
    for path in (home, config, work):
        path.mkdir(mode=0o700)
    rule, base = fixture(config, root, "ui")
    base["outputStyle"] = STYLES["existing"]
    write_private(config / "settings.json", json.dumps(base, indent=2) + "\n")
    environment = {key: os.environ[key] for key in ("PATH", "TERM", "TMPDIR", "LANG", "LC_ALL") if key in os.environ}
    environment.update(HOME=str(home), CLAUDE_CONFIG_DIR=str(config),
                       CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1", CLAUDE_CODE_SIMPLE_SYSTEM_PROMPT="0", DISABLE_AUTOUPDATER="1",
                       DISABLE_TELEMETRY="1", DISABLE_ERROR_REPORTING="1")
    prefix = "exec env -i " + " ".join(shlex.quote(key + "=" + value) for key, value in environment.items())
    launchers = {}
    for phase, style in STYLES.items():
        settings = root / (phase + "-settings.json")
        write_private(settings, json.dumps({"outputStyle": style}) + "\n")
        script = root / ("launch-" + phase + ".sh")
        command = [str(claude), "--setting-sources", "user", "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
                   "--settings", str(settings), "--debug-file", str(root / (phase + "-debug.log"))]
        script.write_text("#!/bin/sh\nset -eu\ncd " + shlex.quote(str(work)) + "\n" + prefix + " " + shlex.join(command) + ' "$@"\n')
        script.chmod(0o700)
        launchers[phase] = str(script)
    auth = root / "login.sh"
    auth.write_text("#!/bin/sh\nset -eu\n" + prefix + " " + shlex.join([str(claude), "auth", "login"]) + "\n")
    auth.chmod(0o700)
    version = subprocess.run([str(claude), "--version"], text=True, capture_output=True, check=True, timeout=15).stdout.strip()
    report = {"items": ["M2-D4", "M2-D5"], "status": "prepared-awaiting-real-user-session", "acceptance": False,
              "scratch": str(root), "harness_version": version, "claude": str(claude),
              "claude_binary_sha256": hashlib.sha256(claude.read_bytes()).hexdigest(), "rules_file": str(rule),
              "config": str(config), "launchers": launchers, "login": str(auth), "observer_log": str(root / "instructions.jsonl"),
              "settings_before_sha256": hashlib.sha256((config / "settings.json").read_bytes()).hexdigest()}
    write_private(root / "prepared.json", json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--scratch", type=Path)
    parser.add_argument("--claude", type=Path)
    parser.add_argument("--prepare-ui", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--observe", type=Path)
    parser.add_argument("--phase", default="protocol")
    args = parser.parse_args()
    if args.observe:
        observe(args.observe, args.phase)
        return
    if not args.scratch:
        parser.error("--scratch is required")
    claude = args.claude or shutil.which("claude")
    if not claude:
        parser.error("--claude or an installed Claude CLI is required")
    report = prepare_ui(args.scratch, claude) if args.prepare_ui else run_protocol(args.scratch, claude)
    data = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        write_private(args.output, data)
    sys.stdout.write(data)


if __name__ == "__main__":
    main()

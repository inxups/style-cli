#!/usr/bin/env python3
"""Measure real Codex config loading against a local request sink; no model call.

The sink deliberately returns HTTP 400 after recording reasoning.effort. These
expected failed turns prove config loading, never model inference or delivery.
Only synthetic config and safe request fields are retained in the report.
"""

import argparse
import json
import os
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def measure(binary):
    requests = []

    class Sink(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            self.send_response(404)
            self.end_headers()

        def do_POST(self):
            data = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
            requests.append({"model": data.get("model"), "reasoning": data.get("reasoning")})
            body = b'{"error":{"message":"STYLE config probe complete","type":"invalid_request_error"}}'
            self.send_response(400)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    cases = [
        ("user", 'model_reasoning_effort="low"', None, [], None, "low"),
        ("file", 'model_reasoning_effort="low"', "high", ["-p", "probe"], None, "high"),
        ("project", 'model_reasoning_effort="low"', "medium", ["-p", "probe"], "trusted", "xhigh"),
        ("untrusted", 'model_reasoning_effort="low"', "medium", ["-p", "probe"], "untrusted", "medium"),
        ("override", 'model_reasoning_effort="low"', "high", ["-p", "probe", "-c", 'model_reasoning_effort="medium"'], "trusted", "medium"),
        ("unknown-raw", 'model_reasoning_effort="future-tier"', None, [], None, "future-tier"),
        ("unset", "", None, [], None, "default"),
        ("legacy-table", '[profiles.probe]\nmodel_reasoning_effort="high"', None, ["-p", "probe"], None, "reject"),
        ("legacy-default", 'profile="probe"', "high", [], None, "reject"),
        ("legacy-conflict", '[profiles.probe]\nmodel_reasoning_effort="medium"', "high", ["-p", "probe"], None, "reject"),
        ("missing-profile", 'model_reasoning_effort="low"', None, ["-p", "probe"], None, "low"),
    ]
    server = ThreadingHTTPServer(("127.0.0.1", 0), Sink)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    results = []
    try:
        with tempfile.TemporaryDirectory(prefix="style-native-config-") as temporary:
            for name, extra, profile, flags, trust, expected in cases:
                root = Path(temporary) / name
                home, work = root / "home", root / "work"
                home.mkdir(parents=True)
                work.mkdir()
                subprocess.run(["git", "init", "-q", str(work)], check=True)
                config = 'model="gpt-6.1-sol"\nmodel_provider="fixture"\n' + extra + "\n"
                if trust:
                    config += "[projects." + json.dumps(str(work.resolve())) + "]\ntrust_level=" + json.dumps(trust) + "\n"
                    (work / ".codex").mkdir()
                    (work / ".codex/config.toml").write_text('model_reasoning_effort="xhigh"\n')
                config += ('[model_providers.fixture]\nname="Local config fixture"\nwire_api="responses"\n'
                           'requires_openai_auth=false\nbase_url="http://127.0.0.1:%d/v1"\n' % server.server_port)
                (home / "config.toml").write_text(config)
                if profile is not None:
                    (home / "probe.config.toml").write_text("model_reasoning_effort=" + json.dumps(profile) + "\n")
                env = {key: value for key, value in os.environ.items()
                       if key not in ("CODEX_HOME", "CODEX_SANDBOX", "OPENAI_API_KEY", "OPENAI_BASE_URL")}
                env["CODEX_HOME"] = str(home)
                offset = len(requests)
                completed = subprocess.run([binary, "exec", *flags, "--skip-git-repo-check", "--json", "-s", "read-only", "STYLE configuration probe."],
                                           input="", cwd=work, env=env, text=True, capture_output=True, timeout=30)
                captured = requests[offset:]
                observed = captured[0].get("reasoning", {}).get("effort") if captured else None
                if expected == "reject":
                    passed = not captured and completed.returncode != 0 and "profile" in completed.stderr.lower()
                else:
                    passed = (len(captured) == 1 and completed.returncode != 0
                              and "STYLE config probe complete" in completed.stdout
                              and (expected == "default" or observed == expected))
                results.append({"case": name, "expected": expected, "observed_request_effort": observed,
                                "request_count": len(captured), "exit": completed.returncode, "passed": passed,
                                "note": "native rejects legacy or absent profile" if expected == "reject" else "local sink deliberately rejects the model request"})
    finally:
        server.shutdown()
        server.server_close()
    return {"harness_version": subprocess.check_output([binary, "--version"], text=True).strip(),
            "measurement": "real CLI configuration loading; local HTTP sink; no inference or STYLE hook acceptance claimed",
            "cases": results, "passed": all(item["passed"] for item in results)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    report = measure(args.codex)
    text = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text)
    print(text, end="")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())

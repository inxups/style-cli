"""Isolated native Codex acceptance support. Never serialize provider secrets.

The local proxy forwards to the user's configured provider and records only
safe request facts and STYLE context. Raw subprocess logs stay in the private
scratch directory, outside the repository. Calls use the real model service.
"""

import hashlib
import json
import os
from pathlib import Path
import queue
import re
import shutil
import ssl
import subprocess
import sys
import threading
import tomllib
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


MARKERS = (
    "Current session:", "M1B_SESSION", "Use a concise review format",
    "When using the code-review skill", "Before creating a pull request",
    "Do not force push", "TODO", "M1_FINAL_", "M1B_WARN_MODEL",
)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def marker_locations(body, markers):
    locations = {marker: [] for marker in markers}
    for index, item in enumerate(body.get("input", [])):
        text = json.dumps(item, ensure_ascii=False)
        for marker in markers:
            if marker in text:
                locations[marker].append({"input_index": index, "role": item.get("role"), "type": item.get("type"),
                                          "occurrences": text.count(marker), "item_sha256": digest(text.encode())})
    return locations


class NativeProbe:
    def __init__(self, root, style, codex="codex"):
        self.root, self.style, self.codex = Path(root).resolve(), str(Path(style).resolve()), codex
        if self.root.exists() and any(self.root.iterdir()):
            raise ValueError("acceptance scratch directory must be empty")
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.home, self.store, self.work = (self.root / name for name in ("codex", "style", "work"))
        self.home.mkdir(exist_ok=True)
        self.work.mkdir(exist_ok=True)
        self.requests, self.calls, self.hook_spans = [], [], []
        self.evidence_markers = ()
        original = Path(os.environ.get("CODEX_HOME", Path.home() / ".codex"))
        config = tomllib.loads((original / "config.toml").read_text())
        provider = config.get("model_provider", "openai")
        definition = config.get("model_providers", {}).get(provider)
        if not definition or not definition.get("base_url"):
            raise ValueError("probe requires an explicitly configured Responses provider; no credentials guessed")
        upstream = definition["base_url"].rstrip("/")
        self.redactions = [value for value in (upstream, definition.get("experimental_bearer_token"),
                                               os.environ.get("OPENAI_API_KEY")) if value]
        tls = ssl.create_default_context()
        # Python's bundled CA set can omit macOS-managed roots used by Codex.
        # Keep certificate verification enabled and include the system roots.
        if sys.platform == "darwin":
            for keychain in ("/System/Library/Keychains/SystemRootCertificates.keychain",
                             "/Library/Keychains/System.keychain"):
                certificates = subprocess.run(["/usr/bin/security", "find-certificate", "-a", "-p", keychain],
                                              capture_output=True, text=True, check=False)
                if certificates.returncode == 0 and certificates.stdout:
                    tls.load_verify_locations(cadata=certificates.stdout)
        owner = self

        class Forwarder(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                self.send_response(404)
                self.end_headers()

            def do_POST(self):
                raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
                body = json.loads(raw)
                if self.path == "/v1/traces":
                    for resource in body.get("resourceSpans", []):
                        for scope in resource.get("scopeSpans", []):
                            for span in scope.get("spans", []):
                                if span.get("name") == "codex.hooks.command":
                                    allowed = ("hook.event_name", "hook.source", "hook.handler_type", "hook.command_outcome")
                                    attributes = {item["key"]: next(iter(item["value"].values()), None)
                                                  for item in span.get("attributes", []) if item["key"] in allowed}
                                    owner.hook_spans.append({"start_ns": span.get("startTimeUnixNano"),
                                                             "end_ns": span.get("endTimeUnixNano"), **attributes})
                    self.send_response(200)
                    self.end_headers()
                    self.wfile.write(b"{}")
                    return
                contexts = []
                for item in body.get("input", []):
                    if isinstance(item, dict) and item.get("role") == "developer":
                        text = json.dumps(item, ensure_ascii=False)
                        if any(marker in text for marker in MARKERS):
                            contexts.append(item)
                owner.requests.append({
                    "model": body.get("model"), "reasoning": body.get("reasoning"),
                    "body_sha256": digest(raw), "style_context": contexts,
                    "static_rule_present": "Review code concisely in zh-CN." in json.dumps(body),
                    **({"marker_locations": marker_locations(body, owner.evidence_markers)} if owner.evidence_markers else {}),
                })
                headers = {key: value for key, value in self.headers.items()
                           if key.lower() not in ("host", "content-length", "connection", "accept-encoding")}
                request = urllib.request.Request(upstream + self.path.removeprefix("/v1"), raw, headers)
                try:
                    response = urllib.request.urlopen(request, timeout=180, context=tls)
                except urllib.error.HTTPError as error:
                    response = error
                except (OSError, urllib.error.URLError):
                    self.send_error(502, "Configured provider unavailable")
                    return
                with response:
                    self.send_response(response.status)
                    for key, value in response.headers.items():
                        if key.lower() not in ("transfer-encoding", "connection", "content-length"):
                            self.send_header(key, value)
                    self.send_header("Connection", "close")
                    self.end_headers()
                    try:
                        while chunk := response.read1(65536):
                            self.wfile.write(chunk)
                            self.wfile.flush()
                    except (BrokenPipeError, ConnectionResetError):
                        pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Forwarder)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.model = config.get("model", "gpt-6.1-sol")
        self.base = f'model={json.dumps(self.model)}\nmodel_provider={json.dumps(provider)}\n'
        self.provider = dict(definition, base_url=f"http://127.0.0.1:{self.server.server_port}/v1",
                             request_max_retries=0, stream_max_retries=0)
        self.provider_name = provider
        self.env = {key: value for key, value in os.environ.items() if key not in (
            "CODEX_HOME", "CODEX_SANDBOX", "STYLE_HOME", "CLAUDE_CONFIG_DIR", "OPENAI_BASE_URL")}
        self.env.update(CODEX_HOME=str(self.home), STYLE_HOME=str(self.store),
                        CLAUDE_CONFIG_DIR=str(self.root / "claude"), GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_CONFIG_NOSYSTEM="1")
        if (original / "auth.json").exists():
            shutil.copyfile(original / "auth.json", self.home / "auth.json")
            (self.home / "auth.json").chmod(0o600)
        self.write_config("low")
        self.baseline_config_sha256 = digest((self.home / "config.toml").read_bytes())

    def close(self):
        self.server.shutdown()
        self.server.server_close()

    def write_config(self, effort, extra=""):
        text = self.base
        if effort is not None:
            text += f'model_reasoning_effort={json.dumps(effort)}\n'
        text += extra + '\n[features]\nhooks=true\n'
        text += '[otel]\ntrace_exporter={otlp-http={endpoint="http://127.0.0.1:%d/v1/traces",protocol="json"}}\n' % self.server.server_port
        text += f'[model_providers.{self.provider_name}]\n'
        for key, value in self.provider.items():
            if isinstance(value, (str, bool, int, float)):
                text += f'{key}={json.dumps(value)}\n'
        path = self.home / "config.toml"
        path.write_text(text)
        path.chmod(0o600)

    def style_command(self, *args, expected=0):
        done = subprocess.run([self.style, *args, "--json"], env=self.env, cwd=self.work,
                              input="", text=True, capture_output=True, timeout=60)
        if done.returncode != expected:
            raise AssertionError(f"style {args[0]} exit {done.returncode}, expected {expected}")
        return json.loads(done.stdout)

    def set_effort(self, effort):
        path = self.home / "config.toml"
        text = path.read_text()
        replacement = f'model_reasoning_effort={json.dumps(effort)}\n' if effort is not None else ''
        if re.search(r'^model_reasoning_effort\s*=', text, flags=re.MULTILINE):
            text = re.sub(r'^model_reasoning_effort\s*=.*\n', replacement, text, flags=re.MULTILINE)
        elif replacement:
            text = re.sub(r'^(model_provider\s*=.*\n)', lambda match: match[0] + replacement, text, count=1, flags=re.MULTILINE)
        path.write_text(text)

    def native_command(self, *args):
        done = subprocess.run([self.codex, *args], env=self.env, cwd=self.work,
                              input="", text=True, capture_output=True, timeout=60)
        if done.returncode:
            raise AssertionError(f"Codex {args[:2]} exit {done.returncode}")
        return json.loads(done.stdout) if done.stdout.strip().startswith(("{", "[")) else done.stdout

    def invoke(self, name, prompt, flags=(), resume=None, timeout=240):
        start = len(self.requests)
        span_start = len(self.hook_spans)
        argv = [self.codex, "exec"]
        if resume:
            argv += ["resume", resume]
        argv += [*flags, "--skip-git-repo-check", "--json"]
        if not resume:
            argv += ["-s", "danger-full-access"]
        done = subprocess.run([*argv, prompt], env=self.env, cwd=self.work,
                              input="", text=True, capture_output=True, timeout=timeout)
        for suffix, data in (("jsonl", done.stdout), ("stderr", done.stderr)):
            path = self.root / f"{name}.{suffix}"
            path.write_text(data)
            path.chmod(0o600)
        events = [json.loads(line) for line in done.stdout.splitlines() if line.startswith("{")]
        thread = next((event["thread_id"] for event in events if event.get("type") == "thread.started"), resume)
        call = self.capture(name, thread, events, start, done.returncode)
        call["hook_spans"] = self.hook_spans[span_start:]
        return call

    def capture(self, name, thread, events, start, exit_code=0):
        rollout = []
        if thread:
            for path in (self.home / "sessions").rglob(f"*{thread}*.jsonl"):
                rollout = [json.loads(line) for line in path.read_text().splitlines()]
        session_path = self.store / "state" / str(thread) / "session.json"
        session = json.loads(session_path.read_text()) if session_path.exists() else None
        stats_path = self.store / "state" / str(thread) / "main" / "stats.jsonl"
        stats = [json.loads(line) for line in stats_path.read_text().splitlines()] if stats_path.exists() else []
        hook_context = [row for row in rollout if row.get("type") == "response_item"
                        and row.get("payload", {}).get("role") == "developer"
                        and "hooks.additional_context" in json.dumps(row)]
        call = {"name": name, "exit": exit_code, "session_id": thread, "state": session,
                "stats": stats, "requests": self.requests[start:], "hook_context": hook_context,
                "events": events, "rollout_sha256": digest(json.dumps(rollout).encode())}
        self.calls.append(call)
        return call

    def sanitize(self, report):
        text = json.dumps(report, ensure_ascii=False).replace(str(self.root), "<scratch>")
        for secret in self.redactions:
            text = text.replace(secret, "<configured-provider-value>")
        return json.loads(text)


class AppServer:
    """Drive published local JSON-RPC methods; retain native notifications."""

    def __init__(self, probe):
        self.probe, self.sequence, self.events = probe, 0, []
        self.messages = queue.Queue()
        self.stderr = (probe.root / "app-server.stderr").open("w")
        self.process = subprocess.Popen([probe.codex, "app-server", "--stdio"], env=probe.env,
                                        cwd=probe.work, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=self.stderr, text=True, bufsize=1)

        def read():
            for line in self.process.stdout:
                self.messages.put(json.loads(line))
            self.messages.put(None)

        threading.Thread(target=read, daemon=True).start()
        self.rpc("initialize", {"clientInfo": {"name": "style-m1-acceptance", "version": "0.1"},
                                "capabilities": {"experimentalApi": True}})

    def next(self):
        message = self.messages.get(timeout=240)
        if message is None:
            raise RuntimeError("app-server exited")
        if "method" in message and "id" not in message:
            self.events.append(message)
        return message

    def rpc(self, method, params):
        self.sequence += 1
        message_id = self.sequence
        self.process.stdin.write(json.dumps({"id": message_id, "method": method, "params": params}) + "\n")
        self.process.stdin.flush()
        while True:
            message = self.next()
            if message.get("id") == message_id:
                if "error" in message:
                    raise RuntimeError(f"{method}: {message['error']}")
                return message.get("result")

    def turn(self, thread, text):
        self.rpc("turn/start", {"threadId": thread, "input": [{"type": "text", "text": text}]})
        while True:
            message = self.next()
            if message.get("method") == "turn/completed" and message.get("params", {}).get("threadId") == thread:
                return message

    def close(self):
        self.process.terminate()
        self.process.wait(timeout=15)
        self.stderr.close()

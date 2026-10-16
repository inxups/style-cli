"""Real Claude CLI driven by a loopback-only, scripted Messages endpoint.

The endpoint never calls a model. Its usage values are synthetic protocol
fixtures, not token measurements. Full requests stay in the private scratch.
"""

import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import threading
import time
import uuid


def digest(data):
    return hashlib.sha256(data).hexdigest()


def text_values(value, path=""):
    if isinstance(value, str):
        yield path, value
    elif isinstance(value, list):
        for index, item in enumerate(value):
            yield from text_values(item, path + "/" + str(index))
    elif isinstance(value, dict):
        for key, item in value.items():
            yield from text_values(item, path + "/" + key)


def body_observation(request, body, artifact_root=None):
    """Only an exact contiguous match proves the raw body reached the API."""
    matches = [{"path": path, "text_chars": len(text), "body_occurrences": text.count(body)}
               for path, text in text_values({key: request[key] for key in ("system", "messages") if key in request})
               if body in text]
    saved, previews = [], []
    for path, text in text_values({key: request[key] for key in ("system", "messages") if key in request}):
        if body[:40] not in text or "<persisted-output>" not in text:
            continue
        previews.append({"path": path, "text_chars": len(text)})
        for name in re.findall(r"Full output saved to: ([^\n]+)", text):
            candidate = Path(name.strip()).resolve()
            if artifact_root is None or not candidate.is_relative_to(Path(artifact_root).resolve()):
                raise ValueError("native artifact outside the private scratch")
            data = candidate.read_bytes()
            saved.append({"path": str(candidate), "sha256": digest(data), "chars": len(data.decode()),
                          "equals_raw_body": data == body.encode()})
    outcome = ("accepted-full" if matches else "file-preview" if saved and all(row["equals_raw_body"] for row in saved)
               else "not-found-or-invalid-artifact")
    return {"outcome": outcome, "full_body_in_request": bool(matches), "matches": matches,
            "previews": previews, "saved_files": saved}


class ClaudeProbe:
    def __init__(self, scratch, style=None, claude=None):
        self.root = Path(scratch).resolve()
        if self.root.exists() and any(self.root.iterdir()):
            raise ValueError("scratch must be empty; historical evidence is never overwritten")
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.root.chmod(0o700)
        self.home, self.config, self.work, self.store = (self.root / name for name in ("home", "claude", "work", "style"))
        for path in (self.home, self.config, self.work):
            path.mkdir()
        self.claude = str(Path(claude).resolve()) if claude else shutil.which("claude")
        if not self.claude:
            raise ValueError("Claude CLI is required")
        native_bin = self.root / "native-bin"
        native_bin.mkdir()
        (native_bin / "claude").symlink_to(self.claude)
        self.style = None
        if style:
            self.style = self.root / "bin/style"
            self.style.parent.mkdir()
            shutil.copyfile(style, self.style)
            self.style.chmod(0o700)
        self.calls, self.requests, self.unexpected = [], [], []
        self.lock = threading.Lock()
        self.responder = lambda request: [{"type": "text", "text": "STYLE_LOCAL_STUB_DONE"}]
        probe = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_POST(self):
                data = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
                if self.path.split("?")[0] == "/v1/messages/count_tokens":
                    # Some native paths require a protocol count. It cannot be
                    # used as actual usage or official U-T3 calibration.
                    self.send_json({"input_tokens": 100})
                    return
                if self.path.split("?")[0] != "/v1/messages":
                    with probe.lock:
                        probe.unexpected.append({"method": "POST", "path": self.path})
                    self.send_json({}, status=404)
                    return
                with probe.lock:
                    number = len(probe.requests) + 1
                    path = probe.root / ("request-%04d.json" % number)
                    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
                    path.chmod(0o600)
                    probe.requests.append({"path": str(path), "sha256": digest(path.read_bytes()), "body": data})
                content = probe.responder(data)
                response = {"id": "msg_style_fixture_" + str(number), "type": "message", "role": "assistant",
                            "model": data["model"], "content": content,
                            "stop_reason": "tool_use" if any(b["type"] == "tool_use" for b in content) else "end_turn",
                            "stop_sequence": None, "usage": {"input_tokens": 10, "output_tokens": 10}}
                if not data.get("stream"):
                    self.send_json(response)
                    return
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Connection", "close")
                self.end_headers()
                start = {**response, "content": [], "stop_reason": None}
                self.event("message_start", {"message": start})
                for index, block in enumerate(content):
                    empty = {**block, "text": ""} if block["type"] == "text" else {**block, "input": {}}
                    self.event("content_block_start", {"index": index, "content_block": empty})
                    delta = ({"type": "text_delta", "text": block["text"]} if block["type"] == "text" else
                             {"type": "input_json_delta", "partial_json": json.dumps(block["input"])})
                    self.event("content_block_delta", {"index": index, "delta": delta})
                    self.event("content_block_stop", {"index": index})
                self.event("message_delta", {"delta": {"stop_reason": response["stop_reason"], "stop_sequence": None},
                                              "usage": {"output_tokens": 10}})
                self.event("message_stop", {})

            def event(self, kind, value):
                data = {"type": kind, **value}
                self.wfile.write(("event: " + kind + "\ndata: " + json.dumps(data) + "\n\n").encode())
                self.wfile.flush()

            def send_json(self, value, status=200):
                data = json.dumps(value).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        # An allowlist prevents ambient OAuth/provider configuration leaking
        # into this test. No real credential is read or sent to the stub.
        self.env = {key: os.environ[key] for key in ("PATH", "TMPDIR", "LANG", "LC_ALL", "TERM") if key in os.environ}
        self.env["PATH"] = str(native_bin) + os.pathsep + self.env.get("PATH", "")
        self.env.update(HOME=str(self.home), CLAUDE_CONFIG_DIR=str(self.config), STYLE_HOME=str(self.store),
                        ANTHROPIC_API_KEY="style-local-fixture-key-not-secret",
                        ANTHROPIC_BASE_URL="http://127.0.0.1:" + str(self.server.server_port),
                        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1", DISABLE_AUTOUPDATER="1",
                        DISABLE_TELEMETRY="1", DISABLE_ERROR_REPORTING="1")
        self.version = subprocess.run([self.claude, "--version"], env=self.env, text=True, capture_output=True, check=True).stdout.strip()

    def invoke(self, name, prompt, extra=(), tools="", timeout=90, setting_sources="user", settings=None, model="claude-sonnet-5-5"):
        start = len(self.requests)
        command = [self.claude, "-p", "--verbose", "--output-format", "stream-json", "--include-hook-events",
                   "--setting-sources", setting_sources, "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
                   "--model", model, "--effort", "high", "--tools", tools,
                   "--debug-file", str(self.root / (name + "-debug.log")), *extra, prompt]
        if settings:
            command[-1:-1] = ["--settings", str(settings)]
        at = time.monotonic()
        result = subprocess.run(command, cwd=self.work, env=self.env, capture_output=True, text=True, timeout=timeout)
        stdout, stderr = self.root / (name + "-stdout.jsonl"), self.root / (name + "-stderr.txt")
        stdout.write_text(result.stdout)
        stderr.write_text(result.stderr)
        rows = []
        for line in result.stdout.splitlines():
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        finals = [row for row in rows if row.get("type") == "result"]
        final = finals[-1] if finals else None
        call = {"name": name, "command": command, "exit": result.returncode,
                "elapsed_seconds": round(time.monotonic() - at, 3), "stdout_path": str(stdout), "stderr_path": str(stderr),
                "result": {key: final[key] for key in ("subtype", "is_error", "terminal_reason", "num_turns", "permission_denials")
                           if key in final} if final else None,
                "request_indices": list(range(start, len(self.requests))),
                "session_id": final.get("session_id") if final else None}
        self.calls.append(call)
        return call

    def install(self, package):
        result = subprocess.run([str(self.style), "install", str(package), "--harness", "claude-code", "--yes", "--json"],
                                cwd=self.work, env=self.env, capture_output=True, text=True, timeout=60, check=True)
        return json.loads(result.stdout)

    def report_requests(self):
        return [{key: value for key, value in row.items() if key != "body"} for row in self.requests]

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


def session_id():
    return str(uuid.uuid4())

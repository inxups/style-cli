"""Authenticated isolated CLI probes and verified native request replay.

Authentication is consumed by the CLI using the user-approved M1 config.
This code does not read credentials or capture HTTP authentication headers.
"""

import copy
import json
import os
from pathlib import Path

from claude_probe_support import ClaudeProbe, digest, text_values


def canonical_hash(value):
    return digest(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode())


def replay_requests(rows, known_rows=()):
    blobs, shapes, previous, requests = {}, {}, [], []
    assistants = {}
    known_blobs = {row["hash"]: row["message"] for row in known_rows if row.get("type") == "api-request-blob"
                   and canonical_hash(row["message"]) == row["hash"]}
    blobs.update(known_blobs)
    for row in [*known_rows, *rows]:
        kind = row.get("type")
        if kind == "api-request-blob":
            if canonical_hash(row["message"]) != row["hash"]:
                if row["message"].get("role") == "system" and row["hash"] in known_blobs:
                    # Native fork rewrites some historical blobs as system
                    # records while preserving the original request hash.
                    # Resolve only through exact verified source content.
                    continue
                raise ValueError("native message content hash mismatch")
            blobs[row["hash"]] = row["message"]
        elif kind == "api-request-shape":
            if canonical_hash(row["shape"]) != row["shapeHash"]:
                raise ValueError("native shape content hash mismatch")
            shapes[row["shapeHash"]] = row["shape"]
        elif kind == "assistant" and row.get("message", {}).get("id"):
            message = row["message"]
            group = assistants.setdefault(message["id"], {})
            start = row.get("apiBlockIndex", len(group))
            for offset, block in enumerate(message.get("content", [])):
                group[start + offset] = block
        elif kind == "user" and isinstance(row.get("message"), dict):
            # Native input may reference the existing timed tool-result row
            # instead of persisting a duplicate api-request-blob.
            blobs[canonical_hash(row["message"])] = row["message"]
            if isinstance(row["message"].get("content"), str):
                message = {"role": "user", "content": [{"type": "text", "text": row["message"]["content"]}]}
                blobs[canonical_hash(message)] = message
    # Native transcripts keep streamed assistant blocks in assistant rows,
    # rather than duplicating them as api-request-blob. Only a candidate whose
    # canonical hash equals the request's reference can be replayed.
    for group in assistants.values():
        content = []
        for _, block in sorted(group.items()):
            content.append(block)
            message = {"role": "assistant", "content": list(content)}
            blobs[canonical_hash(message)] = message
    for row in rows:
        if row.get("type") == "api-request":
            keep = row["keep"]
            if not isinstance(keep, int) or keep < 0 or keep > len(previous):
                raise ValueError("native request prefix cannot be reconstructed")
            hashes = previous[:keep] + row["tail"]
            if row["shapeHash"] not in shapes or any(value not in blobs for value in hashes):
                missing = [value for value in hashes if value not in blobs]
                raise ValueError("native request references missing content: " + row["id"] + " " + ",".join(missing))
            body = {**copy.deepcopy(shapes[row["shapeHash"]]), **copy.deepcopy(row["params"]),
                    "messages": [copy.deepcopy(blobs[value]) for value in hashes]}
            requests.append({"id": row["id"], "session_id": row["sessionId"], "at": row["timestamp"],
                             "query_source": row.get("querySource"), "native_request_digest": row["requestDigest"], "body": body})
            previous = hashes
    return requests


def request_observation(request, markers):
    texts = list(text_values({key: request["body"][key] for key in ("system", "messages") if key in request["body"]}))
    return {key: value for key, value in request.items() if key != "body"} | {
        "model": request["body"].get("model"), "thinking": request["body"].get("thinking"),
        "output_config": request["body"].get("output_config"),
        "marker_locations": {marker: [{"path": path, "text_chars": len(text), "count": text.count(marker)}
                                      for path, text in texts if marker in text] for marker in markers}}


class LiveClaudeProbe(ClaudeProbe):
    def __init__(self, scratch, style, claude, auth_config):
        super().__init__(scratch, style, claude)
        self.auth_config = Path(auth_config).resolve()
        self.seen_requests = set()
        self.transcript_paths = set()
        self.live_env = os.environ.copy()
        for key in ("CODEX_HOME", "CODEX_SANDBOX", "CLAUDE_EFFORT"):
            self.live_env.pop(key, None)
        if any(self.live_env.get(key) for key in ("ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY")):
            raise ValueError("live acceptance requires the native first-party auth route")
        self.live_env.update(CLAUDE_CONFIG_DIR=str(self.auth_config), STYLE_HOME=str(self.store),
                             CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1", DISABLE_AUTOUPDATER="1")

    def invoke_live(self, name, prompt, extra=(), tools="", timeout=180, markers=(), model="claude-haiku-5-5"):
        original = self.env
        try:
            self.env = self.live_env
            call = self.invoke(name, prompt, extra, tools, timeout, setting_sources="",
                               settings=self.config / "settings.json", model=model)
        finally:
            self.env = original
        paths = list((self.auth_config / "projects").glob("**/" + str(call["session_id"]) + ".jsonl"))
        if len(paths) != 1:
            raise ValueError("expected one retained native transcript for " + str(call["session_id"]))
        rows = [json.loads(line) for line in paths[0].read_text().splitlines()]
        known_rows = [json.loads(line) for path in self.transcript_paths if path != paths[0]
                      for line in path.read_text().splitlines()]
        requests = replay_requests(rows, known_rows)
        current = [row for row in requests if row["id"] not in self.seen_requests]
        self.seen_requests.update(row["id"] for row in requests)
        call.update(transcript_path=str(paths[0]), transcript_sha256=digest(paths[0].read_bytes()),
                    replay_source_transcripts=[{"path": str(path), "sha256": digest(path.read_bytes())}
                                               for path in sorted(self.transcript_paths) if path != paths[0]],
                    native_requests=[request_observation(row, markers) for row in current])
        self.transcript_paths.add(paths[0])
        return call

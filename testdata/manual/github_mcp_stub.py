#!/usr/bin/env python3
"""Local stdio MCP stub named for the B7 probe: exposes create_pull_request, contacts nothing.

Mount it for one Codex run only, for example:
  codex exec -c 'mcp_servers.github.command="python3"' \
    -c 'mcp_servers.github.args=["-I", "testdata/manual/github_mcp_stub.py"]' ...
Every call returns a fixed STYLE_M1B_MCP_STUB text; no network or git access.
"""

import json
import sys

TOOL = {
    "name": "create_pull_request",
    "description": "STYLE M1b stub: pretends to create a pull request and returns a fixed marker.",
    "inputSchema": {
        "type": "object",
        "properties": {"title": {"type": "string"}, "body": {"type": "string"}},
        "required": ["title"],
    },
}


def reply(message_id, result):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": message_id, "result": result}) + "\n")
    sys.stdout.flush()


for line in sys.stdin:
    if not line.strip():
        continue
    message = json.loads(line)
    method, message_id = message.get("method"), message.get("id")
    if message_id is None:
        continue
    if method == "initialize":
        reply(message_id, {
            "protocolVersion": message.get("params", {}).get("protocolVersion", "2025-06-18"),
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "github-stub", "version": "0.1"},
        })
    elif method == "tools/list":
        reply(message_id, {"tools": [TOOL]})
    elif method == "tools/call":
        reply(message_id, {"content": [{"type": "text", "text": "STYLE_M1B_MCP_STUB: no pull request was created"}]})
    else:
        sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": message_id, "error": {"code": -32601, "message": "method not found"}}) + "\n")
        sys.stdout.flush()

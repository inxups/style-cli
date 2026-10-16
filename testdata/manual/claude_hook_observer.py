#!/usr/bin/env python3
"""Record only whitelisted native hook facts; do not wrap the timed STYLE hook."""

import datetime
import json
import os
from pathlib import Path
import sys


def observe(payload, environment):
    fields = ("hook_event_name", "session_id", "source", "model", "prompt_id", "agent_id", "agent_type", "tool_name")
    return {
        "at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "payload_keys": sorted(payload),
        "fields": {key: payload[key] for key in fields if key in payload},
        "effort": payload.get("effort"),
        "CLAUDE_EFFORT": environment.get("CLAUDE_EFFORT"),
        "CLAUDE_CODE_DISABLE_THINKING": environment.get("CLAUDE_CODE_DISABLE_THINKING"),
    }


def main():
    payload = json.load(sys.stdin)
    data = (json.dumps(observe(payload, os.environ), ensure_ascii=False) + "\n").encode()
    path = Path(sys.argv[1])
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(descriptor, data)
    finally:
        os.close(descriptor)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Independent author-native observer; raw payloads only in private scratch."""
import json
import os
from pathlib import Path
import sys
import time


def main():
    destination, marker = Path(sys.argv[1]), sys.argv[2]
    payload = sys.stdin.buffer.read()
    decoded = json.loads(payload)
    destination.mkdir(parents=True, exist_ok=True, mode=0o700)
    path = destination / ('%d-%d.json' % (time.time_ns(),os.getpid()))
    fd = os.open(path, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600)
    try:
        remaining=memoryview(payload)
        while remaining:
            written=os.write(fd,remaining)
            if not written:raise OSError('native payload write made no progress')
            remaining=remaining[written:]
    finally: os.close(fd)
    if marker:
        print(json.dumps({'hookSpecificOutput':{'hookEventName':decoded['hook_event_name'],'additionalContext':marker}}))


if __name__=='__main__': main()

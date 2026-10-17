#!/usr/bin/env python3
"""D10 real Claude CLI / loopback model protocol probes; no remote inference.

All configuration, raw requests and logs stay in a new private scratch.
The report emits only declared fixture facts, hashes and boolean observations.
"""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import sys
import threading
import time
from claude_probe_support import ClaudeProbe, text_values


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--scratch', required=True, type=Path)
    parser.add_argument('--claude', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    probe = ClaudeProbe(args.scratch, claude=args.claude)
    phases = []
    http_calls = []

    class HTTPHook(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_POST(self):
            body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
            payload = json.loads(body)
            http_calls.append({'method':'POST', 'event':payload.get('hook_event_name'),
                               'body_sha256':digest(body)})
            data = json.dumps({'hookSpecificOutput':{'hookEventName':'UserPromptSubmit',
                                                    'additionalContext':'M3_HTTP_CANARY'}}).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    http_server = ThreadingHTTPServer(('127.0.0.1', 0), HTTPHook)
    thread = threading.Thread(target=http_server.serve_forever, daemon=True)
    thread.start()
    try:
        key = probe.env['ANTHROPIC_API_KEY']
        (probe.config / '.claude.json').write_text(json.dumps(dict(hasCompletedOnboarding=True,
            theme='dark', customApiKeyResponses={'approved':[key[-20:]], 'rejected':[]})))
        settings = probe.config / 'settings.json'

        def invoke(name, handler, marker=''):
            settings.write_text(json.dumps({'hooks':{'UserPromptSubmit':[{'hooks':[handler]}]}}))
            call = probe.invoke(name, 'Reply OK.', timeout=40)
            requests = [probe.requests[i] for i in call['request_indices']]
            visible = any(marker in text for request in requests
                          for _, text in text_values({k:request['body'][k] for k in ('system','messages')
                                                      if k in request['body']})) if marker else None
            facts = dict(name=name, exit=call['exit'], requests=len(requests),
                         model_context_contains_marker=visible,
                         stdout_sha256=digest(Path(call['stdout_path']).read_bytes()),
                         stderr_sha256=digest(Path(call['stderr_path']).read_bytes()))
            phases.append(facts)
            return facts, requests

        for mode in ('unset', 'continue', 'block'):
            handler = {'type':'command', 'command':sys.executable,
                       'args':['-c','import sys; sys.stdin.read(); sys.exit(7)'], 'timeout':5}
            if mode != 'unset':
                handler['onFailure'] = mode
            facts, _ = invoke('failure-'+mode, handler)
            assert bool(facts['requests']) == (mode != 'block'), facts
        for async_mode in (False, True):
            marker = 'M3_BACKGROUND_CANARY' if async_mode else 'M3_SYNC_CANARY'
            script = probe.root / ('async.py' if async_mode else 'sync.py')
            ended = script.with_suffix('.ended')
            script.write_text('import sys,time,json\nfrom pathlib import Path\n'
                'sys.stdin.read()\ntime.sleep(2)\n'
                f'Path({str(ended)!r}).write_text(str(time.time_ns()))\n'
                f'print(json.dumps({{"hookSpecificOutput":{{"hookEventName":"UserPromptSubmit","additionalContext":{marker!r}}}}}))\n')
            handler = {'type':'command','command':sys.executable,'args':[str(script)],
                       'timeout':1 if async_mode else 5}
            if async_mode:
                handler['async'] = True
            # Keep the model-side response open so the background hook can finish.
            def response(_request):
                if async_mode:
                    time.sleep(3)
                return [{'type':'text', 'text':'OK'}]
            probe.responder = response
            facts, requests = invoke('async' if async_mode else 'sync', handler, marker)
            facts['background_finished'] = ended.exists()
            facts['timeout_seconds'] = handler['timeout']
            facts['first_request_precedes_hook_end'] = bool(ended.exists() and requests and
                Path(requests[0]['path']).stat().st_mtime_ns < int(ended.read_text()))
            assert facts['requests'] and facts['background_finished'], facts
            assert facts['model_context_contains_marker'] == (not async_mode), facts
            if async_mode:
                assert facts['first_request_precedes_hook_end'], facts
        probe.responder = lambda _: [{'type':'text','text':'OK'}]
        facts, _ = invoke('http', {'type':'http','url':f'http://127.0.0.1:{http_server.server_port}/hook',
                                  'timeout':5}, 'M3_HTTP_CANARY')
        assert facts['requests'] and facts['model_context_contains_marker'] and len(http_calls)==1, facts
        assert not probe.unexpected
        report = dict(format=1, design='M3-D10', scope='real-cli-loopback-model-only',
                      remote_model_inference=False, version=probe.version,
                      binary_sha256=digest(Path(probe.claude).read_bytes()), phases=phases,
                      http_calls=http_calls,
                      boundary='Local real CLI -p protocol evidence. No remote model, subscription login, hosted cloud or Windows. Prompt/agent/MCP variants assessed from pinned official documentation only.')
        args.output.write_text(json.dumps(report, indent=2)+'\n')
        print(json.dumps(dict(passed=len(phases), version=probe.version)))
    finally:
        http_server.shutdown()
        http_server.server_close()
        thread.join()
        probe.close()


if __name__ == '__main__':
    main()

"""M3 private real Responses capture on top of the historical native runner."""
import io
import json
import threading
from codex_probe_support import NativeProbe
from m3_probe_support import private_write, sha


class M3CodexProbe(NativeProbe):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.raw_requests = []; self.raw_lock = threading.Lock()
        parent = self.server.RequestHandlerClass
        probe = self

        class Handler(parent):
            def do_POST(self):
                if self.path == '/v1/traces':
                    return super().do_POST()
                raw = self.rfile.read(int(self.headers.get('Content-Length', 0)))
                # No HTTP headers or credentials enter these private files.
                with probe.raw_lock:
                    path = probe.root / ('native-request-%04d.private.json' % len(probe.raw_requests))
                    private_write(path, raw)
                    probe.raw_requests.append({'path':path, 'sha256':sha(raw)})
                original = self.rfile
                try:
                    self.rfile = io.BytesIO(raw)
                    return super().do_POST()
                finally:
                    self.rfile = original
        self.server.RequestHandlerClass = Handler

    def capture(self,name,thread,events,start,exit_code=0):
        # Intentional state corruption must remain present until the real
        # harness finishes. Read it tolerantly without repairing it for hooks.
        rollout=[]
        if thread:
            for path in (self.home/'sessions').rglob('*'+thread+'*.jsonl'):
                rollout=[json.loads(line) for line in path.read_text().splitlines()]
        state_path=self.store/'state'/str(thread)/'session.json'
        state=None;invalid=False
        if state_path.exists():
            try:state=json.loads(state_path.read_bytes())
            except json.JSONDecodeError:invalid=True
        stats_path=self.store/'state'/str(thread)/'main/stats.jsonl'
        stats=[json.loads(line) for line in stats_path.read_text().splitlines()] if stats_path.exists() else []
        call={'name':name,'exit':exit_code,'session_id':thread,'state':state,'state_decode_failed':invalid,'stats':stats,
              'requests':self.requests[start:],'events':events,'rollout_sha256':sha(json.dumps(rollout).encode())}
        self.calls.append(call)
        return call

    def delivery(self, call, body):
        observations = []
        for row in call['requests']:
            key = 'sha256:' + row['body_sha256']
            saved = [r for r in self.raw_requests if r['sha256'] == key]
            if not saved:
                raise ValueError('native request body missing')
            raw = saved[0]['path'].read_bytes()
            if sha(raw) != key:
                raise ValueError('native request body drift')
            request = json.loads(raw)
            count = 0
            for item in request.get('input', []):
                if item.get('role') == 'developer':
                    content = item.get('content', [])
                    if isinstance(content, str): content=[{'text':content}]
                    count += sum(part.get('text','').count(body) for part in content if isinstance(part,dict))
            observations.append(count)
        return {'body_sha256':sha(body.encode()), 'complete_body':bool(observations and observations[-1]),
                'request_occurrences': observations[-1] if observations else 0,
                'per_request_occurrences': observations}

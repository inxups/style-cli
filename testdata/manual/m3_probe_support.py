"""M3 real-acceptance bindings and private recovery primitives.

These checks establish reproducibility and independent native correlation, not
cryptographic authentication against another process running as the same user.
Raw native payloads, transcripts and configuration backups stay outside Git.
"""
import base64
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time


def sha(data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()


def private_write(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    try:
        os.fchmod(descriptor, 0o600)
        remaining = memoryview(data.encode() if isinstance(data, str) else data)
        while remaining:
            written = os.write(descriptor, remaining)
            if not written:
                raise OSError('private file write made no progress')
            remaining = remaining[written:]
    finally:
        os.close(descriptor)


class SourceBinding:
    def __init__(self, files):
        self.files = {label: Path(path).resolve() for label, path in files.items()}
        self.hashes = {label: sha(path.read_bytes()) for label, path in self.files.items()}

    def verify(self):
        for label, path in self.files.items():
            if not path.is_file() or sha(path.read_bytes()) != self.hashes[label]:
                raise ValueError('acceptance source drift: ' + label)
        return dict(self.hashes)

    def add_copy(self, label, path, original):
        path = Path(path).resolve()
        expected = sha(Path(original).read_bytes())
        if sha(path.read_bytes()) != expected:
            raise ValueError('acceptance copy drift: ' + label)
        self.files[label] = path
        self.hashes[label] = expected


def repository_binding(repo, style, harness):
    repo = Path(repo)
    names = subprocess.check_output(['git', 'ls-files', '-z'], cwd=repo).decode().split('\0')
    # Include new collectors before their first commit. Binary source includes
    # embedded vocabularies, schema and module locks, not just edited Go files.
    names += [str(p.relative_to(repo)) for p in (repo/'testdata/manual').glob('*m3*.py')]
    files = {name: repo/name for name in set(names) if name and not name.startswith('docs/') and (repo/name).is_file()}
    files.update(style_binary=style, harness_binary=harness)
    return SourceBinding(files)


class PrivateBaseline:
    """Explicit file set only; never read or serialize authentication stores."""
    def __init__(self, root, names):
        self.root = Path(root).resolve()
        self.records = {}
        for name in names:
            relative = Path(name)
            if relative.is_absolute() or '..' in relative.parts or any(x in name.lower() for x in ('auth', 'credential', 'keychain', '.claude.json')):
                raise ValueError('unsafe baseline input')
            path = self.path(name)
            self.records[name] = None if not path.exists() else (path.read_bytes(), path.stat().st_mode & 0o777)

    def path(self, name):
        path = self.root/name
        for candidate in (path, *path.parents):
            if candidate == self.root:
                break
            if candidate.is_symlink():
                raise ValueError('baseline symlink')
        return path

    def save(self, path):
        rows = {name: None if value is None else {'bytes': base64.b64encode(value[0]).decode(), 'mode': value[1]} for name, value in self.records.items()}
        private_write(path, json.dumps(rows, sort_keys=True))

    def verify(self):
        for name, value in self.records.items():
            path = self.path(name)
            if value is None:
                if path.exists():
                    raise ValueError('created config remains: ' + name)
            elif not path.is_file() or path.read_bytes() != value[0] or path.stat().st_mode & 0o777 != value[1]:
                raise ValueError('configuration not restored: ' + name)
        return {'files': len(self.records), 'bytes_and_modes_restored': True}

    def restore(self):
        for name, value in self.records.items():
            path = self.path(name)
            if value is None:
                path.unlink(missing_ok=True)
            else:
                private_write(path, value[0]); path.chmod(value[1])
        return self.verify()


def wait_probe(process, stderr, timeout=10):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if 'probe ready' in Path(stderr).read_text():
            return
        if process.poll() is not None:
            raise ValueError('probe exited before native launch')
        time.sleep(.025)
    raise TimeoutError('probe did not become ready')


def correlate_probe(report, harness, route, session, event, native_events, delivered, binding):
    binding.verify()
    if report.get('format') != 1 or report.get('status') != 'observed' or not report.get('cleanup', {}).get('complete'):
        raise ValueError('probe did not observe and clean up')
    targets = [row for row in report.get('targets', []) if row.get('harness') == harness and row.get('route') == route]
    if len(targets) != 1:
        raise ValueError('probe route mismatch')
    target = targets[0]
    if not target.get('callback_observed') or target.get('event') != event or target.get('session_sha256') != sha(session.encode()):
        raise ValueError('probe callback mismatch')
    if target.get('runtime_sha256') != binding.hashes['style_binary'] or target.get('harness_sha256') != binding.hashes['harness_binary']:
        raise ValueError('probe binary mismatch')
    try:
        observed_ns=int(datetime.fromisoformat(target['observed_at'].replace('Z','+00:00')).timestamp()*1_000_000_000)
    except (KeyError,ValueError,TypeError):
        raise ValueError('probe observation time missing') from None
    # The caller reads native OTEL/stream hook events independently of STYLE's
    # lease. A hand-invoked callback and its receipt cannot supply these rows.
    if not any(row.get('event') == event and row.get('source') == route and row.get('completed') is True
               and int(row.get('started_ns',0))-250_000_000<=observed_ns<=int(row.get('ended_ns',0))+250_000_000 for row in native_events):
        raise ValueError('independent native event missing')
    if not delivered or any(row.get('body_sha256', '').startswith('sha256:') is False or row.get('complete_body') is not True or row.get('request_occurrences', 0) < 1 for row in delivered):
        raise ValueError('complete body absent from real native request')
    return {'session_sha256': sha(session.encode()), 'event': event, 'route': route, 'native_events': len(native_events),'observed_at':target['observed_at'],
            'delivery': delivered, 'index_sha256': target.get('index_sha256'), 'cleanup_complete': True}

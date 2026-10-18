"""Run native Codex directly against the user-configured Responses service."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import tempfile
import time
import tomllib
from urllib.parse import urlsplit, urlunsplit

os.umask(0o077)
settings_path = Path('/Users/inxups/.style-openai/settings.json')
settings_bytes = settings_path.read_bytes()
settings = json.loads(settings_bytes)
assert set(settings) == {'request_url', 'api_key', 'model'}
assert all(isinstance(value, str) and value.strip() for value in settings.values())
key = settings['api_key'].strip()
assert len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key)
model = settings['model'].strip()
assert re.fullmatch(r'[A-Za-z0-9._:/-]{1,128}', model)
url = urlsplit(settings['request_url'].strip())
assert url.scheme == 'https' and url.hostname and not (url.username or url.password or url.query or url.fragment)
path = url.path.rstrip('/')
if path == '':
    path = '/v1'
elif path.endswith('/responses'):
    path = path[:-len('/responses')]
else:
    assert path.endswith('/v1'), 'Use a base URL or the full Responses endpoint'
base_url = urlunsplit((url.scheme, url.netloc, path, '', ''))
codex = shutil.which('codex')
assert codex
daily_paths = [Path('/Users/inxups/.codex') / name for name in ('config.toml', 'auth.json', 'AGENTS.md', 'hooks.json', 'settings.json')]
daily_paths += [Path('/Users/inxups/.claude/settings.json'), Path('/Users/inxups/.claude-style-official/settings.json')]

def sha(value):
    return hashlib.sha256(value).hexdigest()

def snapshot():
    result = {}
    for file in daily_paths:
        if not file.exists():
            result[str(file)] = None
            continue
        info = file.stat()
        result[str(file)] = (sha(file.read_bytes()), stat.S_IMODE(info.st_mode), info.st_mtime_ns, info.st_ino)
    return result

before = snapshot()
directory = Path(tempfile.mkdtemp(prefix='style-openai-codex-20261018-'))
for name in ('home', 'codex', 'work', 'tmp', 'config', 'cache'):
    (directory / name).mkdir(mode=0o700)
config = '\n'.join([
    'model = ' + json.dumps(model),
    'model_provider = "style_custom"',
    'model_reasoning_effort = "low"',
    'approval_policy = "never"',
    'sandbox_mode = "read-only"',
    'web_search = "disabled"',
    'cli_auth_credentials_store = "file"',
    '[features]',
    'shell_tool = false',
    '[model_providers.style_custom]',
    'name = "STYLE custom Responses"',
    'base_url = ' + json.dumps(base_url),
    'wire_api = "responses"',
    'env_key = "STYLE_OPENAI_API_KEY"',
    'requires_openai_auth = false',
    'supports_websockets = false',
    'request_max_retries = 0',
    'stream_max_retries = 0',
    'stream_idle_timeout_ms = 45000',
    ''
])
tomllib.loads(config)
config_path = directory / 'codex/config.toml'
config_path.write_text(config)
assert key not in config
env = {
    'HOME': str(directory / 'home'),
    'CODEX_HOME': str(directory / 'codex'),
    'XDG_CONFIG_HOME': str(directory / 'config'),
    'XDG_CACHE_HOME': str(directory / 'cache'),
    'TMPDIR': str(directory / 'tmp'),
    'PATH': str(Path(codex).parent) + ':/usr/bin:/bin:/usr/sbin:/sbin',
    'LANG': 'en_US.UTF-8',
    'TERM': 'dumb',
    'NO_COLOR': '1',
    'STYLE_OPENAI_API_KEY': key,
}
prompt = 'Reply exactly STYLE_OPENAI_OK. Do not use any tools.'
last_message = directory / 'last-message.txt'
command = [codex, 'exec', '--strict-config', '--skip-git-repo-check', '--ephemeral', '--ignore-rules', '--sandbox', 'read-only', '--color', 'never', '--json', '--output-last-message', str(last_message), '-']
started = time.monotonic()
timed_out = False
with (directory / 'stdout.jsonl').open('wb') as out, (directory / 'stderr.log').open('wb') as err:
    process = subprocess.Popen(command, cwd=directory / 'work', env=env, stdin=subprocess.PIPE, stdout=out, stderr=err, start_new_session=True)
    try:
        process.communicate(prompt.encode(), timeout=120)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
raw_stdout = (directory / 'stdout.jsonl').read_bytes()
raw_stderr = (directory / 'stderr.log').read_bytes()
events = []
invalid_lines = 0
for line in raw_stdout.splitlines():
    try:
        value = json.loads(line)
        assert isinstance(value, dict)
        events.append(value)
    except (ValueError, AssertionError):
        invalid_lines += 1
types = [event.get('type') for event in events]
completed = [event for event in events if event.get('type') == 'turn.completed']
tools = [event for event in events if event.get('type', '').startswith('item.') and event.get('item', {}).get('type') in ('command_execution', 'mcp_tool_call', 'web_search', 'custom_tool_call')]
last = last_message.read_text().strip() if last_message.exists() else ''
all_text = (raw_stdout + raw_stderr).decode(errors='replace')
error_categories = [label for label, pattern in {
    'http-401': r'\b401\b', 'http-403': r'\b403\b', 'http-404': r'\b404\b',
    'http-429': r'\b429\b', 'timeout': r'timed? out|timeout',
    'invalid-config': r'error loading config|unknown field|invalid configuration',
    'connection': r'connection (?:refused|reset)|failed to connect|dns error',
}.items() if re.search(pattern, all_text, re.I)]
checks = {
    'exit_zero': process.returncode == 0,
    'not_timed_out': not timed_out,
    'valid_json_events': invalid_lines == 0 and bool(events),
    'completed_turn': len(completed) == 1,
    'no_error_events': not any(t in ('error', 'turn.failed') for t in types),
    'exact_marker': last == 'STYLE_OPENAI_OK',
    'no_tool_calls': not tools,
    'daily_configuration_unchanged': snapshot() == before,
    'private_settings_unchanged': settings_path.read_bytes() == settings_bytes,
    'no_credentials_in_codex_config': key not in config,
}
usage = completed[0].get('usage', {}) if completed else {}
safe_usage = {name: value for name, value in usage.items() if name in ('input_tokens', 'cached_input_tokens', 'output_tokens') and type(value) is int and value >= 0}
report = {
    'format': 1, 'date': '2026-10-18', 'task': 'M3-CARRY-01', 'test': 'M3-CARRY-01',
    'purpose': 'Native isolated Codex CLI Responses connectivity only; not a forty-rule calibration.',
    'model': model, 'reasoning_effort': 'low', 'wire_api': 'responses',
    'custom_service': url.hostname != 'api.openai.com',
    'base_url_normalization': 'root HTTPS address plus /v1' if not url.path.rstrip('/') else 'explicit versioned base or Responses endpoint',
    'native_cli': codex, 'command': command,
    'private_directory': str(directory), 'collector_sha256': sha(Path(__file__).read_bytes()),
    'exit_code': process.returncode, 'duration_ms': round((time.monotonic() - started) * 1000),
    'checks': checks, 'passed': all(checks.values()), 'usage': safe_usage,
    'event_types': sorted(set(t for t in types if isinstance(t, str))),
    'error_categories': error_categories, 'raw_stdout_sha256': sha(raw_stdout), 'raw_stderr_sha256': sha(raw_stderr),
    'isolation': ['fresh HOME', 'fresh CODEX_HOME', 'fresh XDG_CONFIG_HOME', 'fresh XDG_CACHE_HOME', 'fresh empty work directory', 'explicit minimal environment', 'no inherited CODEX_SANDBOX/provider/proxy/OAuth variables', 'shell tools and web search disabled', 'read-only sandbox', 'ephemeral session', 'no HTTP forwarding or protocol adapter', 'request and stream retries disabled'],
    'official_openai_calibration_completed': 0,
    'anthropic_official_calibration_preserved': 40,
}
rendered = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
assert all(secret not in rendered for secret in (key, settings['request_url'].strip(), base_url))
report_path = directory / 'report.json'
report_path.write_text(rendered)
print(json.dumps({'report_path': str(report_path), 'passed': report['passed'], 'exit_code': process.returncode, 'duration_ms': report['duration_ms'], 'checks': checks, 'usage': safe_usage, 'event_types': report['event_types'], 'error_categories': error_categories}, ensure_ascii=False), flush=True)
raise SystemExit(0 if report['passed'] else 2)

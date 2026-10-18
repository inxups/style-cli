#!/usr/bin/env python3
"""Launch native Codex with the separate STYLE custom Responses profile."""
import json
import os
from pathlib import Path
import re
import shutil
import stat
import sys
import tempfile
from urllib.parse import urlsplit, urlunsplit


def main():
    os.umask(0o077)
    profile = Path('/Users/inxups/.style-openai')
    settings_path = profile / 'settings.json'
    if settings_path.is_symlink() or stat.S_IMODE(settings_path.stat().st_mode) != 0o600:
        raise ValueError('The private settings file must have mode 0600 and must not be a symlink.')
    settings = json.loads(settings_path.read_text())
    if set(settings) != {'request_url', 'api_key', 'model'}:
        raise ValueError('Expected request_url, api_key and model in the private settings file.')
    if not all(isinstance(value, str) and value.strip() for value in settings.values()):
        raise ValueError('Fill request_url, api_key and model in the private settings file.')
    key = settings['api_key'].strip()
    if len(key) > 4096 or not all(33 <= ord(c) <= 126 for c in key):
        raise ValueError('The API key contains unsupported characters.')
    model = settings['model'].strip()
    if not re.fullmatch(r'[A-Za-z0-9._:/-]{1,128}', model):
        raise ValueError('The model name contains unsupported characters.')
    url = urlsplit(settings['request_url'].strip())
    if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise ValueError('Use an HTTPS URL without embedded credentials, query or fragment.')
    path = url.path.rstrip('/')
    if not path:
        path = '/v1'
    elif path.endswith('/responses'):
        path = path[:-len('/responses')]
    elif not path.endswith('/v1'):
        raise ValueError('Use a versioned base URL or the full Responses endpoint.')
    base_url = urlunsplit((url.scheme, url.netloc, path, '', ''))
    codex = shutil.which('codex')
    if not codex:
        raise ValueError('Native codex CLI was not found on PATH.')
    for name in ('home', 'codex', 'tmp', 'config', 'cache'):
        directory = profile / name
        if directory.is_symlink():
            raise ValueError('The isolated profile directories must not be symlinks.')
        directory.mkdir(mode=0o700, exist_ok=True)
        directory.chmod(0o700)
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
    descriptor, temporary = tempfile.mkstemp(prefix='.config-', dir=profile / 'codex')
    try:
        with os.fdopen(descriptor, 'w') as stream:
            stream.write(config)
        os.replace(temporary, profile / 'codex/config.toml')
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    environment = {
        'HOME': str(profile / 'home'), 'CODEX_HOME': str(profile / 'codex'),
        'XDG_CONFIG_HOME': str(profile / 'config'), 'XDG_CACHE_HOME': str(profile / 'cache'),
        'TMPDIR': str(profile / 'tmp'),
        'PATH': str(Path(codex).parent) + ':/usr/bin:/bin:/usr/sbin:/sbin',
        'LANG': 'en_US.UTF-8', 'TERM': os.environ.get('TERM', 'xterm-256color'),
        'STYLE_OPENAI_API_KEY': key,
    }
    os.execve(codex, [codex, *sys.argv[1:]], environment)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError) as error:
        # Never include a URL, key or raw operating-system exception in diagnostics.
        message = str(error) if isinstance(error, ValueError) else 'Could not read or initialize the isolated profile.'
        # JSON parser errors may quote input; use a fixed diagnostic for them.
        if isinstance(error, json.JSONDecodeError):
            message = 'The private settings file is not valid JSON.'
        print('codex-style-openai: ' + message, file=sys.stderr)
        raise SystemExit(2)

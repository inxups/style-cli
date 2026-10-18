"""Measure the fixed forty-rule corpus using native Codex Responses input usage.

Usage: python3 THIS_FILE --output-dir /tmp/NEW-private-directory
Only native Codex contacts the configured service. No HTTP adapter is used.
This supplemental measurement does not replace official counter acceptance.
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
import tomllib
from urllib.parse import urlsplit, urlunsplit
from unittest.mock import patch

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'testdata/manual'))
sys.path.insert(0, str(ROOT / 'scripts'))
import calibrate_tokens as original
from check_m3_update import fixture, snapshot


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_usage(raw):
    events = [json.loads(line) for line in raw.splitlines() if line.strip()]
    assert events and all(isinstance(event, dict) for event in events), 'Invalid native events'
    assert not any(event.get('type') in ('error', 'turn.failed') for event in events), 'Native error event'
    turns = [event for event in events if event.get('type') == 'turn.completed']
    assert len(turns) == 1, 'Exactly one completed turn is required'
    items = [event['item'] for event in events if event.get('type', '').startswith('item.') and 'item' in event]
    assert all(item.get('type') in ('agent_message', 'reasoning') for item in items), 'Tool or unsupported item'
    usage = turns[0]['usage']
    selected = {name: usage[name] for name in ('input_tokens', 'cached_input_tokens', 'output_tokens')}
    assert all(type(value) is int and value >= 0 for value in selected.values()), 'Invalid native usage'
    assert selected['input_tokens'] > 0 and selected['cached_input_tokens'] <= selected['input_tokens'], 'Invalid input/cache usage'
    return selected


def metrics(rows):
    assert rows and all(row['incremental_input_tokens'] > 0 for row in rows)
    estimated = [row['style_estimate']['tokens'] for row in rows]
    counted = [row['incremental_input_tokens'] for row in rows]
    errors = [(estimate - tokens) / tokens for estimate, tokens in zip(estimated, counted)]
    ordered = sorted(map(abs, errors))
    return {'rules': len(rows), 'estimated_tokens': sum(estimated),
            'native_incremental_input_tokens': sum(counted),
            'aggregate_relative_error': (sum(estimated) - sum(counted)) / sum(counted),
            'mean_signed_relative_error': sum(errors) / len(errors),
            'mean_absolute_relative_error': sum(ordered) / len(ordered),
            'p95_absolute_relative_error': ordered[math.ceil(0.95 * len(ordered)) - 1],
            'max_absolute_relative_error': ordered[-1],
            'underestimated_rules': sum(error < 0 for error in errors),
            'overestimated_rules': sum(error > 0 for error in errors),
            'exact_rules': sum(error == 0 for error in errors)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--settings', type=Path, default=Path('/Users/inxups/.style-openai/settings.json'))
    parser.add_argument('--output-dir', type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    directory = args.output_dir.resolve()
    directory.mkdir(mode=0o700)  # Never overwrite previous evidence.
    private = args.settings.read_bytes()
    settings = json.loads(private)
    key, model = settings['api_key'].strip(), settings['model'].strip()
    assert key and len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key), 'Invalid private API key'
    assert re.fullmatch(r'[A-Za-z0-9._:/-]{1,128}', model), 'Invalid model'
    url = urlsplit(settings['request_url'].strip())
    assert url.scheme == 'https' and url.hostname and not (url.username or url.password or url.query or url.fragment), 'Invalid service URL'
    base_path = url.path.rstrip('/')
    if not base_path:
        base_path = '/v1'
    elif base_path.endswith('/responses'):
        base_path = base_path[:-len('/responses')]
    else:
        assert base_path.endswith('/v1'), 'Use a base URL or a full Responses URL'
    base_url = urlunsplit((url.scheme, url.netloc, base_path, '', ''))
    native = shutil.which('codex')
    assert native, 'Native codex CLI unavailable'
    corpus = original.load_corpus(original.CORPUS)
    built = json.loads((ROOT / 'docs/review/M3-REMOVE-01-2026-10-18.json').read_text())['binary']
    binary = Path(built['build']['command'][4])
    assert sha(binary.read_bytes()) == built['sha256'], 'Production binary changed'
    for name, digest in built['source_inputs'].items():
        assert 'sha256:' + sha((ROOT / name).read_bytes()) == digest, name
    with tempfile.TemporaryDirectory(prefix='style-native-calibration-plan-') as temporary:
        env, _, _, _, _ = fixture(Path(temporary))
        before_plan = snapshot(Path(temporary))
        with patch.dict(os.environ, env, clear=True):
            estimates = original.production_estimates(str(binary), corpus)
        assert snapshot(Path(temporary)) == before_plan, 'Production plan changed inputs'
    watched = [Path('/Users/inxups/.codex') / name for name in ('config.toml', 'auth.json', 'AGENTS.md', 'hooks.json', 'settings.json')]
    watched += [Path('/Users/inxups/.claude/settings.json'), Path('/Users/inxups/.claude-style-official/settings.json'),
                Path('/Users/inxups/.style-openai/codex/config.toml'), Path('/Users/inxups/.local/bin/codex-style-openai')]

    def daily_snapshot():
        result = {}
        for path in watched:
            if path.exists():
                info = path.stat()
                result[str(path)] = (sha(path.read_bytes()), stat.S_IMODE(info.st_mode), info.st_mtime_ns, info.st_ino)
            else:
                result[str(path)] = None
        return result

    before_daily = daily_snapshot()
    for name in ('home', 'codex', 'work', 'tmp', 'config', 'cache', 'runs'):
        (directory / name).mkdir(mode=0o700)
    instructions = directory / 'instructions.txt'
    config = '\n'.join([
        'model = ' + json.dumps(model), 'model_provider = "style_custom"',
        'model_reasoning_effort = "low"', 'approval_policy = "never"',
        'sandbox_mode = "read-only"', 'web_search = "disabled"',
        'cli_auth_credentials_store = "file"',
        'developer_instructions = ' + json.dumps('This is a token calibration run. Reply exactly OK in your final answer. Do not use tools or commentary. Do not continue into another model request.'),
        'model_instructions_file = ' + json.dumps(str(instructions)),
        '[features]', 'shell_tool = false', 'multi_agent = false',
        '[model_providers.style_custom]', 'name = "STYLE native calibration"',
        'base_url = ' + json.dumps(base_url), 'wire_api = "responses"',
        'env_key = "STYLE_OPENAI_API_KEY"', 'requires_openai_auth = false',
        'supports_websockets = false', 'request_max_retries = 0',
        'stream_max_retries = 0', 'stream_idle_timeout_ms = 45000', ''
    ])
    tomllib.loads(config)
    config_path = directory / 'codex/config.toml'
    config_path.write_text(config)
    env = {'HOME': str(directory / 'home'), 'CODEX_HOME': str(directory / 'codex'),
           'XDG_CONFIG_HOME': str(directory / 'config'), 'XDG_CACHE_HOME': str(directory / 'cache'),
           'TMPDIR': str(directory / 'tmp'), 'PATH': str(Path(native).parent) + ':/usr/bin:/bin:/usr/sbin:/sbin',
           'LANG': 'en_US.UTF-8', 'TERM': 'dumb', 'NO_COLOR': '1', 'STYLE_OPENAI_API_KEY': key}
    version = subprocess.run([native, '--version'], env=env, capture_output=True, text=True, timeout=15)
    assert version.returncode == 0 and re.fullmatch(r'codex-cli [\w.\-]+\n?', version.stdout)
    report = {'format': 1, 'date': '2026-10-18', 'task': 'M3-CARRY-01', 'test': 'M3-CARRY-01',
              'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
              'collector_sha256': sha(Path(__file__).read_bytes()),
              'measurement': 'First native Responses input usage from rollout token_usage_record.usage (cross-checked with token_count.last_token_usage) minus the fixed empty-rule baseline. Only model_instructions_file content varies; cached input is recorded, never subtracted. turn.completed usage is diagnostic only.',
              'service': 'user-configured-custom-responses' if url.hostname != 'api.openai.com' else 'user-configured-official-responses',
              'model': model, 'model_is_configured_name': True, 'upstream_model_identity_independently_verified': False,
              'cli_version': version.stdout.strip(), 'wire_api': 'responses', 'reasoning_effort': 'low',
              'corpus_sha256': sha(original.CORPUS.read_bytes()), 'style_binary_sha256': sha(binary.read_bytes()),
              'source_inputs_verified': len(built['source_inputs']), 'production_plan_inputs_unchanged': True,
              'user_message_sha256': sha(corpus['user_message'].encode()),
              'private_directory': str(directory), 'native_invocations': [], 'failed_native_invocations': [], 'rules': [], 'repeat_checks': [],
              'recovery_policy': 'At most one explicit rerun per label and at most three total, only for transport errors with zero recorded model responses. Every failed attempt is preserved; native request/stream retries remain disabled.',
              'completed': 0, 'passed': False, 'production_coefficients_changed': False, 'accuracy_threshold': None,
              'official_openai_calibration_completed': 0, 'anthropic_official_calibration_preserved': 40,
              'isolation': ['same fresh HOME/CODEX_HOME/XDG/tmp/work for all invocations', 'same native configuration, model and user message',
                            'only instruction file content varies', 'read-only with native rollouts retained privately', 'shell/multi-agent tools and search disabled',
                            'no inherited provider/OAuth/proxy/CODEX_SANDBOX', 'no HTTP adapter or forwarding', 'no request or stream retries'],
              'limits': ['Native CLI usage from the custom service is supplemental, not an official count endpoint result.',
                         'Only one completed turn with no tool use is accepted per invocation; the first response usage is measured separately from cumulative turn usage.',
                         'The CLI may include constant environment/envelope overhead; this is removed empirically by baseline subtraction.',
                         'Native instructions are trimmed exactly as recorded in session_meta; initial user/developer messages must remain identical across invocations. Full serialized request bytes are not intercepted.',
                         'No accuracy threshold, billing guarantee or production coefficient change.']}

    def save():
        rendered = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
        assert all(value not in rendered for value in (key, settings['request_url'].strip(), base_url, url.hostname))
        temporary = directory / '.report.json'
        temporary.write_text(rendered)
        temporary.replace(directory / 'report.json')

    def run(label, text, recovery=False):
        assert args.settings.read_bytes() == private, 'Private settings changed during measurement'
        assert config_path.read_text() == config, 'Native config changed during measurement'
        body = corpus['baseline_prefix'] + text
        instructions.write_text(body)
        work_before = snapshot(directory / 'work')
        run_dir = directory / 'runs' / str(len(report['native_invocations']) + len(report['failed_native_invocations'])).zfill(3)
        run_dir.mkdir(mode=0o700)
        command = [native, 'exec', '--strict-config', '--skip-git-repo-check', '--ignore-rules',
                   '--sandbox', 'read-only', '--color', 'never', '--json', '--output-last-message', str(run_dir / 'last.txt'), '-']
        started = time.monotonic()
        record = {'label': label, 'explicit_recovery_attempt': recovery, 'instructions_sha256': sha(body.encode()),
                  'config_template_sha256': sha(config.replace(base_url, '<private-base-url>').encode()),
                  'private_run_directory': str(run_dir)}
        report['native_invocations'].append(record)
        save()
        with (run_dir / 'stdout.jsonl').open('wb') as out, (run_dir / 'stderr.log').open('wb') as err:
            process = subprocess.Popen(command, cwd=directory / 'work', env=env, stdin=subprocess.PIPE,
                                       stdout=out, stderr=err, start_new_session=True)
            try:
                process.communicate(corpus['user_message'].encode(), timeout=120)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                record['timed_out'] = True
        raw = (run_dir / 'stdout.jsonl').read_bytes()
        record.update(exit_code=process.returncode, duration_ms=round(1000 * (time.monotonic() - started)),
                      raw_stdout_sha256=sha(raw), raw_stderr_sha256=sha((run_dir / 'stderr.log').read_bytes()))
        if process.returncode != 0:
            events = []
            for line in raw.splitlines():
                try:
                    events.append(json.loads(line))
                except ValueError:
                    pass
            threads = [event['thread_id'] for event in events if event.get('type') == 'thread.started']
            failed_responses = 0
            if threads:
                for file in (directory / 'codex/sessions').rglob('*' + threads[0] + '*.jsonl'):
                    failed_responses += sum(json.loads(line).get('type') == 'token_usage_record' for line in file.read_bytes().splitlines())
            text_for_classification = raw.decode(errors='replace')
            transport = bool(re.search(r'error sending request|stream disconnected|connection reset|timed out|timeout', text_for_classification, re.I))
            rejected_status = bool(re.search(r'\b(?:400|401|403|404|429)\b', text_for_classification))
            report['native_invocations'].pop()
            record.update(passed=False, error_category='transport' if transport else 'native-failure', recorded_model_responses=failed_responses)
            report['failed_native_invocations'].append(record)
            save()
            if transport and not rejected_status and failed_responses == 0 and not recovery and len(report['failed_native_invocations']) <= 3:
                print(json.dumps({'label': label, 'preserved_transport_failure': True, 'explicit_recovery': True}), flush=True)
                return run(label, text, recovery=True)
            raise RuntimeError('Native invocation did not succeed; failure preserved')
        turn_usage = read_usage(raw)
        events = [json.loads(line) for line in raw.splitlines() if line.strip()]
        thread_id = next(event['thread_id'] for event in events if event.get('type') == 'thread.started')
        rollouts = list((directory / 'codex/sessions').rglob('*' + thread_id + '*.jsonl'))
        assert len(rollouts) == 1, 'Exactly one native rollout is required'
        rollout = rollouts[0]
        rollout_raw = rollout.read_bytes()
        entries = [json.loads(line) for line in rollout_raw.splitlines() if line.strip()]
        info_rows = [entry['payload']['info'] for entry in entries if entry.get('type') == 'event_msg'
                     and entry.get('payload', {}).get('type') == 'token_count' and entry['payload'].get('info')]
        counts = [info['last_token_usage'] for info in info_rows if info.get('last_token_usage', {}).get('input_tokens', 0) > 0]
        assert counts, 'First-response native usage is unavailable'
        response_records = [entry['payload'] for entry in entries if entry.get('type') == 'token_usage_record']
        assert response_records and len({r['response_id'] for r in response_records}) == len(response_records), 'Missing or duplicate response usage identity'
        first = response_records[0]
        usage = {name: first['usage'][name] for name in ('input_tokens', 'cached_input_tokens', 'output_tokens')}
        assert usage == {name: counts[0][name] for name in usage}, 'Native response usage records disagree'
        assert all(type(number) is int and number >= 0 for number in usage.values()) and usage['input_tokens'] > 0
        meta = next(entry['payload'] for entry in entries if entry.get('type') == 'session_meta')
        actual_base = meta['base_instructions']['text']
        assert actual_base == body.strip(), 'Native base instructions do not match the trimmed fixture'
        initial = []
        for entry in entries:
            if entry.get('type') != 'response_item':
                continue
            payload = entry['payload']
            if payload.get('type') == 'message' and payload.get('role') in ('user', 'developer'):
                initial.append({'role': payload['role'], 'content': payload['content']})
            else:
                break
        assert initial, 'Native initial messages unavailable'
        initial_hash = sha(original.encoded(initial))
        if 'native_initial_messages_sha256' in report:
            assert report['native_initial_messages_sha256'] == initial_hash, 'Native initial messages changed'
        else:
            report['native_initial_messages_sha256'] = initial_hash
        record.update(turn_usage=turn_usage, rollout_path=str(rollout), rollout_sha256=sha(rollout_raw),
                      native_response_count=len(response_records), native_base_instructions_verified=True,
                      native_base_instructions_sha256=sha(actual_base.encode()), native_initial_messages_sha256=initial_hash,
                      response_id_sha256=sha(first['response_id'].encode()), first_response_usage=usage)
        assert instructions.read_text() == body and config_path.read_text() == config, 'Native inputs changed'
        assert snapshot(directory / 'work') == work_before, 'Native working files changed'
        record.update(usage=usage, working_files_unchanged=True, instructions_unchanged=True, passed=True)
        save()
        print(json.dumps({'native_runs_completed': len(report['native_invocations']), 'label': label,
                          'input_tokens': usage['input_tokens'], 'rules_completed': report['completed']}), flush=True)
        return record

    def repeat(label, text, expected):
        record = run(label, text)
        observed = record['usage']['input_tokens']
        report['repeat_checks'].append({'label': label, 'expected': expected, 'observed': observed, 'passed': observed == expected})
        save()
        assert observed == expected, 'Input usage is not stable; do not treat deltas as controlled measurements'

    try:
        baseline = run('baseline-initial', '')['usage']['input_tokens']
        report['baseline_input_tokens'] = baseline
        repeat('baseline-preflight-repeat', '', baseline)
        for rule in corpus['rules']:
            counted = run(rule['id'], rule['text'])
            delta = counted['usage']['input_tokens'] - baseline
            assert delta > 0, 'Nonpositive rule delta'
            report['rules'].append({'id': rule['id'], 'language': rule['language'], 'text_sha256': rule['sha256'],
                                    'style_estimate': estimates[rule['id']], 'input_tokens': counted['usage']['input_tokens'],
                                    'incremental_input_tokens': delta, 'instructions_sha256': counted['instructions_sha256'],
                                    'relative_error': (estimates[rule['id']]['tokens'] - delta) / delta})
            report['completed'] = len(report['rules'])
            save()
        for index in (0, 19, 20, 39):
            rule = corpus['rules'][index]
            repeat(rule['id'] + '-repeat', rule['text'], report['rules'][index]['input_tokens'])
        repeat('baseline-final-repeat', '', baseline)
        report['summary'] = {language: metrics([r for r in report['rules'] if language == 'all' or r['language'] == language])
                             for language in ('all', 'en', 'zh')}
        report['passed'] = report['completed'] == 40 and all(r['passed'] for r in report['repeat_checks'])
    except Exception as error:
        report['failure'] = {'error_type': type(error).__name__, 'last_label': report['native_invocations'][-1]['label'] if report['native_invocations'] else None}
    finally:
        report['daily_configuration_unchanged'] = daily_snapshot() == before_daily
        report['private_settings_unchanged'] = args.settings.read_bytes() == private
        report['native_invocation_count'] = len(report['native_invocations']) + len(report['failed_native_invocations'])
        report['successful_native_invocation_count'] = len(report['native_invocations'])
        report['passed'] = report['passed'] and report['daily_configuration_unchanged'] and report['private_settings_unchanged']
        save()
    print(json.dumps({'passed': report['passed'], 'completed': report['completed'],
                      'native_invocations': report['native_invocation_count'], 'report': str(directory / 'report.json'),
                      'summary': report.get('summary'), 'failure': report.get('failure')}, ensure_ascii=False), flush=True)
    return 0 if report['passed'] else 2


if __name__ == '__main__':
    raise SystemExit(main())

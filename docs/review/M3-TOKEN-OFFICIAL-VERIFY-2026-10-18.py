"""Verify official exact counting through the built production STYLE CLI.

Usage: python3 docs/review/M3-TOKEN-OFFICIAL-VERIFY-2026-10-18.py --settings PRIVATE_SETTINGS --report REPORT
The private settings contain env.ANTHROPIC_API_KEY. Credentials are never printed.
Only Count tokens requests are sent. Harness executables are version fixtures;
this script does not establish harness delivery or 40-text estimator calibration.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
from check_m3_update import fixture, manifest, run, sha, snapshot


def validate_count(value, model, text=None):
    assert value['mode'] == 'exact' and value['provider'] == 'anthropic'
    assert value['model'] == model and value['api_version'] == '2023-06-01'
    assert value['scope'] == 'anthropic-messages-request'
    assert type(value['tokens']) is int and value['tokens'] > 0
    assert value['text_sha256'].startswith('sha256:')
    assert value['request_sha256'].startswith('sha256:')
    if text is not None:
        digest = lambda b: 'sha256:' + hashlib.sha256(b).hexdigest()
        request = {'model': model, 'messages': [{'role': 'user', 'content': text}]}
        assert value['text_sha256'] == digest(text.encode())
        assert value['request_sha256'] == digest(json.dumps(request, ensure_ascii=False, separators=(',', ':')).encode())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--settings', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    os.umask(0o077)
    settings = json.loads(args.settings.read_text())
    assert settings['env']['ANTHROPIC_BASE_URL'] == 'https://api.anthropic.com'
    key = settings['env']['ANTHROPIC_API_KEY'].strip()
    model = settings['model']
    assert key and model == 'claude-haiku-5-5'
    removal = json.loads((ROOT / 'docs/review/M3-REMOVE-01-2026-10-18.json').read_text())['binary']
    binary = Path(removal['build']['command'][4])
    assert sha(binary) == removal['sha256']
    for name, digest in removal['source_inputs'].items():
        assert 'sha256:' + sha(ROOT / name) == digest, name
    report = {'format': 1, 'task': 'M3-REAL-03', 'test': 'M3-TOKEN-01',
              'model': model, 'api_version': '2023-06-01',
              'endpoint': 'https://api.anthropic.com/v1/messages/count_tokens',
              'binary_sha256': removal['sha256'], 'source_inputs_verified': len(removal['source_inputs']),
              'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
              'go_rebuilt': False, 'cases': [], 'passed': False,
              'environment': 'fresh fixture HOME/STYLE_HOME/CODEX_HOME/CLAUDE_CONFIG_DIR/PATH; CODEX_SANDBOX and external provider/effort/proxy environment absent; only official API key added',
              'scope': 'production CLI exact counting, not new harness delivery or 40-text estimator calibration'}
    flags = ('--count=exact', '--count-provider=anthropic', '--count-model=' + model)
    try:
        with tempfile.TemporaryDirectory(prefix='style-official-count-') as temporary:
            root = Path(temporary)
            env, home, codex, claude, _ = fixture(root)
            text = root / 'count.txt'
            text.write_text('Hello from STYLE.\n')
            before = snapshot(root)
            missing = run(binary, env, 'count', text, *flags, '--json', want=1)
            assert not missing.stdout and 'ANTHROPIC_API_KEY is required' in missing.stderr
            assert 'Hello from STYLE' not in missing.stderr and snapshot(root) == before
            report['cases'].append({'case': 'missing-key-local-failure', 'passed': True, 'stdout_empty': True, 'inputs_unchanged': True})
            env['ANTHROPIC_API_KEY'] = key
            specimens = [('english', 'Hello from STYLE.\n'),
                         ('chinese', '使用官方接口验证精确计数。\n'),
                         ('long-mixed', 'STYLE counts only selected text. 中文验证。\n' * 32),
                         ('english-repeat', 'Hello from STYLE.\n')]
            for name, body in specimens:
                text.write_text(body)
                before = snapshot(root)
                result = run(binary, env, 'count', text, *flags, '--json')
                assert not result.stderr
                value = json.loads(result.stdout)['count']
                validate_count(value, model, body)
                assert snapshot(root) == before
                report['cases'].append({'case': name, 'passed': True, 'count': value, 'inputs_unchanged': True})
            counted = {row['case']: row['count'] for row in report['cases'] if 'count' in row}
            assert counted['english'] == counted['english-repeat']
            assert counted['long-mixed']['tokens'] > max(counted['english']['tokens'], counted['chinese']['tokens'])
            source = root / 'source'
            source.mkdir()
            (source / 'STYLE.yaml').write_text(manifest('official-count', 'Official count verification.', native=True))
            before = snapshot(root)
            plan = json.loads(run(binary, env, 'plan', source, '--harness=claude-code', *flags, '--json').stdout)
            placements = plan['reports'][0]['counts']
            assert len(placements) == 2 and plan['reports'][0]['count_scope']
            for placement in placements:
                validate_count(placement['count'], model)
            assert snapshot(root) == before
            report['cases'].append({'case': 'plan-static-and-runtime-model-placements', 'passed': True, 'placements': placements, 'native_output_excluded': True, 'inputs_unchanged': True})
            run(binary, env, 'install', source, '--harness=claude-code', '--yes', '--json')
            payload = root / 'payload.json'
            payload.write_text(json.dumps({'session_id': 'official-count', 'hook_event_name': 'PreToolUse', 'tool_name': 'Bash', 'tool_input': {'command': 'true'}}))
            before = snapshot(root)
            explain = json.loads(run(binary, env, 'explain', '--harness=claude-code', '--event=PreToolUse', '--payload', payload, '--index', home / 'build/index.json', *flags, '--json').stdout)
            validate_count(explain['count'], model, 'Official count verification.')
            assert snapshot(root) == before
            report['cases'].append({'case': 'explain-final-model-text', 'passed': True, 'count': explain['count'], 'inputs_unchanged': True})
            run(binary, env, 'uninstall', '--all', '--yes', '--json')
        report['official_count_requests'] = 7
        report['passed'] = True
    except Exception as error:
        report['error'] = str(error).replace(key, '[redacted]')[:2000]
        raise
    finally:
        rendered = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
        assert key not in rendered
        args.report.write_text(rendered)
        args.report.chmod(0o600)
    print(json.dumps({'passed': True, 'cases': len(report['cases']), 'official_count_requests': 7, 'counts': {name: value['tokens'] for name, value in counted.items()}, 'binary_sha256': report['binary_sha256']}))


if __name__ == '__main__':
    main()

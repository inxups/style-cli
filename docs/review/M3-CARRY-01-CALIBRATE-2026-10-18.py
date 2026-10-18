"""Calibrate the fixed forty texts against official counters, without generation.

python3 docs/review/M3-CARRY-01-CALIBRATE-2026-10-18.py \
  --anthropic-settings PRIVATE_SETTINGS --output /tmp/calibration.json
OpenAI uses OPENAI_API_KEY only, if present. Missing credentials stay pending.
The frozen production binary and all its build inputs must still match.
"""
import argparse
import json
import math
import os
from pathlib import Path
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.request
from unittest.mock import patch

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'testdata/manual'))
sys.path.insert(0, str(ROOT / 'scripts'))
import calibrate_tokens as original
from check_m3_update import fixture, snapshot


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def metrics(rows):
    assert rows and all(row['incremental_input_tokens'] > 0 for row in rows)
    deltas = [row['incremental_input_tokens'] for row in rows]
    estimates = [row['style_estimate']['tokens'] for row in rows]
    errors = [(estimate - delta) / delta for estimate, delta in zip(estimates, deltas)]
    absolute = sorted(map(abs, errors))
    return {'rules': len(rows), 'estimated_tokens': sum(estimates),
            'official_incremental_tokens': sum(deltas),
            'aggregate_relative_error': (sum(estimates) - sum(deltas)) / sum(deltas),
            'mean_signed_relative_error': sum(errors) / len(errors),
            'mean_absolute_relative_error': sum(absolute) / len(absolute),
            'p95_absolute_relative_error': absolute[math.ceil(0.95 * len(absolute)) - 1],
            'max_absolute_relative_error': max(absolute),
            'underestimated_rules': sum(error < 0 for error in errors),
            'overestimated_rules': sum(error > 0 for error in errors),
            'exact_rules': sum(error == 0 for error in errors)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--anthropic-settings', required=True, type=Path)
    parser.add_argument('--openai-model', default='gpt-6.1-sol')
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    os.umask(0o077)
    assert not args.output.exists(), 'Preserve previous evidence; choose a new output path'
    config_bytes = args.anthropic_settings.read_bytes()
    settings = json.loads(config_bytes)
    assert settings['env']['ANTHROPIC_BASE_URL'] == 'https://api.anthropic.com'
    keys = {'anthropic': settings['env']['ANTHROPIC_API_KEY'].strip(),
            'openai': os.environ.get('OPENAI_API_KEY', '').strip()}
    assert keys['anthropic']
    for name in ('OPENAI_BASE_URL', 'OPENAI_API_BASE'):
        assert not os.environ.get(name) or os.environ[name].rstrip('/') in (
            'https://api.openai.com', 'https://api.openai.com/v1'), 'Nonofficial OpenAI configuration present'
    models = {'openai': args.openai_model, 'anthropic': settings['model']}
    built = json.loads((ROOT / 'docs/review/M3-REMOVE-01-2026-10-18.json').read_text())['binary']
    binary = Path(built['build']['command'][4])
    assert original.sha(binary.read_bytes()) == built['sha256']
    for name, digest in built['source_inputs'].items():
        assert 'sha256:' + original.sha((ROOT / name).read_bytes()) == digest, name
    corpus = original.load_corpus(original.CORPUS)
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile='/etc/ssl/cert.pem')))
    receipts = []
    journal = args.output.with_suffix('.requests.json')

    def write(path, value):
        rendered = json.dumps(value, ensure_ascii=False, indent=2) + '\n'
        assert all(not key or key not in rendered for key in keys.values())
        path.write_text(rendered)
        path.chmod(0o600)

    def count(provider, body, key):
        headers = {'Content-Type': 'application/json', 'User-Agent': 'style-cli-calibration/1.0'}
        if provider == 'anthropic':
            headers.update({'x-api-key': key, 'anthropic-version': '2023-06-01'})
        else:
            headers['Authorization'] = 'Bearer ' + key
        request_bytes = original.encoded(body)
        started = time.monotonic()
        receipt = {'provider': provider, 'request_sha256': original.sha(request_bytes)}
        try:
            request = urllib.request.Request(original.ENDPOINTS[provider], request_bytes, headers)
            with opener.open(request, timeout=45) as response:
                raw = response.read(1048577)
                assert response.status == 200 and len(raw) <= 1048576
                tokens = json.loads(raw)['input_tokens']
                assert type(tokens) is int and tokens >= 0
                request_id = response.headers.get('request-id', response.headers.get('x-request-id', ''))
                receipt.update(http_status=200, input_tokens=tokens,
                               response_sha256=original.sha(raw), request_id_sha256=original.sha(request_id.encode()))
                return {key: receipt[key] for key in ('input_tokens', 'response_sha256', 'request_id_sha256', 'http_status')}
        except Exception as error:
            receipt.update(error_type=type(error).__name__, http_status=getattr(error, 'code', None))
            raise
        finally:
            receipt['duration_ms'] = round(1000 * (time.monotonic() - started))
            receipts.append(receipt)
            write(journal, receipts)
            if len(receipts) % 10 == 0:
                print(json.dumps({'official_requests_completed': len(receipts)}), flush=True)

    with tempfile.TemporaryDirectory(prefix='style-carry-calibration-') as temporary:
        env, _, _, _, _ = fixture(Path(temporary))
        before = snapshot(Path(temporary))
        for provider, key in keys.items():
            if key:
                env['ANTHROPIC_API_KEY' if provider == 'anthropic' else 'OPENAI_API_KEY'] = key
        with patch.dict(os.environ, env, clear=True), patch.object(original, 'official_count', count):
            report = original.calibrate(str(binary), models)
        assert snapshot(Path(temporary)) == before
    report.update(item='M3-CARRY-01', format=1,
                  source_commit=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                  collector_sha256=original.sha(Path(__file__).read_bytes()),
                  original_collector_sha256=original.sha(Path(original.__file__).read_bytes()),
                  source_inputs_verified=len(built['source_inputs']),
                  anthropic_model_evidence='docs/review/M3-TOKEN-OFFICIAL-2026-10-18.json',
                  environment='fresh HOME/STYLE_HOME/CODEX_HOME/CLAUDE_CONFIG_DIR/PATH; inherited provider/proxy/effort/CODEX_SANDBOX absent',
                  fixture_inputs_unchanged=True, production_coefficients_changed=False,
                  accuracy_threshold=None, repeat_checks=[],
                  limits=['A completed measurement is not an accuracy threshold pass.',
                          'Same-shape system/instructions deltas apply only to this fixed corpus and model.',
                          'No generation, thinking effort, tools, cache billing or whole-session costs are measured.',
                          'Missing official API credentials remain pending; local tokenizers are not substituted.'])
    write(args.output, report)
    for provider, record in report['providers'].items():
        assert len(record['rules']) == 40
        if record['status'] != 'counted':
            continue
        for row in record['rules']:
            assert row['style_estimate']['chars'] == len(row['text'])
            assert row['sha256'] == original.sha(row['text'].encode())
            assert row['incremental_input_tokens'] == row['official_count']['input_tokens'] - record['baseline_count']['input_tokens']
        record['summary'] = {language: metrics([row for row in record['rules'] if language == 'all' or row['language'] == language])
                             for language in ('all', 'en', 'zh')}
        specimens = [('baseline', ''), *[(record['rules'][i]['id'], corpus['rules'][i]['text']) for i in (0, 19, 20, 39)]]
        for name, text in specimens:
            counted = count(provider, original.count_request(provider, record['model'], corpus, text), keys[provider])
            expected = record['baseline_count'] if name == 'baseline' else next(row['official_count'] for row in record['rules'] if row['id'] == name)
            report['repeat_checks'].append({'provider': provider, 'specimen': name, 'expected': expected['input_tokens'],
                                            'observed': counted['input_tokens'], 'passed': counted['input_tokens'] == expected['input_tokens']})
            write(args.output, report)
    report['measurement_completed_for'] = [p for p, r in report['providers'].items() if r['status'] == 'counted']
    report['available_provider_checks_passed'] = bool(report['measurement_completed_for']) and all(row['passed'] for row in report['repeat_checks'])
    report['official_requests'] = receipts
    report['official_request_count'] = len(receipts)
    report['official_settings_unchanged'] = args.anthropic_settings.read_bytes() == config_bytes
    assert report['official_settings_unchanged']
    write(args.output, report)
    print(json.dumps({'all_providers_completed': report['passed'],
                      'available_provider_checks_passed': report['available_provider_checks_passed'],
                      'providers': {p: {'status': r['status'], 'completed': r['completed'], 'summary': r.get('summary')}
                                    for p, r in report['providers'].items()},
                      'official_requests': len(receipts)}, ensure_ascii=False), flush=True)
    return 0 if report['passed'] and report['available_provider_checks_passed'] else 2


if __name__ == '__main__':
    raise SystemExit(main())

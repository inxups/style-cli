"""Offline verification of native Codex corpus measurements and documentation.

Usage: python3 THIS_FILE /tmp/NEW-verification.json
Reads the private native evidence; never starts a model, harness or Go build.
"""
import ast
from collections import Counter
import hashlib
import json
import math
from pathlib import Path
import re
import stat
import statistics
import subprocess
import sys
import unicodedata
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
BASELINE = '47c64493272ce888a6ca4da6aabdc6b43d1d0044'


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encoded(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def main():
    output = Path(sys.argv[1])
    assert not output.exists(), 'Preserve prior verification evidence'
    public_path = ROOT / 'docs/review/M3-CARRY-01-CODEX-2026-10-18.json'
    report = json.loads(public_path.read_text())
    assert report['suite_executed'] and report['available_measurements_verified']
    assert report['completed'] == len(report['rules'])
    assert report['completed'] + len(report['blocked_rules']) == report['attempted_rules'] == 40
    assert report['passed'] == (not report['blocked_rules'] and report['completed'] == 40)
    assert report['official_openai_calibration_completed'] == 0 and report['anthropic_official_calibration_preserved'] == 40
    assert not report['production_coefficients_changed'] and report['accuracy_threshold'] is None
    assert report['daily_configuration_unchanged'] and report['private_settings_unchanged']
    assert report['corpus_sha256'] == sha((ROOT / 'testdata/manual/token-calibration-40.json').read_bytes())
    corpus = json.loads((ROOT / 'testdata/manual/token-calibration-40.json').read_text())
    assert len(corpus['rules']) == 40 and Counter(r['language'] for r in corpus['rules']) == {'en': 20, 'zh': 20}
    assert (ROOT / 'testdata/manual/token-calibration-40.json').read_bytes() == git('show', BASELINE + ':testdata/manual/token-calibration-40.json')
    assert report['collector_sha256'] == sha((ROOT / 'docs/review/M3-CARRY-01-CODEX-CALIBRATE-2026-10-18.py').read_bytes())
    assert report['continuation_source_sha256'] == sha((ROOT / 'docs/review/M3-CARRY-01-CODEX-CONTINUE-2026-10-18.py').read_bytes())
    interrupted = Path(report['interrupted_report_path']).read_bytes()
    assert sha(interrupted) == report['interrupted_report_sha256']
    assert not json.loads(interrupted)['passed']
    built = json.loads((ROOT / 'docs/review/M3-REMOVE-01-2026-10-18.json').read_text())['binary']
    assert report['style_binary_sha256'] == built['sha256']
    assert sha(Path(built['build']['command'][4]).read_bytes()) == built['sha256']
    for name, digest in built['source_inputs'].items():
        assert 'sha256:' + sha((ROOT / name).read_bytes()) == digest, name
    blocked_ids = {row['id'] for row in report['blocked_rules']}
    measured_ids = {row['id'] for row in report['rules']}
    assert not blocked_ids & measured_ids and blocked_ids | measured_ids == {row['id'] for row in corpus['rules']}
    labels = ['baseline-initial', 'baseline-preflight-repeat', *[r['id'] for r in corpus['rules'] if r['id'] not in blocked_ids],
              *[corpus['rules'][i]['id'] + '-repeat' for i in (0, 19, 20, 39) if corpus['rules'][i]['id'] not in blocked_ids], 'baseline-final-repeat']
    invocations = report['native_invocations']
    assert report['successful_native_invocation_count'] == len(invocations) == len(labels)
    assert report['native_invocation_count'] == len(invocations) + len(report['failed_native_invocations'])
    assert sum(row['error_category'] == 'transport' for row in report['failed_native_invocations']) <= 3
    assert [run['label'] for run in invocations] == labels
    mapping = {run['label']: run for run in invocations}
    baseline = report['baseline_input_tokens']
    assert baseline > 0 and all(mapping[label]['usage']['input_tokens'] == baseline for label in labels if label.startswith('baseline-'))
    context_hashes = set()
    config_hashes = set()
    native_model_responses = 0
    for run in invocations:
        assert run['passed'] and run['exit_code'] == 0
        assert run['working_files_unchanged'] and run['instructions_unchanged'] and run['native_base_instructions_verified']
        label = run['label']
        rule_id = label.removesuffix('-repeat')
        text = '' if label.startswith('baseline-') else next(row['text'] for row in corpus['rules'] if row['id'] == rule_id)
        expected = corpus['baseline_prefix'] + text
        assert run['instructions_sha256'] == sha(expected.encode())
        assert run['native_base_instructions_sha256'] == sha(expected.strip().encode())
        rollout_path = Path(run['rollout_path'])
        raw = rollout_path.read_bytes()
        assert sha(raw) == run['rollout_sha256'] and stat.S_IMODE(rollout_path.stat().st_mode) == 0o600
        entries = [json.loads(line) for line in raw.splitlines() if line.strip()]
        response_records = [entry['payload'] for entry in entries if entry['type'] == 'token_usage_record']
        assert len(response_records) == run['native_response_count'] >= 1
        assert len(set(row['response_id'] for row in response_records)) == len(response_records)
        native_model_responses += len(response_records)
        first = response_records[0]
        assert sha(first['response_id'].encode()) == run['response_id_sha256']
        assert run['usage'] == run['first_response_usage'] == {key: first['usage'][key] for key in ('input_tokens', 'cached_input_tokens', 'output_tokens')}
        counts = [entry['payload']['info']['last_token_usage'] for entry in entries
                  if entry['type'] == 'event_msg' and entry['payload'].get('type') == 'token_count'
                  and entry['payload'].get('info') and entry['payload']['info'].get('last_token_usage', {}).get('input_tokens', 0) > 0]
        assert all(counts[0][key] == run['usage'][key] for key in run['usage'])
        assert all(type(value) is int and value >= 0 for value in run['usage'].values())
        assert run['usage']['input_tokens'] > 0 and run['usage']['cached_input_tokens'] <= run['usage']['input_tokens']
        meta = next(entry['payload'] for entry in entries if entry['type'] == 'session_meta')
        assert meta['base_instructions']['text'] == expected.strip()
        assert 'codex-cli ' + meta['cli_version'] == report['cli_version']
        assert meta['model_provider'] == 'style_custom'
        assert Path(meta['cwd']).resolve() == (Path(report['private_directory']) / 'work').resolve()
        contexts = [entry['payload'] for entry in entries if entry['type'] == 'turn_context']
        assert contexts and all(context['model'] == report['model'] for context in contexts)
        initial = []
        for entry in entries:
            if entry['type'] != 'response_item':
                continue
            payload = entry['payload']
            if payload.get('type') == 'message' and payload.get('role') in ('user', 'developer'):
                initial.append({'role': payload['role'], 'content': payload['content']})
            else:
                break
        assert initial and sha(encoded(initial)) == run['native_initial_messages_sha256']
        assert initial[-1]['role'] == 'user'
        assert ''.join(part.get('text', '') for part in initial[-1]['content']) == corpus['user_message']
        context_hashes.add(run['native_initial_messages_sha256'])
        config_hashes.add(run['config_template_sha256'])
        directory = Path(run['private_run_directory'])
        for name, digest in (('stdout.jsonl', run['raw_stdout_sha256']), ('stderr.log', run['raw_stderr_sha256'])):
            assert sha((directory / name).read_bytes()) == digest
        events = [json.loads(line) for line in (directory / 'stdout.jsonl').read_bytes().splitlines() if line.strip()]
        assert not any(event['type'] in ('error', 'turn.failed') for event in events)
        completed = [event for event in events if event['type'] == 'turn.completed']
        assert len(completed) == 1
        assert all(completed[0]['usage'][key] == run['turn_usage'][key] for key in run['turn_usage'])
        assert all(event['item']['type'] in ('agent_message', 'reasoning') for event in events if 'item' in event)
    assert len(context_hashes) == len(config_hashes) == 1
    assert report['native_initial_messages_sha256'] in context_hashes
    assert report['user_message_sha256'] == sha(corpus['user_message'].encode())
    prior = json.loads((ROOT / 'docs/review/M3-CARRY-01-ANTHROPIC-2026-10-18.json').read_text())
    prior_estimates = {row['id']: row['style_estimate'] for row in prior['providers']['anthropic']['rules']}
    for failed in report['failed_native_invocations']:
        assert not failed['passed'] and failed['exit_code'] != 0
        assert failed['recorded_model_responses'] == 0
        if failed['label'] in blocked_ids:
            assert failed['service_rejection_category'] == 'policy-rejection'
            assert not failed['explicit_recovery_attempt']
            block = next(row for row in report['blocked_rules'] if row['id'] == failed['label'])
            assert block['status'] == 'blocked-service-policy' and block['input_tokens'] is None and block['incremental_input_tokens'] is None
            original = next(row for row in corpus['rules'] if row['id'] == block['id'])
            assert block['text_sha256'] == original['sha256']
            error_text = (Path(failed['private_run_directory']) / 'stdout.jsonl').read_text()
            assert 'flagged as potentially violating our usage policy' in error_text
        else:
            assert failed['error_category'] == 'transport' and mapping[failed['label']]['explicit_recovery_attempt']
        directory = Path(failed['private_run_directory'])
        for name, digest in (('stdout.jsonl', failed['raw_stdout_sha256']), ('stderr.log', failed['raw_stderr_sha256'])):
            assert sha((directory / name).read_bytes()) == digest
        failed_events = [json.loads(line) for line in (directory / 'stdout.jsonl').read_bytes().splitlines() if line.strip()]
        thread = next(row['thread_id'] for row in failed_events if row['type'] == 'thread.started')
        for file in (Path(report['private_directory']) / 'codex/sessions').rglob('*' + thread + '*.jsonl'):
            assert not any(json.loads(line)['type'] == 'token_usage_record' for line in file.read_bytes().splitlines() if line.strip())
    assert [row['id'] for row in report['rules']] == [row['id'] for row in corpus['rules'] if row['id'] not in blocked_ids]
    for row in report['rules']:
        expected = next(original for original in corpus['rules'] if original['id'] == row['id'])
        assert row['id'] == expected['id'] and row['language'] == expected['language']
        assert row['text_sha256'] == expected['sha256'] == sha(expected['text'].encode())
        assert row['input_tokens'] == mapping[row['id']]['usage']['input_tokens']
        assert row['incremental_input_tokens'] == row['input_tokens'] - baseline > 0
        assert row['instructions_sha256'] == mapping[row['id']]['instructions_sha256']
        assert row['style_estimate']['chars'] == len(expected['text'])
        assert row['style_estimate'] == prior_estimates[row['id']]
        error = (row['style_estimate']['tokens'] - row['incremental_input_tokens']) / row['incremental_input_tokens']
        assert math.isclose(row['relative_error'], error, abs_tol=1e-14)
    repeats = report['repeat_checks']
    assert len(repeats) == 2 + sum(corpus['rules'][i]['id'] not in blocked_ids for i in (0, 19, 20, 39))
    assert all(row['passed'] and row['observed'] == row['expected'] == mapping[row['label']]['usage']['input_tokens'] for row in repeats)
    for language, recorded in report['summary'].items():
        rows = [row for row in report['rules'] if language == 'all' or row['language'] == language]
        estimated = sum(row['style_estimate']['tokens'] for row in rows)
        counted = sum(row['incremental_input_tokens'] for row in rows)
        errors = [(row['style_estimate']['tokens'] - row['incremental_input_tokens']) / row['incremental_input_tokens'] for row in rows]
        absolute = sorted(map(abs, errors))
        calculated = {'rules': len(rows), 'estimated_tokens': estimated, 'native_incremental_input_tokens': counted,
                      'aggregate_relative_error': (estimated - counted) / counted,
                      'mean_signed_relative_error': statistics.mean(errors), 'mean_absolute_relative_error': statistics.mean(absolute),
                      'p95_absolute_relative_error': absolute[math.ceil(len(rows) * 0.95) - 1], 'max_absolute_relative_error': absolute[-1],
                      'underestimated_rules': sum(error < 0 for error in errors), 'overestimated_rules': sum(error > 0 for error in errors),
                      'exact_rules': sum(error == 0 for error in errors)}
        assert calculated.keys() == recorded.keys()
        assert all(math.isclose(recorded[key], value, abs_tol=1e-14) for key, value in calculated.items())
    for name, directory in (('TURN', 'native'), ('TRIM', 'response'), ('TRANSPORT', 'final')):
        rejected = json.loads((ROOT / f'docs/review/M3-CARRY-01-CODEX-{name}-2026-10-18.json').read_text())
        assert not rejected['passed'] and not rejected['controlled_measurement_accepted']
        archive = Path(f'/tmp/style-m3-carry-codex-{directory}-20261018/collector.py')
        assert sha(archive.read_bytes()) == rejected['collector_sha256']

    # Reuse only pure Markdown helpers, without running the historical verifier.
    tree = ast.parse((ROOT / 'docs/review/M3-CARRY-01-VERIFY-2026-10-18.py').read_text())
    helpers = ast.Module(body=[node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in ('declared', 'anchors')], type_ignores=[])
    namespace = {'re': re, 'unicodedata': unicodedata}
    exec(compile(helpers, 'historical-markdown-helpers', 'exec'), namespace)
    for name, count in (('docs/STYLE-M3-TASKS.md', 45), ('docs/STYLE-M3-TESTS.md', 40)):
        states = namespace['declared'](ROOT / name)
        old = {item: state for state, item in re.findall(r'^- \[([x ~!?-])\] \*\*(M3-[A-Z0-9-]+)', git('show', BASELINE + ':' + name).decode(), re.M)}
        assert len(states) == count and states == old
    tests = namespace['declared'](ROOT / 'docs/STYLE-M3-TESTS.md')
    covered = set()
    for line in (ROOT / 'docs/STYLE-M3-TASKS.md').read_text().splitlines():
        if not re.match(r'^- \[[x ~!?-]\] \*\*M3-', line):
            continue
        reference = re.search(r'测试：([^。]+)', line)
        assert reference, line
        expanded = re.sub(r'M3-([A-Z]+)(\d+)–([A-Z]+)(\d+)',
                          lambda m: '、'.join('M3-' + m[1] + str(i) for i in range(int(m[2]), int(m[4]) + 1)) if m[1] == m[3] else m[0], reference[1])
        ids = set(re.findall(r'M3-[A-Z0-9-]+', expanded))
        assert ids and ids <= tests.keys()
        covered.update(ids)
    assert covered == tests.keys()
    originals = git('ls-tree', '-r', '--name-only', BASELINE).decode().splitlines()
    archives = [p for p in originals if re.match(r'docs/(?:STYLE-M[12]B?(?:-|\.)|review/M[12]B?[-.])', p)]
    reviews = [p for p in originals if p.startswith('docs/review/')]
    goldens = [p for p in originals if p.startswith(('testdata/', 'agenthooks/')) and ('golden' in p or p.endswith('.golden'))]
    for name in set(archives + reviews + goldens):
        assert (ROOT / name).read_bytes() == git('show', BASELINE + ':' + name), name
    assert (ROOT / 'docs/PROGRESS.md').read_bytes().startswith(git('show', BASELINE + ':docs/PROGRESS.md'))
    changed = set(git('diff', '--name-only', BASELINE).decode().splitlines())
    changed.update(git('ls-files', '--others', '--exclude-standard').decode().splitlines())
    assert all(path.startswith('docs/') or path == 'testdata/manual/README.md' for path in changed), changed
    links = fences = 0
    settings = json.loads(Path('/Users/inxups/.style-openai/settings.json').read_text())
    secrets = [settings['api_key'].strip(), settings['request_url'].strip(), urlsplit(settings['request_url']).hostname]
    for name in sorted(changed):
        file = ROOT / name
        text = file.read_text()
        assert all(secret not in text for secret in secrets if secret), (name, 'Private value in change')
        if name.endswith('.py'):
            ast.parse(text)
        if not name.endswith('.md'):
            continue
        opened = False
        for line in text.splitlines():
            if re.match(r'^\s*```', line):
                opened = not opened
                fences += 1
        assert not opened, name
        for target in re.findall(r'(?<!!)\[[^\]]*\]\(([^)]+)\)', text):
            target = target.strip().split(' "')[0].strip('<>')
            if re.match(r'^[A-Za-z][A-Za-z0-9+.-]*:', target):
                continue
            path, _, fragment = target.partition('#')
            resolved = (file.parent / unquote(path)).resolve() if path else file
            assert resolved.exists(), (name, target)
            if fragment and resolved.suffix == '.md':
                assert unquote(fragment) in namespace['anchors'](resolved), (name, target)
            links += 1
    subprocess.run(['git', 'diff', '--check'], cwd=ROOT, check=True)
    result = {'passed': True, 'baseline_commit': BASELINE, 'public_report_sha256': sha(public_path.read_bytes()),
              'verifier_sha256': sha(Path(__file__).read_bytes()), 'rules_attempted': 40, 'rules_measured': report['completed'],
              'rules_blocked': len(blocked_ids), 'native_invocations': report['native_invocation_count'],
              'native_model_responses': native_model_responses,
              'failed_transport_invocations': sum(row['error_category'] == 'transport' for row in report['failed_native_invocations']),
              'service_policy_rejections': len(blocked_ids),
              'successful_native_invocations': len(invocations), 'repeat_checks': len(repeats), 'metrics_independently_recomputed': 3,
              'native_rollouts_usage_and_controlled_inputs_verified': True, 'rejected_preflights_preserved': 3,
              'task_ids': 45, 'test_ids': 40, 'all_tests_mapped': True, 'milestone_states_unchanged': True,
              'archives_unchanged': len(archives), 'historical_review_files_unchanged': len(reviews),
              'goldens_unchanged': len(goldens), 'progress_append_only': True,
              'source_inputs_verified': len(built['source_inputs']), 'local_links_and_anchors': links,
              'fence_markers': fences, 'private_values_absent_from_changes': True, 'git_diff_check': True,
              'network_requests': 0, 'native_harness_launches': 0, 'go_tests_rerun': False}
    with output.open('x') as stream:
        stream.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()

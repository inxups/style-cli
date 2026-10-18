"""Continue an interrupted native measurement without changing its inputs.

Usage: python3 THIS_FILE --directory /tmp/style-m3-carry-codex-measured-20261018
Reuses the frozen collector's exact run function and retains every prior log.
Service policy rejections are recorded as blocked, never rewritten or retried.
"""
import argparse
import ast
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from types import SimpleNamespace
from unittest.mock import patch

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', required=True, type=Path)
    args = parser.parse_args()
    os.umask(0o077)
    directory = args.directory.resolve()
    report_path = directory / 'report.json'
    previous = report_path.read_bytes()
    report = json.loads(previous)
    assert not report['passed'] and 'continuation_source_sha256' not in report, 'Preserve completed or already continued evidence'
    source = ROOT / 'docs/review/M3-CARRY-01-CODEX-CALIBRATE-2026-10-18.py'
    spec = importlib.util.spec_from_file_location('frozen_native_collector', source)
    collector = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(collector)
    assert collector.sha(source.read_bytes()) == report['collector_sha256'], 'Frozen collector changed'
    settings_path = Path('/Users/inxups/.style-openai/settings.json')
    private = settings_path.read_bytes()
    settings = json.loads(private)
    config_path = directory / 'codex/config.toml'
    config = config_path.read_text()
    parsed = collector.tomllib.loads(config)
    assert parsed['model'] == settings['model'] == report['model']
    base_url = parsed['model_providers']['style_custom']['base_url']
    assert base_url == settings['request_url'].rstrip('/') + '/v1', 'This continuation expects the accepted root URL configuration'
    assert parsed['model_instructions_file'] == str(directory / 'instructions.txt')
    key = settings['api_key'].strip()
    native = shutil.which('codex')
    assert native
    corpus = collector.original.load_corpus(collector.original.CORPUS)
    assert report['corpus_sha256'] == collector.sha(collector.original.CORPUS.read_bytes())
    built = json.loads((ROOT / 'docs/review/M3-REMOVE-01-2026-10-18.json').read_text())['binary']
    binary = Path(built['build']['command'][4])
    assert collector.sha(binary.read_bytes()) == built['sha256'] == report['style_binary_sha256']
    for name, digest in built['source_inputs'].items():
        assert 'sha256:' + collector.sha((ROOT / name).read_bytes()) == digest
    with tempfile.TemporaryDirectory(prefix='style-native-continue-plan-') as temporary:
        plan_env, _, _, _, _ = collector.fixture(Path(temporary))
        before_plan = collector.snapshot(Path(temporary))
        with patch.dict(os.environ, plan_env, clear=True):
            estimates = collector.original.production_estimates(str(binary), corpus)
        assert collector.snapshot(Path(temporary)) == before_plan
    watched = [Path('/Users/inxups/.codex') / name for name in ('config.toml', 'auth.json', 'AGENTS.md', 'hooks.json', 'settings.json')]
    watched += [Path('/Users/inxups/.claude/settings.json'), Path('/Users/inxups/.claude-style-official/settings.json'),
                Path('/Users/inxups/.style-openai/codex/config.toml'), Path('/Users/inxups/.local/bin/codex-style-openai')]
    def daily_snapshot():
        result = {}
        for path in watched:
            if path.exists():
                info = path.stat()
                result[str(path)] = (collector.sha(path.read_bytes()), info.st_mode, info.st_mtime_ns, info.st_ino)
            else:
                result[str(path)] = None
        return result

    before_daily = daily_snapshot()
    old_daily_unchanged = report['daily_configuration_unchanged']
    old_settings_unchanged = report['private_settings_unchanged']
    archive = directory / 'report.before-continuation.json'
    with archive.open('xb') as stream:
        stream.write(previous)
    report.update(continuation_source_sha256=collector.sha(Path(__file__).read_bytes()),
                  interrupted_report_sha256=collector.sha(previous), interrupted_report_path=str(archive),
                  interrupted_failure=report.pop('failure', None), blocked_rules=[], available_measurements_verified=False,
                  requested_rules=40, suite_executed=False)
    env = {'HOME': str(directory / 'home'), 'CODEX_HOME': str(directory / 'codex'),
           'XDG_CONFIG_HOME': str(directory / 'config'), 'XDG_CACHE_HOME': str(directory / 'cache'),
           'TMPDIR': str(directory / 'tmp'), 'PATH': str(Path(native).parent) + ':/usr/bin:/bin:/usr/sbin:/sbin',
           'LANG': 'en_US.UTF-8', 'TERM': 'dumb', 'NO_COLOR': '1', 'STYLE_OPENAI_API_KEY': key}

    def save():
        rendered = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
        assert all(value not in rendered for value in (key, settings['request_url'].strip(), base_url))
        temporary = directory / '.report.json'
        temporary.write_text(rendered)
        temporary.replace(report_path)

    def policy_rejected(record):
        raw = (Path(record['private_run_directory']) / 'stdout.jsonl').read_text()
        return ('flagged as potentially violating our usage policy' in raw
                and record['recorded_model_responses'] == 0)

    def record_blocked(rule, record):
        record['service_rejection_category'] = 'policy-rejection'
        report['blocked_rules'].append({'id': rule['id'], 'language': rule['language'], 'text_sha256': rule['sha256'],
                                        'status': 'blocked-service-policy', 'native_run_directory': record['private_run_directory'],
                                        'input_tokens': None, 'incremental_input_tokens': None})
        print(json.dumps({'rule': rule['id'], 'status': 'blocked-service-policy', 'count_available': False}), flush=True)
        save()

    for record in report['failed_native_invocations']:
        if policy_rejected(record):
            rule = next(row for row in corpus['rules'] if row['id'] == record['label'])
            record_blocked(rule, record)
        else:
            assert record['error_category'] == 'transport', 'Unclassified prior failure'
    # Lift only the original run function; all request/config/rollout guards stay identical.
    tree = ast.parse(source.read_text())
    main_node = next(node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name == 'main')
    run_node = next(node for node in main_node.body if isinstance(node, ast.FunctionDef) and node.name == 'run')
    namespace = dict(vars(collector))
    namespace.update(args=SimpleNamespace(settings=settings_path), private=private, settings=settings, key=key,
                     report=report, directory=directory, config=config, config_path=config_path,
                     base_url=base_url, instructions=directory / 'instructions.txt', env=env, native=native,
                     corpus=corpus, save=save)
    exec(compile(ast.Module(body=[run_node], type_ignores=[]), str(source), 'exec'), namespace)
    run = namespace['run']
    baseline = report['baseline_input_tokens']
    try:
        completed = {row['id'] for row in report['rules']}
        blocked = {row['id'] for row in report['blocked_rules']}
        for rule in corpus['rules']:
            if rule['id'] in completed or rule['id'] in blocked:
                continue
            try:
                counted = run(rule['id'], rule['text'])
            except RuntimeError:
                record = report['failed_native_invocations'][-1]
                if record['label'] == rule['id'] and policy_rejected(record):
                    record_blocked(rule, record)
                    blocked.add(rule['id'])
                    continue
                raise
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
            if rule['id'] in blocked:
                continue
            counted = run(rule['id'] + '-repeat', rule['text'])
            expected = next(row['input_tokens'] for row in report['rules'] if row['id'] == rule['id'])
            observed = counted['usage']['input_tokens']
            report['repeat_checks'].append({'label': rule['id'] + '-repeat', 'expected': expected, 'observed': observed, 'passed': observed == expected})
            save()
            assert observed == expected, 'Repeated rule input usage changed'
        counted = run('baseline-final-repeat', '')
        observed = counted['usage']['input_tokens']
        report['repeat_checks'].append({'label': 'baseline-final-repeat', 'expected': baseline, 'observed': observed, 'passed': observed == baseline})
        assert observed == baseline, 'Baseline changed after continuation'
        report['summary'] = {language: collector.metrics([row for row in report['rules'] if language == 'all' or row['language'] == language])
                             for language in ('all', 'en', 'zh')}
        report['attempted_rules'] = len(report['rules']) + len(report['blocked_rules'])
        report['suite_executed'] = report['attempted_rules'] == 40
        report['available_measurements_verified'] = report['suite_executed'] and all(row['passed'] for row in report['repeat_checks'])
    except Exception as error:
        report['continuation_failure'] = {'error_type': type(error).__name__}
    finally:
        report['daily_configuration_unchanged'] = old_daily_unchanged and daily_snapshot() == before_daily
        report['private_settings_unchanged'] = old_settings_unchanged and settings_path.read_bytes() == private
        report['successful_native_invocation_count'] = len(report['native_invocations'])
        report['native_invocation_count'] = len(report['native_invocations']) + len(report['failed_native_invocations'])
        report['available_measurements_verified'] = (report['available_measurements_verified']
                                                     and report['daily_configuration_unchanged'] and report['private_settings_unchanged'])
        report['passed'] = report['available_measurements_verified'] and report['completed'] == 40 and not report['blocked_rules']
        save()
    print(json.dumps({'suite_executed': report['suite_executed'], 'measured': report['completed'], 'blocked': len(report['blocked_rules']),
                      'available_measurements_verified': report['available_measurements_verified'], 'passed': report['passed'],
                      'summary': report.get('summary'), 'report': str(report_path), 'failure': report.get('continuation_failure')}, ensure_ascii=False), flush=True)
    return 0 if report['available_measurements_verified'] else 2


if __name__ == '__main__':
    raise SystemExit(main())

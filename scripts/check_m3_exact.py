#!/usr/bin/env python3
"""M3-A15: real built CLI encodings and isolated estimate/wire compatibility.

Version-only harness fixtures do not establish real harness delivery or API use.
"""
import argparse
import json
from pathlib import Path
import tempfile
from check_m3_update import fixture, manifest, record, run, sha, snapshot


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--binary', required=True, type=Path)
    p.add_argument('--old-binary', required=True, type=Path)
    p.add_argument('--report', required=True, type=Path)
    args = p.parse_args()
    binary, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    golden = json.loads(Path('testdata/count/exact-text-v1.json').read_text())
    with tempfile.TemporaryDirectory(prefix='style-m3-exact-') as temporary:
        root = Path(temporary).resolve()
        env, home, codex, claude, stub = fixture(root)
        text = root / "text ' 中文.txt"
        for item in golden['cases']:
            text.write_bytes(item['text'].encode())
            before = snapshot(root)
            for encoding, expected in item['tokens'].items():
                result = json.loads(run(binary, env, 'count', text, '--exact=openai:'+encoding,
                                        '--json').stdout)['count']
                assert result['tokens'] == expected and result['text_sha256'] == 'sha256:'+item['sha256']
                assert result['scope'] == 'utf8-text-encoding' and result['implementation'].endswith('@v0.7.0')
                assert snapshot(root) == before
                record(cases, 'local-encoding', item['id'], encoding=encoding, tokens=expected,
                       text_sha256=item['sha256'])
        text.write_text('PRIVATE COUNT INPUT')
        before = snapshot(root)
        failure = run(binary, env, 'count', text, '--exact=anthropic:claude-fixture',
                      '--json', want=1)
        assert not failure.stdout and 'PRIVATE COUNT INPUT' not in failure.stderr
        assert 'ANTHROPIC_API_KEY' in failure.stderr and snapshot(root) == before
        record(cases, 'api', 'missing-key-fails-without-text-disclosure')
        for provider in ('codex', 'claude-code'):
            source = root / ('source-' + provider)
            source.mkdir()
            (source / 'STYLE.yaml').write_text(manifest('counted', 'hello world', native=True))
            baseline = run(old, env, 'plan', source, '--harness='+provider, '--json')
            for mode in ((),):
                result = run(binary, env, 'plan', source, '--harness='+provider, *mode, '--json')
                assert (result.stdout, result.stderr) == (baseline.stdout, baseline.stderr)
                record(cases, provider, 'plan-default-estimate-byte-compatibility', explicit=bool(mode))
            exact_args = ('--exact=openai:o200k_base',)
            counted = json.loads(run(binary, env, 'plan', source, '--harness='+provider, *exact_args, '--json').stdout)
            report = counted['reports'][0]
            assert len(report.pop('counts')) == 2 and report.pop('count_scope')
            assert counted == json.loads(baseline.stdout)
            record(cases, provider, 'plan-counts-model-text-excludes-native')
            run(old, env, 'install', source, '--harness='+provider, '--yes')
            payload = root / 'payload.json'
            payload.write_text(json.dumps(dict(session_id='s', hook_event_name='PreToolUse',
                                              tool_name='Bash', tool_input={'command':'true'})))
            explain_args = ('explain', '--harness='+provider, '--event=PreToolUse', '--payload', payload,
                            '--index', home / 'build/index.json', '--json')
            before = snapshot(home, codex, claude, source, payload.parent / 'not-present')
            baseline = run(old, env, *explain_args)
            for mode in ((),):
                result = run(binary, env, *explain_args, *mode)
                assert (result.stdout, result.stderr) == (baseline.stdout, baseline.stderr)
                record(cases, provider, 'explain-default-estimate-byte-compatibility', explicit=bool(mode))
            result = json.loads(run(binary, env, *explain_args, *exact_args).stdout)
            count = result.pop('count')
            assert count['tokens'] == 2 and count['scope'] == 'utf8-text-encoding'
            assert result == json.loads(baseline.stdout) and snapshot(home, codex, claude, source) == before
            record(cases, provider, 'explain-final-render-count-wire-and-inputs-unchanged')
            run(old, env, 'uninstall', '--all', '--yes')
    args.report.write_text(json.dumps(dict(format=1, acceptance='M3-A15',
        binary_sha256=sha(binary), old_binary_sha256=sha(old),
        golden_sha256=sha(Path('testdata/count/exact-text-v1.json')), cases=cases), indent=2)+'\n')
    print(json.dumps(dict(passed=len(cases))))


if __name__ == '__main__':
    main()

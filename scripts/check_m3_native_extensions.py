#!/usr/bin/env python3
"""Old native installs require reinstall; native mappings retain four fields.

Harness commands are version-only fixtures. No native author command runs.
"""
import argparse
import json
from pathlib import Path
import shutil
import tempfile
from check_m3_update import fixture, manifest, record, run, sha, snapshot
from style_json import report as style_report


def legacy_manifest(name):
    # Intentional historical input, accepted only by the old binary.
    return manifest(name, 'Native baseline.', native=True).replace(
        'native_hooks:\n  - id: executable\n',
        '  - id: executable\n    target: native\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--old-binary', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    binary, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    for route in ('codex-user', 'codex-plugin', 'claude-user', 'codex-project', 'claude-project'):
        provider = 'claude-code' if route.startswith('claude-') else 'codex'
        with tempfile.TemporaryDirectory(prefix='style-native-migration-') as temporary:
            root = Path(temporary).resolve()
            env, base, codex, claude, stub = fixture(root)
            runner = stub / 'style'
            shutil.copyfile(old, runner)
            runner.chmod(0o700)
            project = root / 'project space'
            project.mkdir()
            source = root / 'source'
            source.mkdir()
            source_file = source / 'STYLE.yaml'
            source_file.write_text(legacy_manifest('native-compat'))
            scope = ('--project', project) if route.endswith('-project') else ()
            selected = ('--harness=' + provider, '--route=' + provider + '=' + route)
            installed = style_report(run(runner, env, 'install', source, *scope, *selected, '--yes', '--json').stdout)
            home = Path(installed['scope']['home']) if scope else base
            if route == 'codex-plugin':
                hooks = home / 'adapters/codex/plugin/hooks/hooks.json'
            elif scope:
                hooks = project / ('.codex/hooks.json' if provider == 'codex' else '.claude/settings.json')
            else:
                hooks = codex / 'hooks.json' if provider == 'codex' else claude / 'settings.json'
            foreign = [dict(type='http', url='https://example.invalid/foreign', headers={'X-Fixture': 'unchanged'}),
                       dict(type='prompt', prompt='Foreign prompt must remain'),
                       dict(type='agent', prompt='Foreign agent must remain'),
                       dict(type='mcp_tool', server='foreign', tool='inspect', input={'fixture': 'unchanged'}),
                       dict(type='command', command='printf FOREIGN', asyncRewake=True)] if provider == 'claude-code' else [
                           {'type': 'command', 'command': 'printf FOREIGN', 'async': False, 'timeout': 9}]
            config = json.loads(hooks.read_text())
            config['hooks'].setdefault('Stop', []).append({'hooks': foreign})
            hooks.write_text(json.dumps(config, indent=2) + '\n')
            run(runner, env, 'sync', *scope, '--json')
            second = root / 'second'
            second.mkdir()
            (second / 'STYLE.yaml').write_text(legacy_manifest('old-other'))
            run(runner, env, 'install', second, *scope, *selected, '--yes', '--json')
            shutil.copyfile(binary, runner)
            before = snapshot(home, codex, claude, project)
            operations = [('validate', source, '--json'),
                          ('plan', source, '--harness=' + provider, '--json'),
                          (*scope, 'sync', '--json'),
                          (*scope, 'sync', 'native-compat', '--json'),
                          (*scope, 'update', '--all', '--json'),
                          (*scope, 'restore', '--json')]
            for operation in operations:
                result = run(runner, env, *operation, want=1, confirm=True)
                output = result.stdout + result.stderr
                assert 'native-format-removed' in output and 'native_hooks' in output and 'uninstall' in output, output
                assert snapshot(home, codex, claude, project) == before, operation
                record(cases, route, 'legacy-rejected-without-writes', command=operation[0])
            listed = style_report(run(runner, env, *scope, 'list', '--json').stdout)
            assert len(listed['packages']) == 2
            assert all('native-format-removed' in item['error'] for item in listed['packages'])
            doctor = style_report(run(runner, env, *scope, 'doctor', '--json', want=1).stdout)
            assert any(d['code'] == 'native-format-removed' for d in doctor['diagnostics'])
            record(cases, route, 'legacy-visible-in-list-and-doctor')
            # Migrating upstream does not bypass the old authenticated approval.
            source_file.write_text(manifest('native-compat', 'Native baseline.', native=True))
            result = run(runner, env, *scope, 'update', 'native-compat', '--json', want=1, confirm=True)
            assert 'native-format-removed' in result.stdout + result.stderr
            assert snapshot(home, codex, claude, project) == before
            result = run(runner, env, *scope, 'uninstall', 'native-compat', '--json', want=1, confirm=True)
            assert 'uninstall --all' in result.stdout + result.stderr
            assert snapshot(home, codex, claude, project) == before
            run(runner, env, *scope, 'uninstall', '--all', '--json', confirm=True)
            run(runner, env, *scope, 'install', source, *selected, '--json', confirm=True)
            run(runner, env, *scope, 'sync', '--json')
            assert any(group['hooks'] == foreign for group in json.loads(hooks.read_text())['hooks']['Stop'])
            assert 'native-compat/executable' not in run(runner, env, *scope, 'stats', '--json').stdout
            index = (home / 'build/index.json').read_text()
            assert 'NATIVE-' not in index and 'native-compat/executable' not in index
            planned = style_report(run(runner, env, 'plan', source, '--harness=' + provider, '--json').stdout)
            assert 'NATIVE-' not in json.dumps(planned)
            record(cases, route, 'reinstall-recovers-and-excludes-hooks-from-runtime')
            candidate = root / 'candidate'
            candidate.mkdir()
            for extension in ('async: true', 'onFailure: block', 'type: http', 'headers: {Authorization: PRIVATE_VALUE}'):
                (candidate / 'STYLE.yaml').write_text(
                    'spec: style/v0.3\nname: candidate\nversion: 1.0.0\ndescription: rejection fixture\nnative_hooks:\n'
                    f"  - {{id: hook, {provider}: {{event: Stop, command: 'touch {root / 'NEVER'}', {extension}}}}}\n")
                before = snapshot(home, codex, claude, project, candidate)
                result = run(runner, env, *scope, 'install', candidate, *selected, '--json', want=1, confirm=True)
                output = result.stdout + result.stderr
                assert 'not supported;' in output and 'PRIVATE_VALUE' not in output
                assert snapshot(home, codex, claude, project, candidate) == before and not (root / 'NEVER').exists()
                record(cases, route, 'extension-rejection-zero-write', field=extension.split(':', 1)[0])
            run(runner, env, *scope, 'uninstall', '--all', '--json', confirm=True)
            assert any(group['hooks'] == foreign for group in json.loads(hooks.read_text())['hooks']['Stop'])
            assert '--owner=style' not in hooks.read_text()
            record(cases, route, 'uninstall-preserves-foreign-types-and-fields')
    args.report.write_text(json.dumps(dict(format=1, acceptance='native-hooks-migration', binary_sha256=sha(binary),
        old_binary_sha256=sha(old), real_harness=False, cases=cases), indent=2) + '\n')
    print(json.dumps(dict(passed=len(cases))))


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""M3-A12 isolated CLI unknown matching and genuine T42 compatibility gates."""
import argparse
import json
from pathlib import Path
import tempfile
from check_m3_update import fixture, run, snapshot, record, sha
from check_m3_refresh import handler
from check_m3_project import invoke_hook
from style_json import report as style_report

MANIFEST = """spec: style/v0.3
name: missing-data
version: 1.0.0
description: explicit unknown fixture
rules:
  - {id: r, target: event, event: tool.pre, when: {model: unknown}, text: 'Unknown rule'}
"""


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
        with tempfile.TemporaryDirectory(prefix='style-m3-unknown-') as temp:
            root = Path(temp).resolve()
            env, base, codex, claude, stub = fixture(root)
            (stub / 'style').symlink_to(binary)
            project = root / "project ' space"
            project.mkdir()
            source = root / 'source'
            source.mkdir()
            (source / 'STYLE.yaml').write_text(MANIFEST)
            scope = ('--project', project) if route.endswith('-project') else ()
            selection = ('--harness', provider, '--route', provider + '=' + route)
            before = snapshot(base, codex, claude, project)
            preview = style_report(run(binary, env, 'install', source, *scope, *selection, '--json', want=1).stdout)
            assert preview['awaiting_confirmation'] and snapshot(base, codex, claude, project) == before
            record(cases, route, 'preview-zero-write')
            installed = style_report(run(binary, env, 'install', source, *scope, *selection, '--json', confirm=True).stdout)
            home = Path(installed['scope']['home']) if scope else base
            index_path = home / 'build/index.json'
            index = json.loads(index_path.read_text())
            assert index['format'] == 5
            lock = (home / 'style.lock.yaml').read_text()
            assert 'min_index_format: 5' in lock and 'format: 1\n' not in lock
            if route == 'codex-plugin':
                hook_path = home / 'adapters/codex/plugin/hooks/hooks.json'
                env['PLUGIN_ROOT'] = str(hook_path.parent.parent)
            elif scope:
                hook_path = project / ('.codex/hooks.json' if provider == 'codex' else '.claude/settings.json')
            else:
                hook_path = codex / 'hooks.json' if provider == 'codex' else claude / 'settings.json'
            record(cases, route, 'index5-strict-lock-marker')
            for mode, value, want in [('missing', None, True), ('empty', '', True), ('known', 'model-known', False), ('literal', 'unknown', False)]:
                native = dict(session_id='unknown-' + mode, hook_event_name='PreToolUse',
                              tool_name='Bash', tool_input={'command': 'true'}, cwd=str(project))
                if value is not None:
                    native['model'] = value
                payload = json.dumps(native)
                payload_file = root / 'payload.json'
                payload_file.write_text(payload)
                before = snapshot(home / 'state', hook_path.parent)
                explained = style_report(run(binary, env, 'explain', '--harness', provider, '--event', 'PreToolUse',
                                           '--payload', payload_file, '--index', index_path, '--json').stdout)
                assert snapshot(home / 'state', hook_path.parent) == before
                actual = invoke_hook(handler(hook_path, 'PreToolUse'), env, project, payload)
                assert actual.returncode == 0 and 'level=ERROR' not in actual.stderr, actual.stderr
                assert actual.stdout == explained['wire']['stdout'], (route, mode, actual.stdout, explained['wire'])
                assert ('Unknown rule' in actual.stdout) == want
                again = invoke_hook(handler(hook_path, 'PreToolUse'), env, project, payload)
                assert 'Unknown rule' not in again.stdout
            record(cases, route, 'missing-empty-known-literal-readonly-explain-actual-codec-dedup')
            before = snapshot(base, codex, claude, project)
            rejected = run(old, dict(env, STYLE_HOME=str(home)), 'agenthooks', 'run', '--provider=' + provider, payload=payload)
            assert 'Unknown rule' not in rejected.stdout
            assert 'index-invalid' in rejected.stderr or 'index is unavailable' in rejected.stderr
            assert snapshot(base, codex, claude, project) == before
            for command in (('sync',), ('update', '--all', '--yes'), ('trust', 'missing-data', '--yes'),
                            ('install', '--frozen', '--yes'), ('install', source, *selection, '--yes'),
                            ('uninstall', '--all', '--yes')):
                run(old, dict(env, STYLE_HOME=str(home)), *command, want=1)
                assert snapshot(base, codex, claude, project) == before
            record(cases, route, 'actual-T42-runtime-six-management-commands-refuse-zero-write')
            for command in (('sync',), ('update', '--all'), ('restore',)):
                run(binary, env, *command, *scope, '--json', confirm=True)
            doctor = style_report(run(binary, env, 'doctor', *scope, '--json').stdout)
            assert any(d['code'] == 'runtime-index-5' for d in doctor['diagnostics'])
            (home / 'build/index.json').unlink()
            (home / 'build/explain.json').unlink()
            run(binary, env, 'restore', *scope, '--json', confirm=True)
            assert json.loads(index_path.read_text()) == index
            assert (home / 'style.lock.yaml').read_text() == lock
            record(cases, route, 'new-management-frozen-exact-lock-rebuild-doctor')
            (source / 'STYLE.yaml').write_text(MANIFEST.replace('when: {model: unknown}, ', ''))
            run(binary, env, 'update', '--all', *scope, '--json', confirm=True)
            assert 'min_index_format' not in (home / 'style.lock.yaml').read_text()
            assert json.loads(index_path.read_text())['format'] == 1
            # User lock2 remains legal. Project lock3 was already understood by T42.
            run(old, dict(env, STYLE_HOME=str(home)), 'sync', *scope, '--json')
            record(cases, route, 'remove-last-unknown-clears-marker-old-manager-compatible')
            run(binary, env, 'uninstall', '--all', *scope, '--json', confirm=True)
            record(cases, route, 'uninstall')
    args.report.write_text(json.dumps(dict(format=1, binary_sha256=sha(binary),
                                          old_binary_sha256=sha(old), real_harness=False, cases=cases), indent=2) + '\n')
    print(json.dumps(dict(checks=len(cases), passed=True)))


if __name__ == '__main__':
    main()

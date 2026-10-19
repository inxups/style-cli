#!/usr/bin/env python3
"""M3-A13 compiled user mappings, native codec replay and genuine old binary gates."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
from check_m3_update import fixture, run, snapshot, record, sha
from check_m3_refresh import handler
from check_m3_project import invoke_hook

MANIFEST = """spec: style/v0.3
name: effort-policy
version: 1.0.0
description: effort mapping fixture
rules:
  - id: r
    target: event
    event: tool.pre
    repeat: every
    reason: mapping fixture
    text: 'Effort={{thinking_effort | default: "missing"}}/raw={{thinking_effort_raw | default: "missing"}}'
"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--old-binary', required=True, type=Path)
    parser.add_argument('--codex-ancestor', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    binary, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    for route in ('codex-user', 'codex-plugin', 'claude-user', 'codex-project', 'claude-project'):
        provider = 'claude-code' if route.startswith('claude-') else 'codex'
        with tempfile.TemporaryDirectory(prefix='style-m3-effort-') as temp:
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
            installed = json.loads(run(binary, env, 'install', source, *scope, *selection, '--yes', '--json').stdout)
            home = Path(installed['scope']['home']) if scope else base
            index_path = home / 'build/index.json'
            assert json.loads(index_path.read_text())['format'] == 1
            mapping_file = home / 'effort.yaml'
            def config(tier):
                mapping_file.write_text('format: 1\nharnesses:\n  ' + provider + ': {future: ' + tier + '}\n')
            config('high')
            planned = json.loads(run(binary, env, 'plan', source, *scope, '--harness', provider, '--json').stdout)
            assert any(d['code'] == 'effort-mapping' for d in planned['diagnostics'])
            run(binary, env, 'sync', *scope, '--json')
            index = json.loads(index_path.read_text())
            assert index['format'] == 6 and index['effort_mapping'][provider] == {'future': 'high'}
            lock = (home / 'style.lock.yaml').read_text()
            assert 'min_index_format: 6' in lock and 'effort_mapping:' in lock
            record(cases, route, 'old-install-upgrade-plan-compiled-snapshot-and-strict-lock')
            if route == 'codex-plugin':
                hook_path = home / 'adapters/codex/plugin/hooks/hooks.json'
                env['PLUGIN_ROOT'] = str(hook_path.parent.parent)
            elif scope:
                hook_path = project / ('.codex/hooks.json' if provider == 'codex' else '.claude/settings.json')
            else:
                hook_path = codex / 'hooks.json' if provider == 'codex' else claude / 'settings.json'
            native = dict(session_id='mapped', hook_event_name='PreToolUse', tool_name='Bash',
                          tool_input={'command': 'true'}, cwd=str(project))
            if provider == 'claude-code':
                native['effort'] = {'level': 'future'}
            payload = json.dumps(native)
            payload_file = root / 'payload.json'
            payload_file.write_text(payload)
            facts_file = root / 'facts.json'
            facts_file.write_text(json.dumps(dict(format=1, harness=provider,
                                                   payload_sha256='sha256:' + hashlib.sha256(payload.encode()).hexdigest(),
                                                   **{('codex_config_effort' if provider == 'codex' else 'claude_env_effort'): 'future'})))
            def invoke():
                hook = handler(hook_path, 'PreToolUse')
                if provider != 'codex':
                    return invoke_hook(hook, env, project, payload)
                return subprocess.run([str(args.codex_ancestor.resolve()), '-c', 'model_reasoning_effort=future'],
                                      env=env, input=json.dumps(dict(hook=hook, cwd=str(project), payload=payload)),
                                      text=True, capture_output=True, timeout=20)
            def parity(expected, cached=False):
                before = snapshot(home / 'state', hook_path.parent)
                state_flags = ('--state-dir', home / 'state') if cached else ()
                explained = json.loads(run(binary, env, 'explain', '--harness', provider, '--event', 'PreToolUse',
                                           '--payload', payload_file, '--facts', facts_file, '--index', index_path,
                                           *state_flags, '--json').stdout)
                assert snapshot(home / 'state', hook_path.parent) == before
                actual = invoke()
                assert actual.returncode == 0 and 'level=ERROR' not in actual.stderr, actual.stderr
                assert actual.stdout == explained['wire']['stdout'], (route, actual.stdout, explained['wire'])
                assert expected in actual.stdout, actual.stdout
            parity('Effort=high/raw=future')
            state_files = list((home / 'state').rglob('session.json'))
            assert len(state_files) == 1
            state = json.loads(state_files[0].read_text())
            assert state['effort_mapping_source'] == 'user' and state['effort_approx'] is True
            record(cases, route, 'configured-hook-user-approx-raw-readonly-explain-parity')
            before = snapshot(base, codex, claude, project)
            rejected = run(old, dict(env, STYLE_HOME=str(home)), 'agenthooks', 'run', '--provider=' + provider, payload=payload)
            assert 'Effort=' not in rejected.stdout
            assert 'index-invalid' in rejected.stderr or 'index is unavailable' in rejected.stderr
            assert snapshot(base, codex, claude, project) == before
            for command in (('sync',), ('update', '--all', '--yes'), ('trust', 'effort-policy', '--yes'),
                            ('install', '--frozen', '--yes'), ('install', source, *selection, '--yes'),
                            ('uninstall', '--all', '--yes')):
                run(old, dict(env, STYLE_HOME=str(home)), *command, want=1)
                assert snapshot(base, codex, claude, project) == before
            record(cases, route, 'actual-T43-runtime-six-management-commands-refuse-zero-write')
            mapping_file.write_text('invalid live mapping')
            (home / 'build/index.json').unlink()
            (home / 'build/explain.json').unlink()
            run(binary, env, 'restore', *scope, '--yes', '--json')
            assert json.loads(index_path.read_text()) == index and (home / 'style.lock.yaml').read_text() == lock
            record(cases, route, 'frozen-rebuild-uses-approved-lock-not-live-mapping')
            config('low')
            doctor = json.loads(run(binary, env, 'doctor', *scope, '--json').stdout)
            assert any(d['code'] == 'effort-mapping-pending' for d in doctor['diagnostics'])
            run(binary, env, 'sync', *scope, '--json')
            parity('Effort=low/raw=future', cached=True)
            record(cases, route, 'doctor-pending-sync-remaps-cached-raw')
            mapping_file.unlink()
            run(binary, env, 'sync', *scope, '--json')
            assert json.loads(index_path.read_text())['format'] == 1
            assert 'min_index_format' not in (home / 'style.lock.yaml').read_text()
            parity('Effort=missing/raw=future', cached=True)
            state = json.loads(state_files[0].read_text())
            assert 'effort_mapping_sha256' not in state and 'effort_mapping_source' not in state
            assert 'effort' not in state and 'effort_approx' not in state and state['effort_raw'] == 'future'
            run(old, dict(env, STYLE_HOME=str(home)), 'sync', *scope, '--json')
            compatible = run(old, dict(env, STYLE_HOME=str(home)), 'agenthooks', 'run',
                             '--provider=' + provider, payload=payload)
            assert 'Effort=missing/raw=future' in compatible.stdout and 'level=ERROR' not in compatible.stderr
            record(cases, route, 'removal-clears-tier-approx-provenance-and-old-runtime-management-compatible')
            run(binary, env, 'uninstall', '--all', *scope, '--yes', '--json')
            record(cases, route, 'uninstall')
    args.report.write_text(json.dumps(dict(format=1, binary_sha256=sha(binary), old_binary_sha256=sha(old),
                                          real_harness=False, cases=cases), indent=2) + '\n')
    print(json.dumps(dict(checks=len(cases), passed=True)))


if __name__ == '__main__':
    main()

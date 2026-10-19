#!/usr/bin/env python3
"""M3-A11 built CLI and genuine old-runner refusal; isolated offline fixtures."""
import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path
import tempfile
from check_m3_update import fixture, run, snapshot, record, sha
from check_m3_refresh import handler
from check_m3_project import invoke_hook

MANIFEST = """spec: style/v0.3
name: service-provider
version: 1.0.0
description: service selector fixture
rules:
  - id: r
    target: event
    event: tool.pre
    text: 'Service={{provider | default: "missing"}}'
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
        with tempfile.TemporaryDirectory(prefix='style-m3-provider-') as temp:
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
            preview = json.loads(run(binary, env, 'install', source, *scope, *selection, '--json', want=1).stdout)
            assert preview['awaiting_confirmation'] and snapshot(base, codex, claude, project) == before
            record(cases, route, 'preview-zero-write')
            installed = json.loads(run(binary, env, 'install', source, *scope, *selection, '--json', confirm=True).stdout)
            home = Path(installed['scope']['home']) if scope else base
            index_path = home / 'build/index.json'
            index = json.loads(index_path.read_text())
            assert index['format'] == 4
            if route == 'codex-plugin':
                hook_path = home / 'adapters/codex/plugin/hooks/hooks.json'
                env['PLUGIN_ROOT'] = str(hook_path.parent.parent)
            elif scope:
                hook_path = project / ('.codex/hooks.json' if provider == 'codex' else '.claude/settings.json')
            else:
                hook_path = codex / 'hooks.json' if provider == 'codex' else claude / 'settings.json'
            record(cases, route, 'dynamic-index4')
            modes = [('launch', {}, 'local.ID'), ('invalid-launch', {}, '')] if provider == 'codex' else [
                ('default', {}, 'anthropic'),
                ('bedrock', {'CLAUDE_CODE_USE_BEDROCK': '1'}, 'bedrock'),
                ('vertex', {'CLAUDE_CODE_USE_VERTEX': '1'}, 'vertex'),
                ('foundry', {'CLAUDE_CODE_USE_FOUNDRY': '1'}, 'foundry'),
                ('conflict', {'CLAUDE_CODE_USE_BEDROCK': '1', 'CLAUDE_CODE_USE_VERTEX': '1'}, ''),
                ('endpoint', {'ANTHROPIC_BASE_URL': 'https://PRIVATE_ENDPOINT_CANARY.invalid'}, '')]
            # The Go fixture controls argv without interpreter rewriting.
            # It is a hook launcher, never a real Codex harness invocation.
            def invoke_selected(hook, selected_env, payload, service):
                if provider != 'codex':
                    return invoke_hook(hook, selected_env, project, payload)
                return subprocess.run([str(args.codex_ancestor.resolve()), '-c', 'model_provider=' + (service or 'unknown')],
                                      env=selected_env,
                                      input=json.dumps(dict(hook=hook, cwd=str(project), payload=payload)),
                                      text=True, capture_output=True, timeout=20)
            for mode, selectors, service in modes:
                hook_env = dict(env, **selectors)
                payload = json.dumps(dict(session_id='provider-' + mode, hook_event_name='PreToolUse',
                                          tool_name='Bash', tool_input={'command': 'true'}, cwd=str(project)))
                payload_file = root / 'payload.json'
                payload_file.write_text(payload)
                facts_file = root / 'facts.json'
                facts_file.write_text(json.dumps(dict(format=1, harness=provider,
                                                       payload_sha256='sha256:' + hashlib.sha256(payload.encode()).hexdigest(),
                                                       model_provider=service)))
                before = snapshot(home / 'state', hook_path.parent)
                explained = json.loads(run(binary, hook_env, 'explain', '--harness', provider, '--event', 'PreToolUse',
                                           '--payload', payload_file, '--facts', facts_file, '--index', index_path, '--json').stdout)
                assert snapshot(home / 'state', hook_path.parent) == before
                actual = invoke_selected(handler(hook_path, 'PreToolUse'), hook_env, payload, service)
                assert actual.returncode == 0 and 'level=ERROR' not in actual.stderr, actual.stderr
                assert actual.stdout == explained['wire']['stdout'], (route, mode, actual.stdout, explained['wire'])
                context = json.loads(actual.stdout)['hookSpecificOutput']['additionalContext']
                assert context == 'Service=' + (service or 'missing'), context
                again = invoke_selected(handler(hook_path, 'PreToolUse'), hook_env, payload, service)
                assert 'Service=' not in again.stdout
            record(cases, route, 'configured-hook-service-selector-default-conflict-readonly-explain-dedup', modes=len(modes))
            for path in (home / 'state').rglob('*'):
                if path.is_file():
                    assert b'PRIVATE_ENDPOINT_CANARY' not in path.read_bytes()
            record(cases, route, 'state-stats-no-endpoint')
            before = snapshot(base, codex, claude, project)
            rejected = run(old, dict(env, STYLE_HOME=str(home)), 'agenthooks', 'run', '--provider=' + provider, payload=payload)
            assert 'Service=' not in rejected.stdout
            assert 'index-invalid' in rejected.stderr or 'index is unavailable' in rejected.stderr
            assert snapshot(base, codex, claude, project) == before
            for command in (('sync',), ('update', '--all', '--yes'), ('trust', 'service-provider', '--yes'), ('install', '--frozen', '--yes')):
                run(old, dict(env, STYLE_HOME=str(home)), *command, want=1)
                assert snapshot(base, codex, claude, project) == before
            record(cases, route, 'actual-old-runtime-and-management-refuse-zero-write')
            for command in (('sync',), ('update', '--all'), ('restore',)):
                run(binary, env, *command, *scope, '--json', confirm=True)
            doctor = json.loads(run(binary, env, 'doctor', *scope, '--json').stdout)
            assert any(d['code'] == 'runtime-index-4' for d in doctor['diagnostics'])
            (home / 'build/index.json').unlink()
            (home / 'build/explain.json').unlink()
            run(binary, env, 'restore', *scope, '--json', confirm=True)
            assert json.loads(index_path.read_text()) == index
            record(cases, route, 'new-management-frozen-rebuild-and-doctor')
            run(binary, env, 'uninstall', '--all', *scope, '--json', confirm=True)
            record(cases, route, 'uninstall')
    args.report.write_text(json.dumps(dict(format=1, binary_sha256=sha(binary),
                                          old_binary_sha256=sha(old), real_harness=False, cases=cases), indent=2) + '\n')
    print(json.dumps(dict(checks=len(cases), passed=True)))


if __name__ == '__main__':
    main()

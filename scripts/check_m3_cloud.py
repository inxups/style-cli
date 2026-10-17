#!/usr/bin/env python3
"""M3-A7 offline built CLI; direct hooks and version stubs are not cloud delivery."""
import argparse
import json
import os
from pathlib import Path
import shutil
import tempfile
from check_m3_update import fixture, manifest, run, snapshot, record, sha
from check_m3_project import invoke_hook


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    cases = []
    with tempfile.TemporaryDirectory(prefix='style-m3-cloud-') as temporary:
        root = Path(temporary).resolve()
        env, base, codex, claude, stub = fixture(root)
        (stub / 'style').symlink_to(binary)
        project = root / "project ' space"
        project.mkdir()
        source = root / 'source'
        source.mkdir()
        (source / 'STYLE.yaml').write_text(manifest('portable', 'Cloud.', native=True) +
            "  - id: missing-dependency\n    target: native\n    claude-code: {event: PreToolUse, matcher: '*', command: 'style-fixture-absent-command'}\n")
        installed = json.loads(run(binary, env, 'install', source, '--project', project,
                                   '--harness=claude-code', '--yes', '--json').stdout)
        home = Path(installed['scope']['home'])
        before = snapshot(base, project, codex, claude)
        preview = json.loads(run(binary, env, 'export', '--target=claude-cloud',
                                 '--project', project, '--json', want=1).stdout)
        assert preview['awaiting_confirmation'] and preview['native_hooks']
        assert snapshot(base, project, codex, claude) == before
        record(cases, 'claude-cloud', 'full-preview-zero-write')
        exported = json.loads(run(binary, env, 'export', '--target=claude-cloud',
                                  '--project', project, '--yes', '--json').stdout)
        assert exported['changed']
        assert len(exported['manifest']['files']) == 6
        assert str(root) not in json.dumps(exported['manifest'])
        for name in ('index.json', 'explain.json'):
            assert str(root) not in (project / '.style/cloud' / name).read_text()
        again = json.loads(run(binary, env, 'export', '--target=claude-cloud',
                               '--project', project, '--yes', '--json').stdout)
        assert not again['changed']
        record(cases, 'claude-cloud', 'portable-deterministic-bundle')
        clone = root / 'fresh clone'
        shutil.copytree(project, clone)
        remote_home = root / 'remote-home'
        remote_home.mkdir(mode=0o700)
        remote = dict(env, HOME=str(remote_home), STYLE_HOME=str(root / 'no-install'),
                      CLAUDE_CONFIG_DIR=str(root / 'no-claude'), CLAUDE_CODE_REMOTE='true')
        groups = json.loads((clone / '.claude/settings.json').read_text())['hooks']['PreToolUse']
        hooks = [h for group in groups for h in group['hooks']]
        dynamic = next(h for h in hooks if 'agenthooks' in str(h))
        native = next(h for h in hooks if h['command'].startswith(': --owner=style'))
        payload = json.dumps(dict(hook_event_name='PreToolUse', session_id='cloud-fixture',
                                  cwd='/forged', tool_name='Bash', tool_input={'command': 'true'}))
        before = snapshot(project, clone, base, codex, claude)
        result = invoke_hook(dynamic, remote, clone, payload)
        assert result.returncode == 0 and 'Cloud.' in result.stdout, (result.stdout, result.stderr)
        assert snapshot(project, clone, base, codex, claude) == before
        assert list((remote_home / '.style/cloud-state').glob('*/state/*/session.json'))
        record(cases, 'claude-cloud', 'fresh-clone-no-private-install-runtime', real_cloud=False)
        result = invoke_hook(native, remote, clone, payload)
        assert result.returncode == 0 and result.stdout == 'NATIVE-Cloud.', result
        assert 'Static Cloud.' in (clone / '.claude/rules/style-portable.md').read_text()
        record(cases, 'claude-cloud', 'portable-native-command-and-static-file', real_delivery=False)
        # Direct native commands retain the native process failure contract.
        missing = next(h for h in hooks if 'style-fixture-absent-command' in h['command'])
        result = invoke_hook(missing, remote, clone, payload)
        assert result.returncode == 127
        record(cases, 'claude-cloud', 'missing-native-dependency-fails')
        original = (clone / '.style/cloud/index.json').read_bytes()
        (clone / '.style/cloud/index.json').write_text('{}')
        before = snapshot(clone, base, remote_home)
        result = invoke_hook(dynamic, remote, clone, payload)
        assert result.returncode == 0 and 'Cloud.' not in result.stdout and 'digest' in result.stderr
        assert snapshot(clone, base, remote_home) == before
        (clone / '.style/cloud/index.json').write_bytes(original)
        local = dict(remote, CLAUDE_CODE_REMOTE='false')
        result = invoke_hook(dynamic, local, clone, payload)
        assert result.returncode == 0 and 'no local binding' in result.stderr
        record(cases, 'claude-cloud', 'tamper-rejected-and-local-binding-required')
        missing_runner = dict(remote, PATH=os.defpath)
        result = invoke_hook(dynamic, missing_runner, clone, payload)
        assert result.returncode == 127 and 'Cloud.' not in result.stdout
        record(cases, 'claude-cloud', 'missing-runner-native-process-failure')
        before = snapshot(base, project)
        result = json.loads(run(binary, env, 'uninstall', '--all', '--project', project,
                                '--json', want=1).stdout)
        assert result['awaiting_confirmation'] and len(result['cloud_files']) == 3
        assert snapshot(base, project) == before
        run(binary, env, 'uninstall', '--all', '--project', project, '--yes', '--json')
        assert not (project / '.style/cloud/manifest.json').exists()
        assert not (home / 'cloud-export.json').exists()
        record(cases, 'claude-cloud', 'last-uninstall-preview-and-bundle-removal')
    args.report.write_text(json.dumps(dict(format=1, binary_sha256=sha(binary),
                                          real_cloud=False, cases=cases), indent=2) + '\n')
    print(f'{len(cases)} cloud CLI checks passed')


if __name__ == '__main__':
    main()

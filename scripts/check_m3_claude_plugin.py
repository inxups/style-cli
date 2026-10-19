#!/usr/bin/env python3
"""M3-A8 built CLI and genuine old-runner gate; native registration metadata is a fixture."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
from check_m3_update import fixture, manifest, run, snapshot, record, sha
from style_json import report as style_report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--old-binary', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    binary, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    with tempfile.TemporaryDirectory(prefix='style-m3-claude-plugin-') as temporary:
        root = Path(temporary).resolve()
        env, base, codex, claude, stub = fixture(root)
        (stub / 'style').symlink_to(binary)
        source = root / 'source'
        source.mkdir()
        (source / 'STYLE.yaml').write_text(manifest('plugin-rules', 'Plugin.', native=True))
        selection = ('--harness=claude-code', '--route=claude-code=claude-plugin')
        before = snapshot(base, codex, claude)
        preview = style_report(run(binary, env, 'install', source, *selection, '--json', want=1).stdout)
        assert preview['registration'][0]['state'] == 'registration-required'
        assert snapshot(base, codex, claude) == before
        record(cases, 'claude-plugin', 'preview-registration-no-write')
        installed = style_report(run(binary, env, 'install', source, *selection, '--json', want=1, confirm=True).stdout)
        registration = installed['registration'][0]
        market = Path(registration['marketplace'])
        plugin = market / 'plugins/style-runtime'
        name = 'style-' + hashlib.sha256(str(base.resolve()).encode()).hexdigest()
        assert registration['plugin'] == 'style-runtime@' + name
        assert json.loads((market / '.claude-plugin/marketplace.json').read_text())['plugins'][0]['source'] == './plugins/style-runtime'
        assert not (plugin / 'bin').exists()
        assert (plugin / 'scripts/style').stat().st_mode & 0o111
        assert 'format: 3' in (base / 'style.lock.yaml').read_text()
        record(cases, 'claude-plugin', 'generated-private-marketplace-and-format3')
        before = snapshot(base, claude)
        for command in (('sync',), ('update', '--all', '--yes'), ('install', '--frozen', '--yes'),
                        ('trust', 'plugin-rules', '--yes'), ('uninstall', '--all', '--yes')):
            result = run(old, env, *command, want=1)
            assert any(x in result.stdout + result.stderr for x in ('lock format', 'project binding', 'project lock', 'frozen-lock-invalid', 'frozen-metadata-incomplete', 'lock-invalid')), result
            assert snapshot(base, claude) == before
        record(cases, 'claude-plugin', 'genuine-old-binary-refuses-user-format3')
        # The real native CLI performs these operations in C4. The offline
        # fixture verifies conservative readers without changing live settings.
        metadata = claude / 'plugins'
        metadata.mkdir(exist_ok=True)
        (metadata / 'known_marketplaces.json').write_text(json.dumps({name: {
            'source': {'source': 'directory', 'path': str(market)}, 'installLocation': str(market)}}))
        (metadata / 'installed_plugins.json').write_text(json.dumps({'version': 2, 'plugins': {
            registration['plugin']: [{'scope': 'user', 'version': '0.1.0',
                                      'installPath': str(metadata / 'cache' / name / 'style-runtime/0.1.0')}]}}))
        settings_path = claude / 'settings.json'
        settings = json.loads(settings_path.read_text()) if settings_path.exists() else {}
        settings['enabledPlugins'] = {registration['plugin']: True}
        settings_path.write_text(json.dumps(settings))
        repeated = style_report(run(binary, env, 'install', source, *selection, '--json', confirm=True).stdout)
        assert not repeated['changed'] and repeated['registration'][0]['state'] == 'enabled'
        record(cases, 'claude-plugin', 'registered-enabled-observation-not-delivery')
        raw = json.loads((plugin / 'hooks/hooks.json').read_text())
        hooks = [h for g in raw['hooks']['PreToolUse'] for h in g['hooks']]
        dynamic = next(h for h in hooks if 'agenthooks' in str(h))
        command = [dynamic['command'], *[arg.replace('${CLAUDE_PLUGIN_ROOT}', str(plugin)) for arg in dynamic['args']]]
        payload = json.dumps(dict(hook_event_name='PreToolUse', session_id='fixture',
                                  tool_name='Bash', tool_input={'command': 'true'}))
        result = subprocess.run(command, env=dict(env, CLAUDE_PLUGIN_ROOT=str(plugin)), input=payload,
                                text=True, capture_output=True, timeout=15)
        assert result.returncode == 0 and 'Plugin.' in result.stdout, (result.stdout, result.stderr)
        assert 'Static Plugin.' in (claude / 'rules/style-plugin-rules.md').read_text()
        record(cases, 'claude-plugin', 'configured-launcher-ordinary-wire-and-external-static')
        before = snapshot(base, claude)
        run(binary, env, 'uninstall', '--all', want=1, confirm=True)
        run(binary, env, 'install', source, '--harness=claude-code', '--route=claude-code=claude-user', want=1, confirm=True)
        assert snapshot(base, claude) == before
        record(cases, 'claude-plugin', 'active-plugin-removal-and-switch-blocked')
        settings['enabledPlugins'][registration['plugin']] = False
        settings_path.write_text(json.dumps(settings))
        native_before = snapshot(metadata)
        run(binary, env, 'install', source, '--harness=claude-code', '--route=claude-code=claude-user', '--json', confirm=True)
        assert snapshot(metadata) == native_before
        assert not (plugin / 'scripts/style').exists()
        assert '--owner=style' in settings_path.read_text()
        assert 'Static Plugin.' in (claude / 'rules/style-plugin-rules.md').read_text()
        record(cases, 'claude-plugin', 'disabled-plugin-switch-keeps-native-db-and-static')
        run(binary, env, 'uninstall', '--all', confirm=True)
        assert snapshot(metadata) == native_before
        assert not (claude / 'rules/style-plugin-rules.md').exists()
        assert '--owner=style' not in settings_path.read_text()
        record(cases, 'claude-plugin', 'uninstall-removes-effects-retains-native-metadata')
    args.report.write_text(json.dumps(dict(format=1, binary_sha256=sha(binary), old_binary_sha256=sha(old),
                                          real_harness=False, cases=cases), indent=2) + '\n')
    print(f'{len(cases)} Claude plugin CLI checks passed')


if __name__ == '__main__':
    main()

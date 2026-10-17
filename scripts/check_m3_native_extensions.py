#!/usr/bin/env python3
"""M3-A16: four-field native compatibility and foreign extension preservation.

Harness commands are version-only fixtures. No native author command runs.
"""
import argparse
import json
from pathlib import Path
import shutil
import tempfile
from check_m3_update import fixture, manifest, record, run, sha, snapshot


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--old-binary', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    binary, old = args.binary.resolve(), args.old_binary.resolve()
    cases = []
    for route in ('codex-user','codex-plugin','claude-user','codex-project','claude-project'):
        provider = 'claude-code' if route.startswith('claude-') else 'codex'
        with tempfile.TemporaryDirectory(prefix='style-m3-native-boundary-') as temporary:
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
            source_file.write_text(manifest('native-compat', 'Native baseline.', native=True))
            scope = ('--project',project) if route.endswith('-project') else ()
            selected = ('--harness='+provider,'--route='+provider+'='+route)
            baseline = run(runner,env,'plan',source,'--harness='+provider,'--json')
            installed = json.loads(run(runner,env,'install',source,*scope,*selected,'--yes','--json').stdout)
            home = Path(installed['scope']['home']) if scope else base
            if route == 'codex-plugin':
                hooks = home / 'adapters/codex/plugin/hooks/hooks.json'
            elif scope:
                hooks = project / ('.codex/hooks.json' if provider=='codex' else '.claude/settings.json')
            else:
                hooks = codex / 'hooks.json' if provider=='codex' else claude / 'settings.json'
            foreign = [dict(type='http',url='https://example.invalid/foreign',headers={'X-Fixture':'unchanged'}),
                       dict(type='prompt',prompt='Foreign prompt must remain'),
                       dict(type='agent',prompt='Foreign agent must remain'),
                       dict(type='mcp_tool',server='foreign',tool='inspect',input={'fixture':'unchanged'}),
                       dict(type='command',command='printf FOREIGN',asyncRewake=True)] if provider=='claude-code' else [{'type':'command','command':'printf FOREIGN','async':False,'timeout':9}]
            config = json.loads(hooks.read_text())
            config['hooks'].setdefault('Stop',[]).append({'hooks':foreign})
            hooks.write_text(json.dumps(config,indent=2)+'\n')
            run(runner,env,'sync',*scope,'--json')
            before = snapshot(home,codex,claude,project)
            shutil.copyfile(binary,runner)
            actual = run(runner,env,'plan',source,'--harness='+provider,'--json')
            assert (baseline.stdout,baseline.stderr)==(actual.stdout,actual.stderr)
            record(cases,route,'four-field-plan-json-byte-compatible')
            synced = json.loads(run(runner,env,'sync',*scope,'--json').stdout)
            assert not synced['changed'] and snapshot(home,codex,claude,project)==before
            assert any(group['hooks']==foreign for group in json.loads(hooks.read_text())['hooks']['Stop'])
            record(cases,route,'old-install-current-sync-config-bytes-and-metadata-unchanged')
            assert 'native-compat/executable' not in run(runner,env,'stats',*scope,'--json').stdout
            record(cases,route,'native-excluded-from-runtime-stats')
            # Unknown extension values do not reach an execution or disclosure path.
            candidate = root / 'candidate'
            candidate.mkdir()
            for extension in ('async: true','onFailure: block','type: http','headers: {Authorization: PRIVATE_VALUE}'):
                (candidate/'STYLE.yaml').write_text('spec: style/v0.3\nname: candidate\nversion: 1.0.0\ndescription: rejection fixture\nrules:\n'
                    f"  - {{id: r, target: native, {provider}: {{event: Stop, command: 'touch {root / 'NEVER'}', {extension}}}}}\n")
                before = snapshot(home,codex,claude,project,candidate)
                result = run(runner,env,'install',candidate,*scope,*selected,'--yes','--json',want=1)
                assert 'not supported;' in result.stdout+result.stderr and 'PRIVATE_VALUE' not in result.stdout+result.stderr
                assert snapshot(home,codex,claude,project,candidate)==before and not (root/'NEVER').exists()
                record(cases,route,'extension-rejection-zero-write',field=extension.split(':',1)[0])
            run(runner,env,'uninstall','--all',*scope,'--yes','--json')
            remaining = json.loads(hooks.read_text())['hooks']['Stop']
            assert any(group['hooks']==foreign for group in remaining)
            assert '--owner=style' not in hooks.read_text()
            record(cases,route,'uninstall-preserves-foreign-types-and-fields')
    args.report.write_text(json.dumps(dict(format=1,acceptance='M3-A16',binary_sha256=sha(binary),
        old_binary_sha256=sha(old),real_harness=False,cases=cases),indent=2)+'\n')
    print(json.dumps(dict(passed=len(cases))))


if __name__ == '__main__':
    main()

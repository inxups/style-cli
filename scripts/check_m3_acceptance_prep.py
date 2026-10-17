#!/usr/bin/env python3
"""M3-P1 offline six-route preparation and exact private config recovery."""
import argparse
import json
from pathlib import Path
import sys
import tempfile

from check_m3_update import fixture, run, sha
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'testdata/manual'))
from m3_live_fixture import package
from m3_probe_support import PrivateBaseline, SourceBinding


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',required=True,type=Path);parser.add_argument('--report',required=True,type=Path)
    args=parser.parse_args();binary=args.binary.resolve();cases=[]
    for route in ['codex-user','codex-plugin','codex-project','claude-user','claude-plugin','claude-project']:
        with tempfile.TemporaryDirectory(prefix='style-m3-prepare-') as temp:
            root=Path(temp).resolve();env,base,codex,claude,stub=fixture(root);(stub/'style').symlink_to(binary)
            harness='codex' if route.startswith('codex') else 'claude-code';source=package(root,harness)
            project=root/'project';project.mkdir();scope=['--project',project] if route.endswith('-project') else []
            profiles=[PrivateBaseline(codex,['config.toml','hooks.json']),
                      PrivateBaseline(claude,['settings.json','CLAUDE.md','rules/style-m3-live.md']),
                      PrivateBaseline(project,['.codex/hooks.json','.codex/config.toml','.claude/settings.json','.claude/rules/style-m3-live.md','.style/project.json','.style/hooks/style'])]
            binding=SourceBinding({'style_binary':binary,'native_observer':Path(__file__).resolve().parents[1]/'testdata/manual/m3_native_observer.py'})
            binding.add_copy('native_observer_copy',root/'native-observer.py',binding.files['native_observer'])
            run(binary,env,'validate',source,'--json')
            expected=1 if route=='claude-plugin' else 0
            installed=json.loads(run(binary,env,'install',source,'--harness',harness,'--route',harness+'='+route,*scope,'--yes','--json',want=expected).stdout)
            assert installed['changed']
            home=Path(installed['scope']['home']) if scope else base
            (home/'effort.yaml').write_text('format: 1\nharnesses:\n  '+harness+': {high: medium, low: high}\n')
            run(binary,env,'sync',*scope,'--json')
            assert json.loads((home/'build/index.json').read_text())['format']==7
            run(binary,env,'uninstall','--all',*scope,'--yes','--json')
            assert not (home/'build/index.json').exists() and not (home/'style.lock.yaml').exists()
            restored=[baseline.verify() for baseline in profiles]
            binding.verify()
            cases.append({'route':route,'manifest_validated':True,'install_sync_uninstall':True,'restoration':restored,'native_observer_copy_bound':True,'real_harness':False})
    args.report.write_text(json.dumps({'format':1,'passed':True,'binary_sha256':sha(binary),'cases':cases},indent=2)+'\n')
    print(json.dumps({'passed':True,'routes':len(cases)}))


if __name__=='__main__':main()

#!/usr/bin/env python3
"""Run both native Windows modules, retaining all failures in CI artifacts."""
import json, os, subprocess, sys
from pathlib import Path

root=Path(__file__).resolve().parent.parent
out=Path(os.environ['RUNNER_TEMP'])/'style-windows-evidence';out.mkdir(exist_ok=True)
failed=False
for module,directory in [('root',root),('agenthooks',root/'agenthooks')]:
    with (out/(module+'.jsonl')).open('wb') as log:
        result=subprocess.run(['go','test','-count=1','-json','./...'],cwd=directory,stdout=log,stderr=subprocess.STDOUT)
    events=[]
    for line in (out/(module+'.jsonl')).read_text(encoding='utf-8',errors='replace').splitlines():
        try:events.append(json.loads(line))
        except ValueError:print(line)
    bad=[e for e in events if e.get('Action')=='fail' and e.get('Test')]
    for entry in bad:
        print(module,entry['Package'],entry['Test'])
        print(''.join(e.get('Output','') for e in events if e.get('Package')==entry['Package'] and e.get('Test')==entry['Test'])[-2500:])
    print(module,json.dumps({'exit':result.returncode,'passed':sum(e.get('Action')=='pass' and bool(e.get('Test')) for e in events),'failed':len(bad),'skipped':sum(e.get('Action')=='skip' for e in events)}))
    failed |= result.returncode!=0
sys.exit(int(failed))

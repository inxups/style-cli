"""Reproduce the M3 closing snapshot checks from a full repository checkout.

Usage: python3 docs/review/M3-FINAL-VERIFY-2026-10-18.py /tmp/m3-audit.json --closed
The script reads repository inputs and writes only the requested result file.
"""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import unicodedata

root=Path(__file__).resolve().parents[2]
def git(*args):return subprocess.check_output(['git',*args],cwd=root)
def digest(data):return hashlib.sha256(data).hexdigest()
def declared(file):
    rows=re.findall(r'^- \[([x ~!?])\] \*\*(M3-[A-Z0-9-]+)',file.read_text(),re.M)
    assert len(rows)==len({name for _,name in rows}),('duplicate IDs',file)
    return {name:state for state,name in rows}
tasks=declared(root/'docs/STYLE-M3-TASKS.md');tests=declared(root/'docs/STYLE-M3-TESTS.md')
assert len(tasks)==44 and len(tests)==39
covered=set()
for line in (root/'docs/STYLE-M3-TASKS.md').read_text().splitlines():
    if not re.match(r'^- \[[x ~!?]\] \*\*M3-',line):continue
    name=re.search(r'\*\*(M3-[A-Z0-9-]+)',line)[1]
    reference=re.search(r'测试：([^。]+)',line);assert reference,(name,'missing tests')
    text=reference[1]
    text=re.sub(r'M3-([A-Z]+)(\d+)–([A-Z]+)(\d+)',lambda m:'、'.join('M3-'+m[1]+str(i) for i in range(int(m[2]),int(m[4])+1)) if m[1]==m[3] else m[0],text)
    ids=set(re.findall(r'M3-[A-Z0-9-]+',text));assert ids and ids<=tests.keys(),(name,ids-tests.keys())
    covered.update(ids)
assert covered==tests.keys(),('unreferenced tests',tests.keys()-covered)
assert tasks['M3-REAL-02']=='~' and tasks['M3-REAL-03']=='~'
assert tasks['M3-REAL-04']==tests['M3-C3']==tests['M3-W1']==tests['M3-CARRY-01']=='?'
assert tests['M3-TOKEN-01']=='~'
if '--closed' in sys.argv or '--final' in sys.argv:
    pending=set() if '--closed' in sys.argv else {'M3-DOCS-01'}
    assert {k for k,v in tasks.items() if v!='x'}=={'M3-CARRY-01','M3-REAL-02','M3-REAL-03','M3-REAL-04'}|pending
    assert {k for k,v in tests.items() if v!='x'}=={'M3-CARRY-01','M3-C3','M3-W1','M3-TOKEN-01'}|pending
baseline='b7af4e75d0967ed5f7e8728f86f80bc7a930114c'
originals=git('ls-tree','-r','--name-only',baseline).decode().splitlines()
archives=[p for p in originals if re.match(r'docs/(?:STYLE-M[12]B?(?:-|\.)|review/M[12]B?[-.])',p)]
goldens=[p for p in originals if p.startswith(('testdata/','agenthooks/')) and ('golden' in p or p.endswith('.golden'))]
for path in archives+goldens:
    assert (root/path).read_bytes()==git('show',baseline+':'+path),('modified archive/golden',path)
oldprogress=git('show',baseline+':docs/PROGRESS.md');assert (root/'docs/PROGRESS.md').read_bytes().startswith(oldprogress)
changed=set(git('diff','--name-only',baseline).decode().splitlines())
changed.update(git('ls-files','--others','--exclude-standard').decode().splitlines())
documents=[root/p for p in sorted(changed) if p.endswith('.md')]
def anchors(path):
    text=path.read_text();result=set(re.findall(r'<a\s+(?:name|id)=[\'"]([^\'"]+)',text));used={}
    for title in re.findall(r'^#{1,6}\s+(.+?)\s*#*\s*$',text,re.M):
        title=re.sub(r'\[([^]]+)\]\([^)]*\)',r'\1',title)
        title=re.sub(r'<[^>]+>','',title).strip().lower()
        slug=''.join(c for c in title if c.isalnum() or c in '_- ' or unicodedata.category(c).startswith('M')).replace(' ','-')
        duplicate=used.get(slug,0);result.add(slug+('-'+str(duplicate) if duplicate else ''));used[slug]=duplicate+1
    return result
links=0;fences=0
for path in documents:
    text=path.read_text();opened=None
    for line in text.splitlines():
        match=re.match(r'^ {0,3}(`{3,}|~{3,})(.*)$',line)
        if not match:continue
        marker,rest=match.groups()
        if opened:
            if marker[0]==opened[0] and len(marker)>=len(opened) and not rest.strip():opened=None
        else:opened=marker;fences+=1
    assert opened is None,(path,'unclosed fence')
    for target in re.findall(r'\]\(([^)]+)\)',text):
        if '://' in target:continue
        name,_,anchor=target.partition('#');destination=path.parent/name if name else path
        assert destination.exists(),(path,target)
        if anchor and destination.suffix=='.md':assert anchor in anchors(destination),(path,target)
        links+=1
accepted='2c24e5f9f3bbad6a3af2586519b6d0e421179dbf'
history=[p for p in git('ls-tree','-r','--name-only',accepted,'--','docs/review').decode().splitlines() if p.endswith('.json')]
for path in history:
    commit=baseline if path in originals else accepted
    assert (root/path).read_bytes()==git('show',commit+':'+path),('rewritten historical report',path)
production_changes=git('diff','--name-only','5da3b4516222b05c616323433715b449a1a88803').decode().splitlines()
assert all(p.startswith(('docs/','testdata/manual/')) for p in production_changes),production_changes
binding=json.loads((root/'docs/review/M3-B-CODEX-PLUGIN-2026-10-18.json').read_bytes())['binding']
inputs=[p for p in binding if (root/p).is_file() and not p.startswith('testdata/manual/') and p not in ('style_binary','harness_binary','native_observer_copy')]
for p in inputs:assert 'sha256:'+digest((root/p).read_bytes())==binding[p],('binary source drift',p)
subprocess.run(['git','diff','--check'],cwd=root,check=True)
report={'format':1,'passed':True,'closed_scope_checked':'--closed' in sys.argv,'task_ids':len(tasks),'test_ids':len(tests),'all_tests_have_task_mapping':True,'archives_unchanged':len(archives),'historical_golden_unchanged':len(goldens),'historical_reports_unchanged':len(history),'report_protection':'pre-M3 reports against closed milestone baseline; M3 reports against accepted final source commit '+accepted,'progress_append_only_baseline':baseline,'progress_prefix_sha256':digest(oldprogress),'release_source_binding_files_verified':len(inputs),'production_source_commit':'5da3b4516222b05c616323433715b449a1a88803','ci_source_commit':accepted,'deferred_still_unpassed':['C3','W1/REAL-04','Anthropic independent API'],'official_calibration':'each 0/40; nonblocking'}
report.update(documents=len(documents),local_links_and_anchors=links,fence_blocks=fences)
destination=Path(sys.argv[1]);destination.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))

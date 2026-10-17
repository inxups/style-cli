"""Shared, explicit M3 real-model fixture. No executable hook wraps STYLE."""
import json
from pathlib import Path
import shlex
import sys
from m3_probe_support import private_write

REFRESH = 'M3_REFRESH_2E841C. Keep responses concise.'
STATIC = 'M3_STATIC_192C5A. Use concrete statements.'
NATIVE_START = 'M3_NATIVE_START_42D671. Native session observer.'
NATIVE_UPDATED = 'M3_NATIVE_UPDATED_91B326. Updated native session observer.'
NATIVE_TOOL = 'M3_NATIVE_TOOL_612B83. Native Bash observer.'
UNMATCHED = 'M3_NATIVE_UNMATCHED_833E2A.'
GUARD = 'M3_GUARD_6218F2. Check the requested shell command.'
UNKNOWN = 'M3_UNKNOWN_EFFORT_812E49.'


def package(root, harness):
    root=Path(root); source=root/'source'; observer=root/'native-observer.py'
    private_write(observer,Path(__file__).with_name('m3_native_observer.py').read_bytes())
    native=[]
    for name,event,matcher,marker in [('start','SessionStart',None,NATIVE_START),('prompt','UserPromptSubmit',None,''),('tool','PreToolUse','^Bash$',NATIVE_TOOL),('unmatched','PreToolUse','^Read$',UNMATCHED)]:
        hook={'event':event,'command':shlex.join([sys.executable,'-I',str(observer),str(root/'native-events'),marker]),'timeout':10}
        if matcher:hook['matcher']=matcher
        native.append({'id':'native-'+name,'target':'native',harness:hook})
    rules=[{'id':'static','target':'system','text':STATIC},
           {'id':'refresh','target':'system','repeat':'session','refresh':2,'reason':'Real M3 refresh acceptance','text':REFRESH},
           {'id':'facts','target':'event','event':'session.start','repeat':'session','text':'M3_FACTS provider={{provider | default: "unknown"}} effort={{thinking_effort | default: "unknown"}} raw={{thinking_effort_raw | default: "unknown"}}'},
           {'id':'unknown','target':'system','when':{'thinking_effort':'unknown'},'text':UNKNOWN},
           {'id':'fence','target':'event','event':'prompt.submitted','repeat':'every','text':'M3_EVENT {{event.prompt}}'},
           {'id':'guard','target':'tool','tool':'shell','at':'before','repeat':'every','fail':'closed','text':GUARD},*native]
    document={'spec':'style/v0.3','name':'m3-live','version':'1.0.0','description':'Isolated M3 native acceptance',
              'reads':{'model':True,'provider':True,'thinking_effort':True,'event':['prompt']},'rules':rules}
    private_write(source/'STYLE.yaml',json.dumps(document,ensure_ascii=False,indent=2)+'\n')
    return source


def native_payload(root, session, event):
    rows=[]
    for path in sorted((Path(root)/'native-events').glob('*.json')):
        data=json.loads(path.read_bytes())
        if data.get('session_id')==session and data.get('hook_event_name')==event:
            rows.append((path,data))
    if not rows:raise ValueError('independent native payload missing')
    return rows[0]


def outputs(call,rule):
    return [row for row in call['stats'] if row.get('entry','').split('@')[0]=='m3-live/'+rule and row.get('result')=='output']

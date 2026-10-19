#!/usr/bin/env python3
"""Native Windows CLI checks; fixture harnesses only answer --version."""
import json, os, shutil, subprocess, sys, tempfile
from pathlib import Path

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8', errors='backslashreplace')
    sys.stderr.reconfigure(encoding='utf-8', errors='backslashreplace')

ROOT = Path(__file__).resolve().parent.parent

def main():
    binary, helper = map(lambda s: Path(s).resolve(), sys.argv[1:3])
    if os.name != 'nt':
        raise SystemExit('requires native Windows; cross compilation does not count')
    checks = []
    def check(ok, label):
        checks.append({'check':label,'pass':bool(ok)})
        if not ok: raise AssertionError(label)
    with tempfile.TemporaryDirectory(prefix='style-win-') as raw:
        root=Path(raw); bindir=root/'bin 空格 %PATH%! & test';bindir.mkdir()
        runner=bindir/'style.exe';shutil.copy2(binary,runner)
        for name in ['codex.exe','claude.exe']:shutil.copy2(helper,bindir/name)
        env=os.environ.copy()
        for key in list(env):
            if key.startswith(('CODEX_','CLAUDE_','STYLE_','ANTHROPIC_','OPENAI_')):env.pop(key,None)
        user=root/'用户';user.mkdir();env.update(HOME=str(user),USERPROFILE=str(user),PATH=str(bindir)+os.pathsep+env['PATH'],STYLE_HOME=str(user/'.style'),CLAUDE_CONFIG_DIR=str(user/'.claude'))
        def run(args, data=None):
            return subprocess.run([str(runner),*args],input=data,text=True,encoding='utf-8',capture_output=True,env=env,cwd=user,timeout=60)
        baseline = {
            user/'.codex'/'config.toml': b'# preserved external config\nmodel = "fixture"\n',
            user/'.claude'/'settings.json': '{ "env": {"KEEP": "保留"} }\r\n'.encode('utf-8'),
        }
        for path, data in baseline.items():
            path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data)
        package=ROOT/'testdata'/'packages'/'terse-reviewer'
        result=run(['install',str(package),'--harness','codex,claude-code','--route','codex=codex-user','--route','claude-code=claude-user','--yes'])
        check(result.returncode==0,'install both user routes: '+result.stderr[-300:])
        for harness,config in [('codex',user/'.codex'/'hooks.json'),('claude-code',user/'.claude'/'settings.json')]:
            doc=json.loads(config.read_text(encoding='utf-8'))
            entry=doc['hooks']['PreToolUse'][0]['hooks'][0]
            payload=json.dumps({'session_id':'win-中文','cwd':str(user),'hook_event_name':'PreToolUse','tool_name':'Bash','tool_input':{'command':'git push --force origin main'}},ensure_ascii=False)
            if harness=='codex':
                argv=['cmd.exe','/d','/s','/c',entry['command']]
                check('/WindowsPowerShell/v1.0/powershell.exe -NoLogo' in entry['command'],'Codex encoded launcher')
            else:
                argv=[entry['command'],*entry['args']]
                check(argv[0].lower().endswith('.exe'),'Claude exec form')
            result=subprocess.run(argv,input=payload,encoding='utf-8',text=True,capture_output=True,env=env,cwd=user,timeout=60)
            check(result.returncode==0,'native hook exit '+harness+': '+result.stderr[-300:])
            output=json.loads(result.stdout)
            check(output.get('hookSpecificOutput',{}).get('permissionDecision')=='deny','native hook deny '+harness)
        check(run(['sync']).returncode==0,'sync both user routes')
        lock = user/'.style'/'style.lock.yaml'; before_lock=lock.read_bytes()
        installed = user/'.style'/'packages'/'terse-reviewer'/'STYLE.yaml'; before_package=installed.read_bytes()
        installed.write_bytes(b'corrupt managed package')
        frozen=run(['restore','--yes','--json'])
        check(frozen.returncode==0,'offline frozen restoration: '+frozen.stderr[-300:])
        check(installed.read_bytes()==before_package,'frozen restores exact locked package bytes')
        check(lock.read_bytes()==before_lock,'frozen preserves exact lock bytes')
        for route in ['codex-project','codex-plugin','claude-project','claude-plugin']:
            harness='codex' if route.startswith('codex-') else 'claude-code'
            result=run(['install',str(package),'--harness',harness,'--route',harness+'='+route,'--yes'])
            check(result.returncode!=0,'explicit unsupported Windows route '+route)
        check(lock.read_bytes()==before_lock,'unsupported routes preserve lock')
        for command in [['doctor','--json'],['list','--json'],['stats','--json']]:
            result=run(command)
            check(result.returncode==0,' '.join(command)+': '+result.stdout[-2000:]+result.stderr[-500:])
        for encoding in ['o200k_base','cl100k_base']:
            text=root/'输入.txt';text.write_text('Hello 世界\n',encoding='utf-8')
            result=run(['count',str(text),'--count=exact','--count-provider=openai','--encoding='+encoding,'--json'])
            check(result.returncode==0,'exact '+encoding+': '+result.stderr[-200:])
        check(run(['uninstall','terse-reviewer','--yes']).returncode==0,'uninstall')
        for path,data in baseline.items():
            check(path.read_bytes()==data,'external config exact restoration '+path.name)
        check(not (user/'.codex'/'hooks.json').exists(),'remove created Codex hooks after uninstall')
    report={'platform':sys.platform,'checks':checks,'passed':len(checks),'boundary':'Native Windows STYLE CLI with version-only harness stubs; not W1 real harness acceptance.'}
    if len(sys.argv)>3:Path(sys.argv[3]).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()

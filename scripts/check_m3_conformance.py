#!/usr/bin/env python3
"""M3-A18: old install/state upgrade and combined features on six routes.

Version stubs, controlled Codex ancestry and configured-hook calls are offline
fixtures. They never establish native harness trust or model delivery.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

from check_m3_update import fixture, record, run, sha, snapshot
from check_m3_refresh import handler
from check_m3_project import invoke_hook
from style_json import report as style_report


def manifest(name,combined):
    rules=[{'id':'static','target':'system','text':'A18 static compatibility.'},
           {'id':'first','target':'tool','tool':'shell','at':'before','priority':10,'text':'A18_FIRST'}]
    doc={'spec':'style/v0.3','name':name,'version':'1.0.1' if combined else '1.0.0','description':'Conformance upgrade fixture','rules':rules}
    if combined:
        rules[1].update(refresh=2,repeat='session',reason='Combined refresh acceptance',fail='closed',text='A18_FIRST provider={{provider}} effort={{thinking_effort}}',when={'provider':['fixture','anthropic'],'thinking_effort':'high'})
        rules.extend([{'id':'last','target':'tool','tool':'shell','at':'before','priority':90,'text':'A18_LAST'},
                      {'id':'unknown','target':'tool','tool':'shell','at':'before','when':{'model':'unknown'},'text':'A18_UNKNOWN'},
                      {'id':'fence','target':'event','event':'prompt.submitted','repeat':'every','reason':'Explicit event fixture','text':'A18_EVENT {{event.prompt}}'}])
    return json.dumps(doc,indent=2)+'\n'


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',required=True,type=Path)
    parser.add_argument('--old-binary',required=True,type=Path)
    parser.add_argument('--codex-ancestor',required=True,type=Path)
    parser.add_argument('--report',required=True,type=Path)
    args=parser.parse_args();binary=args.binary.resolve();old=args.old_binary.resolve();cases=[]
    for route in ['codex-user','codex-plugin','codex-project','claude-user','claude-project','claude-plugin']:
        harness='codex' if route.startswith('codex') else 'claude-code'
        with tempfile.TemporaryDirectory(prefix='style-m3-conformance-') as temp:
            root=Path(temp).resolve();env,base,codex,claude,stub=fixture(root);(stub/'style').symlink_to(old)
            project=root/"project ' space";project.mkdir();scope=['--project',project] if route.endswith('-project') else []
            source=root/'source';source.mkdir();(source/'STYLE.yaml').write_text(manifest('first',False))
            selection=['--harness',harness,'--route',harness+'='+route]
            installed=style_report(run(old,env,'install',source,*scope,*selection,'--yes','--json',want=1 if route=='claude-plugin' else 0).stdout)
            home=Path(installed['scope']['home']) if scope else base
            if route=='codex-plugin':
                hook_path=home/'adapters/codex/plugin/hooks/hooks.json';env['PLUGIN_ROOT']=str(hook_path.parent.parent)
            elif route=='claude-plugin':
                registration=installed['registration'][0];market=Path(registration['marketplace']);plugin=market/'plugins/style-runtime'
                hook_path=plugin/'hooks/hooks.json';env['CLAUDE_PLUGIN_ROOT']=str(plugin)
                metadata=claude/'plugins';metadata.mkdir(exist_ok=True);market_name=registration['plugin'].split('@')[1]
                (metadata/'known_marketplaces.json').write_text(json.dumps({market_name:{'source':{'source':'directory','path':str(market)},'installLocation':str(market)}}))
                (metadata/'installed_plugins.json').write_text(json.dumps({'version':2,'plugins':{registration['plugin']:[{'scope':'user','version':'0.1.0','installPath':str(metadata/'cache'/market_name/'style-runtime/0.1.0')}]}}))
                settings_path=claude/'settings.json';settings=json.loads(settings_path.read_text()) if settings_path.exists() else {}
                settings['enabledPlugins']={registration['plugin']:True};settings_path.write_text(json.dumps(settings))
            elif scope:hook_path=project/('.codex/hooks.json' if harness=='codex' else '.claude/settings.json')
            else:hook_path=codex/'hooks.json' if harness=='codex' else claude/'settings.json'
            def payload(event,turn='',session='upgrade',known=True):
                doc={'session_id':session,'hook_event_name':event,'tool_name':'Bash','tool_input':{'command':'true'},'cwd':str(project),
                     'source':'startup','turn_id':turn,'prompt_id':turn,'prompt':'A18_PRIVATE </style-untrusted-event>'}
                if known:doc['model']='fixture-model'
                if harness=='claude-code':doc['effort']={'level':'future'}
                return json.dumps(doc)
            def invoke(event,turn='',session='upgrade',known=True):
                hook=handler(hook_path,event);raw=payload(event,turn,session,known)
                if route=='claude-plugin':hook={**hook,'args':[arg.replace('${CLAUDE_PLUGIN_ROOT}',str(plugin)) for arg in hook['args']]}
                if harness=='codex':
                    result=subprocess.run([str(args.codex_ancestor.resolve()),'-c','model_reasoning_effort=future','-c','model_provider=fixture'],env=env,input=json.dumps({'hook':hook,'cwd':str(project),'payload':raw}),text=True,capture_output=True,timeout=20)
                else:result=invoke_hook(hook,env,project,raw)
                assert result.returncode==0 and 'level=ERROR' not in result.stderr,(route,result.stdout,result.stderr)
                return result.stdout
            assert json.loads((home/'build/index.json').read_text())['format']==1
            assert 'A18_FIRST' in invoke('PreToolUse')
            old_state=home/'state/upgrade/session.json';assert old_state.exists()
            record(cases,route,'actual-old-install-lock-index-state')
            (source/'STYLE.yaml').write_text(manifest('first',True))
            (home/'effort.yaml').write_text('format: 1\nharnesses:\n  '+harness+': {future: high}\n')
            run(binary,env,*scope,'update','--all','--json', confirm=True)
            (stub/'style').unlink();(stub/'style').symlink_to(binary)
            alias=root/'alias';alias.mkdir();(alias/'STYLE.yaml').write_text(manifest('second',True))
            installed=style_report(run(binary,env,*scope,'install',alias,*selection,'--json', confirm=True).stdout)
            second=next(r for r in installed['reports'] if r['plan']['package']=='second')
            assert all(r.get('deduplicated_into','').startswith('first/') for r in second['plan']['rules'])
            assert second['session_cost']['total']==0
            index=home/'build/index.json';assert json.loads(index.read_text())['format']==7
            invoke('SessionStart')
            event=root/'event.json';event.write_text(payload('PreToolUse'))
            facts=root/'facts.json';facts.write_text(json.dumps({'format':1,'harness':harness,'payload_sha256':'sha256:'+hashlib.sha256(event.read_bytes()).hexdigest(),'model_provider':'fixture' if harness=='codex' else 'anthropic',('codex_config_effort' if harness=='codex' else 'claude_env_effort'):'future'}))
            before=snapshot(home/'state',hook_path.parent)
            explain=style_report(run(binary,env,'explain','--harness',harness,'--event','PreToolUse','--payload',event,'--facts',facts,'--index',index,'--state-dir',home/'state','--json').stdout)
            assert snapshot(home/'state',hook_path.parent)==before
            actual=invoke('PreToolUse');assert actual==explain['wire']['stdout'],(route,actual,explain['wire'])
            assert actual.count('A18_FIRST')==1 and actual.index('A18_FIRST')<actual.index('A18_LAST'),(route,actual)
            assert ('provider=fixture' if harness=='codex' else 'provider=anthropic') in actual and 'effort=high' in actual
            record(cases,route,'upgrade-index7-mapping-provider-sort-dedup-readonly-explain-parity')
            assert 'A18_FIRST' not in invoke('PreToolUse')
            fence=invoke('UserPromptSubmit','one');assert 'A18_EVENT' in fence and '\\u003c/style-untrusted-event' in fence
            assert 'A18_FIRST' not in invoke('PreToolUse','one')
            invoke('UserPromptSubmit','two');assert 'A18_FIRST' in invoke('PreToolUse','two')
            assert 'A18_UNKNOWN' in invoke('PreToolUse',session='unknown',known=False)
            record(cases,route,'refresh-fence-unknown-combination')
            saved=old_state.read_bytes();old_state.write_text('{')
            # Failure output is expected to contain a fixed error diagnostic.
            hook=handler(hook_path,'PreToolUse')
            if route=='claude-plugin':hook={**hook,'args':[arg.replace('${CLAUDE_PLUGIN_ROOT}',str(plugin)) for arg in hook['args']]}
            failed=invoke_hook(hook,env,project,payload('PreToolUse'))
            assert failed.returncode==0 and '"deny"' in failed.stdout
            old_state.write_bytes(saved);assert '"deny"' not in invoke('PreToolUse','two')
            record(cases,route,'combined-fail-closed-and-recovery')
            original_index=index.read_bytes();lock=home/'style.lock.yaml';locked=lock.read_bytes();clock=snapshot(home/'state')
            (home/'effort.yaml').write_text('invalid current mapping');index.unlink();(home/'build/explain.json').unlink()
            run(binary,env,*scope,'restore','--json', confirm=True)
            assert index.read_bytes()==original_index and lock.read_bytes()==locked and snapshot(home/'state')==clock
            before=snapshot(base,codex,claude,project)
            run(old,{**env,'STYLE_HOME':str(home)},'sync',want=1)
            assert snapshot(base,codex,claude,project)==before
            record(cases,route,'frozen-approved-mapping-state-preserved-old-writer-rejected')
            (home/'effort.yaml').unlink()
            # Removal still recompiles the remaining package, so restore valid mapping.
            (home/'effort.yaml').write_text('format: 1\nharnesses:\n  '+harness+': {future: high}\n')
            run(binary,env,*scope,'uninstall','first','--json', confirm=True)
            output=invoke('PreToolUse',session='remaining');assert output.count('A18_FIRST')==1
            assert 'second/first' in (home/'build/index.json').read_text()
            if route=='claude-plugin':
                settings['enabledPlugins'][registration['plugin']]=False;settings_path.write_text(json.dumps(settings))
            run(binary,env,*scope,'uninstall','--all','--json', confirm=True)
            assert not index.exists()
            record(cases,route,'canonical-promotion-and-last-uninstall')
    args.report.write_text(json.dumps({'format':1,'passed':True,'real_harness':False,'binary_sha256':sha(binary),'old_binary_sha256':sha(old),'ancestor_sha256':sha(args.codex_ancestor),'cases':cases},indent=2)+'\n')
    print(json.dumps({'passed':True,'checks':len(cases)}))


if __name__=='__main__':main()

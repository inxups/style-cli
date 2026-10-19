#!/usr/bin/env python3
"""M3 real-install probe timeout/interruption checks; never invokes a model."""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
from types import SimpleNamespace

from m3_live_fixture import package
from m3_probe_support import PrivateBaseline, private_write, repository_binding, sha, wait_probe
from probe_codex_routes import register_plugin, unregister_plugin


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['style','harness-binary','scratch','output']:parser.add_argument('--'+name,required=True,type=Path)
    parser.add_argument('--harness',required=True,choices=['codex','claude-code'])
    parser.add_argument('--route',required=True)
    args=parser.parse_args();os.umask(0o077)
    binding=repository_binding(Path(__file__).resolve().parents[2],args.style,args.harness_binary)
    root=args.scratch.resolve()
    if root.exists() and any(root.iterdir()):raise ValueError('acceptance scratch directory must be empty')
    root.mkdir(parents=True,exist_ok=True,mode=0o700)
    profile=root/('codex' if args.harness=='codex' else 'claude');profile.mkdir()
    work=root/'work';work.mkdir();bindir=root/'native-bin';bindir.mkdir()
    native_name='codex' if args.harness=='codex' else 'claude'
    (bindir/native_name).symlink_to(args.harness_binary.resolve());(bindir/'style').symlink_to(args.style.resolve())
    env={key:os.environ[key] for key in ['PATH','TMPDIR','LANG','LC_ALL','TERM'] if key in os.environ}
    env.update(HOME=str(root/'home'),STYLE_HOME=str(root/'store'),PATH=str(bindir)+os.pathsep+env.get('PATH',''),
               CODEX_HOME=str(profile if args.harness=='codex' else root/'codex'),
               CLAUDE_CONFIG_DIR=str(profile if args.harness=='claude-code' else root/'claude'),
               CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1',DISABLE_AUTOUPDATER='1')
    Path(env['HOME']).mkdir()
    # This collector has no HTTP server, proxy or inference entry point and
    # never reads a real authentication profile. Native calls are local plugin
    # management or --version only.
    def native_command(*argv):
        if argv[0] not in ['plugin','--version']:raise ValueError('model invocation is forbidden')
        result=subprocess.run([str(args.harness_binary.resolve()),*argv],cwd=work,env=env,capture_output=True,text=True,timeout=60)
        if result.returncode:raise ValueError('native management failed: '+argv[0])
        return result.stdout
    probe=SimpleNamespace(root=root,style=args.style.resolve(),work=work,store=root/'store',native_command=native_command)
    version=native_command('--version').strip()
    names=['config.toml','hooks.json'] if args.harness=='codex' else ['settings.json','rules/style-m3-live.md','plugins/known_marketplaces.json','plugins/installed_plugins.json']
    scope=['--project',str(work)] if args.route.endswith('-project') else []
    if args.harness=='codex':
        config='[features]\ncodex_hooks=true\n'
        if scope:
            subprocess.run(['git','init','-q',str(work)],check=True,env=env)
            config+='[projects.'+json.dumps(str(work))+']\ntrust_level="trusted"\n'
        private_write(profile/'config.toml',config)
    baseline=PrivateBaseline(profile,names)
    project_baseline=PrivateBaseline(work,['.codex/hooks.json','.codex/config.toml','.claude/settings.json','.claude/rules/style-m3-live.md','.style/project.json','.style/hooks/style'])
    source=package(probe.root,args.harness);registered=None;plugin_name=None
    report={'format':1,'harness':args.harness,'harness_version':version,'route':args.route,'passed':False,'models_invoked':False,'authentication_profile_read':False,'local_http_server':False}
    def style(*argv,expected=0,confirm=False):
        result=subprocess.run([str(probe.style),*argv,*scope,'--json'],cwd=probe.work,env=env,input='y\n' if confirm else '',capture_output=True,text=True,timeout=20)
        assert result.returncode==expected,(argv[0],result.returncode)
        return json.loads(result.stdout)
    try:
        installed=style('install',str(source),'--harness',args.harness,'--route',args.harness+'='+args.route,expected=1 if args.route=='claude-plugin' else 0, confirm=True)
        if args.route=='codex-plugin':register_plugin(probe);registered='codex'
        if args.route=='claude-plugin':
            entry=installed['registration'][0];plugin_name=entry['plugin'];registered=plugin_name.split('@')[1]
            probe.native_command('plugin','marketplace','add',entry['marketplace'],'--scope','user','--json')
            probe.native_command('plugin','install',plugin_name,'--scope','user','--json')
        timeout=style('probe','--harness',args.harness,'--timeout=1s',expected=1)
        assert timeout['status']=='timed-out' and timeout['cleanup']['complete'] and not any(t['callback_observed'] for t in timeout['targets'])
        assert len(timeout['targets'])==1 and timeout['targets'][0]['route']==args.route
        assert all(t['index_status']=='captured' for t in timeout['targets'])
        stdout=probe.root/'interrupt.private.json';stderr=probe.root/'interrupt.private.stderr'
        with stdout.open('w') as out,stderr.open('w') as err:
            process=subprocess.Popen([str(probe.style),'probe','--harness',args.harness,'--timeout=60s',*scope,'--json'],cwd=probe.work,env=env,stdout=out,stderr=err)
            try:
                wait_probe(process,stderr);process.send_signal(signal.SIGINT);process.wait(timeout=10)
            finally:
                if process.poll() is None:process.terminate();process.wait(timeout=10)
        interrupted=json.loads(stdout.read_bytes());assert interrupted['status']=='interrupted' and interrupted['cleanup']['complete'] and process.returncode!=0
        assert len(interrupted['targets'])==1 and interrupted['targets'][0]['route']==args.route
        assert all(t['index_status']=='captured' for t in interrupted['targets'])
        report.update(timeout=timeout,interrupted=interrupted,interrupt_exit=process.returncode)
        if registered=='codex':unregister_plugin(probe);registered=None
        elif registered:
            probe.native_command('plugin','uninstall',plugin_name,'--scope','user','--json')
            probe.native_command('plugin','marketplace','remove',registered,'--scope','user','--json');registered=None
        style('uninstall','--all', confirm=True);baseline.restore()
        report['restoration']=baseline.verify();report['project_restoration']=project_baseline.verify()
        report['binding']=binding.verify();report['passed']=True
    except Exception as error:
        private_write(probe.root/'failure.private.txt',repr(error));report['failure_type']=type(error).__name__;report['failure_sha256']=sha(str(error).encode());raise
    finally:
        if registered=='codex':unregister_plugin(probe)
        elif registered:
            probe.native_command('plugin','uninstall',plugin_name,'--scope','user','--json')
            probe.native_command('plugin','marketplace','remove',registered,'--scope','user','--json')
        if not report['passed']:baseline.restore();project_baseline.restore()
        private_write(args.output,json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':main()

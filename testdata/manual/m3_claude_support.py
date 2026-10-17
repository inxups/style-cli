"""M3 authenticated native Claude discovery and independently replayed requests."""
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import termios
import time

from claude_live_probe import LiveClaudeProbe, replay_requests
from claude_probe_support import text_values
from m3_probe_support import private_write, sha


class M3ClaudeProbe(LiveClaudeProbe):
    def __init__(self,*args,**kwargs):
        super().__init__(*args,**kwargs)
        self.config=self.auth_config
        self.live_env['PATH']=str(self.root/'native-bin')+os.pathsep+self.live_env.get('PATH','')
        self.live_env['CLAUDE_CODE_EFFORT_LEVEL']='high'
        self.live_env['CLAUDE_EFFORT']='high'
        self.live_env.pop('CODEX_HOME',None);self.live_env.pop('CODEX_SANDBOX',None)

    def invoke(self,name,prompt,extra=(),tools='',timeout=180,setting_sources='',settings=None,model='claude-haiku-5-5'):
        # Discover the actual user/project/plugin configuration. Historical
        # LiveClaudeProbe's explicit settings-file mode is not route acceptance.
        return super().invoke(name,prompt,extra,tools,timeout,setting_sources='user,project,local',settings=None,model=model)

    def native_command(self,*args):
        result=subprocess.run([self.claude,*args],cwd=self.work,env=self.live_env,capture_output=True,text=True,timeout=60)
        if result.returncode:raise ValueError('native Claude management failed: '+args[0])
        return result.stdout

    def style_command(self,*args,scope=(),expected=0):
        result=subprocess.run([str(self.style),*args,*scope,'--json'],cwd=self.work,env=self.live_env,capture_output=True,text=True,timeout=60)
        if result.returncode!=expected:raise ValueError('STYLE management failed: '+args[0])
        return json.loads(result.stdout)

    def capture_state(self,call):
        session=call['session_id'];path=self.store/'state'/session/'session.json'
        call['state']=None;call['state_decode_failed']=False
        if path.exists():
            try:call['state']=json.loads(path.read_bytes())
            except json.JSONDecodeError:call['state_decode_failed']=True
        path=self.store/'state'/session/'main/stats.jsonl'
        call['stats']=[json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        call['requests']=call['native_requests']
        rows=[json.loads(line) for line in Path(call['stdout_path']).read_text().splitlines() if line.startswith('{')]
        call['native_hook_events']=[row for row in rows if row.get('type')=='system' and 'hook' in row.get('subtype','')]
        return call

    def live(self,name,prompt,resume=None,tools=''):
        extra=['--resume',resume] if resume else []
        if tools:extra+=['--allowedTools',tools]
        start=time.time_ns()
        call=self.capture_state(self.invoke_live(name,prompt,extra=extra,tools=tools))
        call['started_ns']=start;call['ended_ns']=time.time_ns()
        return call

    def delivery(self,call,body):
        rows=[json.loads(line) for line in Path(call['transcript_path']).read_text().splitlines()]
        known=[json.loads(line) for path in self.transcript_paths if str(path)!=call['transcript_path'] for line in path.read_text().splitlines()]
        requested={row['id'] for row in call['native_requests']}
        counts=[]
        for request in replay_requests(rows,known):
            if request['id'] not in requested:continue
            count=sum(text.count(body) for _,text in text_values({k:request['body'][k] for k in ('system','messages') if k in request['body']}))
            counts.append(count)
        return {'body_sha256':sha(body.encode()),'complete_body':bool(counts and counts[-1]),'request_occurrences':counts[-1] if counts else 0,'per_request_occurrences':counts}

    def trust_workspace(self):
        master,slave=pty.openpty();fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,140,0,0))
        argv=[self.claude,'--model','claude-haiku-5-5','--setting-sources','user,project,local','--strict-mcp-config','--mcp-config','{"mcpServers":{}}']
        process=subprocess.Popen(argv,stdin=slave,stdout=slave,stderr=slave,cwd=self.work,env=self.live_env,start_new_session=True);os.close(slave)
        chunks=[];accepted=False;exit_sent=False;start=time.monotonic()
        try:
            while time.monotonic()-start<35 and process.poll() is None:
                ready,_,_=select.select([master],[],[],.1)
                if ready:
                    try:chunk=os.read(master,65536)
                    except OSError:break
                    chunks.append(chunk)
                text=b''.join(chunks).decode(errors='replace')
                if not accepted and ('Yes, I trust this folder' in text or 'Yes, trust folder' in text):
                    os.write(master,b'\r');accepted=True
                # An interactive startup is sufficient to record native folder
                # trust. No prompt or model request is sent through this UI.
                if time.monotonic()-start>12 and not exit_sent:
                    os.write(master,b'/exit\r');exit_sent=True
            if process.poll() is None:process.terminate()
            process.wait(timeout=10)
        finally:
            os.close(master);private_write(self.root/'trust-ui.private.txt',b''.join(chunks))
            if process.poll() is None:process.kill();process.wait(timeout=10)
        return {'native_trust_prompt_accepted':accepted,'ui_exit':process.returncode,'ui_log_sha256':sha(b''.join(chunks)),'bypass_permissions':False}

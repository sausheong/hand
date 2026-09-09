"""Run one real Pi RPC read-tool turn against the guarded fixture gateway."""
import hashlib
import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import sys
import time

from evaluation_pi_events import PiRunObserver


def check(runtime,config_path,report_path):
    runtime=Path(runtime).resolve();report_path=Path(report_path).resolve();root=report_path.parent
    config=json.loads(Path(config_path).read_text());model='claude-sonnet-4-5-20250929'
    package=runtime/'node_modules/@earendil-works/pi-coding-agent';metadata=json.loads((package/'package.json').read_text())
    if metadata['version']!='0.85.1':raise ValueError('pinned Pi CLI required')
    home=root/'agent-home';home.mkdir();agent=home/'agent';agent.mkdir();workspace=root/'workspace';workspace.mkdir()
    fixture=workspace/'example.go';fixture.write_text('package main\n// HAND_GATEWAY_READ_PROBE\n')
    (agent/'models.json').write_text(json.dumps({'providers':{'anthropic':{'baseUrl':config['url'],'apiKey':config['token'],'modelOverrides':{model:{'maxTokens':4096}}}}}))
    guard=root/'fetch-guard.mjs'
    guard.write_text('const origin='+json.dumps(config['url'])+';\n'+'''const upstream = new URL(origin);
if (upstream.hostname !== '127.0.0.1' || upstream.protocol !== 'http:') throw new Error('loopback required');
const nativeFetch = globalThis.fetch;
globalThis.fetch = async (input, init) => {
 const url = new URL(input instanceof Request ? input.url : String(input));
 if (url.origin !== upstream.origin || url.pathname !== '/v1/messages') throw new Error('non-gateway request forbidden');
 return nativeFetch(input, {...init, redirect:'error'});
};
''')
    node=shutil.which('node');cli=package/'dist/bundle/cli.js'
    argv=[node,'--import',str(guard),str(cli),'--mode','rpc','--no-extensions','--no-skills','--no-prompt-templates','--no-themes',
        '--tools','read','--thinking','off','--provider','anthropic','--model',model,'--session-dir',str(home/'sessions')]
    observer=PiRunObserver('prompt-1','stats-1','anthropic',model)
    report=dict(status='failed',argv=argv,cli_sha256=hashlib.sha256(cli.read_bytes()).hexdigest(),limitations=['Actual CLI/tool turn with read-only fixture tools, thinking off and 4096 output cap; not the default comparison configuration.','Synthetic model responses only; no genuine model task success, live billing or container isolation.'])
    with (root/'agent-stderr.log').open('wb') as stderr,(root/'agent-stdout.jsonl').open('wb') as stdout:
        process=subprocess.Popen(argv,cwd=workspace,env={'HOME':str(home),'PATH':str(Path(node).parent)+':/usr/bin:/bin','PI_CODING_AGENT_DIR':str(agent)},stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=stderr,start_new_session=True)
        try:
            def send(value):process.stdin.write((json.dumps(value)+'\n').encode());process.stdin.flush()
            send(dict(id='prompt-1',type='prompt',message='Read example.go and report the marker you found.'))
            started=time.monotonic();stats_sent=False
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout,selectors.EVENT_READ)
                while observer.stats is None:
                    if time.monotonic()-started>20:raise TimeoutError('Pi tool turn did not settle')
                    if not selector.select(0.1):continue
                    chunk=os.read(process.stdout.fileno(),65536)
                    if not chunk:raise ValueError('Pi closed before statistics')
                    stdout.write(chunk);stdout.flush();observer.feed(chunk)
                    if observer.errors:raise ValueError('Pi RPC observation failed')
                    if observer.ready_for_stats and not stats_sent:
                        send(dict(id='stats-1',type='get_session_stats'));stats_sent=True
            process.stdin.close();exit_code=process.wait(timeout=5)
            remainder=process.stdout.read()
            if remainder:stdout.write(remainder);stdout.flush();observer.feed(remainder)
            result=observer.finish();report.update(exit_code=exit_code,observation=result)
            events=[json.loads(line) for line in (root/'agent-stdout.jsonl').read_text().splitlines()]
            tools=[e for e in events if e.get('type')=='tool_execution_end']
            report['tools']=tools
            if (exit_code!=0 or result['status']!='observed_unverified' or result['outcome']!='completed'
                    or result['assistant_messages']!=2 or len(tools)!=1 or tools[0].get('toolName')!='read'
                    or tools[0].get('isError') or 'HAND_GATEWAY_READ_PROBE' not in json.dumps(tools[0])):
                raise ValueError('real read/tool settlement was not verified')
            report['status']='sdk_gateway_passed'
        except Exception as error:
            report['error_type']=type(error).__name__
        finally:
            if process.poll() is None:process.kill();process.wait(timeout=5)
            process.stdout.close()
            if not process.stdin.closed:process.stdin.close()
            report_path.write_text(json.dumps(report,indent=2)+'\n')
    return report


if __name__=='__main__':
    result=check(*sys.argv[1:]);print(json.dumps({'status':result['status']}));raise SystemExit(0 if result['status']=='sdk_gateway_passed' else 1)

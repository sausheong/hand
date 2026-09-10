#!/usr/bin/env python3
"""Run a compiled Hand background HTTP server inside its container boundary."""
import argparse
import hashlib
import json
import subprocess
import threading
import time
import traceback
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from check_rpc_client import Client


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ['hand-binary', 'worker', 'docker', 'socket', 'out']:
        p.add_argument('--'+name, required=True, type=Path)
    p.add_argument('--image', required=True)
    a = p.parse_args()
    binary, worker, docker, socket = [x.resolve(strict=True) for x in [a.hand_binary, a.worker, a.docker, a.socket]]
    out = a.out.resolve(); out.mkdir(parents=True, exist_ok=False)
    workspace = out/'workspace'; workspace.mkdir()
    (workspace/'index.html').write_text('isolated server response')
    sentinel = out/'outside-secret'; sentinel.write_text('outside sentinel')
    home = out/'home'; (home/'.hand').mkdir(parents=True)
    sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
    config = dict(backend='container', docker=str(docker), socket=str(socket), image=a.image,
                  worker=str(worker), worker_sha256=sha(worker), writable=False, network=False)
    (home/'.hand/config.json').write_text(json.dumps(dict(execution=config)))
    docker_config=out/'docker-config'; docker_config.mkdir()
    base=[str(docker),'--config',str(docker_config),'--host','unix://'+str(socket)]
    commands=[]
    def run(args):
        r=subprocess.run(base+args,capture_output=True,text=True,timeout=20)
        commands.append(dict(args=args,code=r.returncode,stdout=r.stdout,stderr=r.stderr))
        (out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
        if r.returncode: raise RuntimeError(r.stderr)
        return r.stdout
    requests=[]
    class Provider(BaseHTTPRequestHandler):
        def log_message(self,*args): pass
        def do_POST(self):
            requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
            delta={'content':'server started'}; finish='stop'
            if len(requests)==1:
                args=json.dumps(dict(action='start',command='python3 -u -m http.server 8765 --bind 127.0.0.1'))
                delta={'tool_calls':[dict(index=0,id='server',type='function',function=dict(name='process',arguments=args))]};finish='tool_calls'
            self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
            for d,f in [(delta,None),({},finish)]: self.wfile.write(('data: '+json.dumps(dict(choices=[dict(index=0,delta=d,finish_reason=f)]))+'\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
    server=ThreadingHTTPServer(('127.0.0.1',0),Provider)
    thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
    client=None; container=None
    report=dict(status='failed',qualification='development',image=a.image,binary_sha256=sha(binary),worker_sha256=sha(worker),runner_sha256=sha(Path(__file__)))
    try:
        client=Client(binary,workspace,f'http://127.0.0.1:{server.server_port}/v1',out,'client')
        hello=client.call('hello','hello'); assert 'container' in json.dumps(hello)
        client.call('prompt','prompt',dict(text='Start a background HTTP server'))
        deadline=time.monotonic()+15;approved=False
        while time.monotonic()<deadline:
            pending=client.call('pending','approval.pending')['approvals']
            if pending:
                assert len(pending)==1 and not approved
                event=pending[0]; assert event['request_id']=='prompt'
                client.call('approve','approval.respond',dict(run_id=event['run_id'],approval_id=event['payload']['approval_id'],decision='once'));approved=True
            record=client.call('record','request.get',dict(id='prompt'))
            if record['state']=='completed': break
            time.sleep(.01)
        assert approved and record['state']=='completed', record
        own=[]
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            ids=run(['ps','-q','--filter','name=harness-exec-']).split()
            own=[]
            for ident in ids:
                info=json.loads(run(['inspect',ident]))[0]
                if any(m['Destination']=='/workspace' and Path(m['Source']).resolve()==workspace for m in info['Mounts']):own.append((ident,info))
            if own: break
            time.sleep(.05)
        assert len(own)==1, own
        container,info=own[0]; (out/'container.json').write_text(json.dumps(info,indent=2)+'\n')
        assert info['HostConfig']['NetworkMode']=='none' and info['HostConfig']['ReadonlyRootfs']
        assert all(not m['RW'] for m in info['Mounts'] if m['Destination']=='/workspace')
        assert {m['Destination'] for m in info['Mounts']} <= {'/workspace','/tmp','/harness-resources','/hand-worker'}
        assert all(not m['RW'] for m in info['Mounts'] if m['Destination'] != '/tmp')
        probe = "import pathlib,time,urllib.request;\nfor i in range(100):\n try:\n  data=urllib.request.urlopen('http://127.0.0.1:8765',timeout=1).read();break\n except OSError: time.sleep(.01)\nelse: raise RuntimeError('server not ready')\nassert data==b'isolated server response'\nassert not pathlib.Path("+repr(str(sentinel))+").exists()\nassert not pathlib.Path('/var/run/docker.sock').exists()\nprint('server and boundary verified')"
        assert 'server and boundary verified' in run(['exec',container,'python3','-c',probe])
        client.close();client=None
        assert container not in run(['ps','-aq','--filter','name=harness-exec-']).split(), 'container survived Hand shutdown'
        assert len(requests)==2
        report.update(status='passed',provider_calls=2,explicit_approval=True,server_responded=True,inspected_isolation=True,shutdown_joined=True)
    except Exception as exc:
        report['error']=repr(exc)
        report['traceback']=traceback.format_exc()
    finally:
        if client is not None:
            try: client.close()
            except Exception as exc: report['cleanup_error']=repr(exc)
        server.shutdown();server.server_close();thread.join()
        (out/'provider-requests.json').write_text(json.dumps(requests,indent=2)+'\n')
        (out/'run.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(report));return 0 if report['status']=='passed' else 1

if __name__=='__main__': raise SystemExit(main())

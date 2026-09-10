#!/usr/bin/env python3
"""Exercise profile selection and dismissal in a compiled Hand terminal."""
import fcntl
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import pty
import select
import signal
import struct
import subprocess
import termios
import threading
import time
from benchmark_startup import ANSI
from check_skill_approval_pty import main


def trial(binary, output, decision):
    directory = output/decision
    home, workspace = directory/'home', directory/'workspace'
    (home/'.hand').mkdir(parents=True); workspace.mkdir()
    requests = []

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            size = int(self.headers.get('Content-Length', 0))
            if size > 1 << 20:
                self.send_error(413); return
            body = json.loads(self.rfile.read(size))
            requests.append(dict(model=body.get('model'), output=body.get('max_completion_tokens'), authorization=self.headers.get('Authorization')))
            self.send_response(200); self.send_header('Content-Type', 'text/event-stream'); self.end_headers()
            self.wfile.write(b'data: {"choices":[{"index":0,"delta":{"content":"PROFILE_REQUEST_FINISHED"},"finish_reason":null}]}\n\ndata: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')

    server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    config = dict(default_profile='alpha', profiles={name:dict(provider='local', model=model,
                  endpoint=f'http://127.0.0.1:{server.server_port}/v1', context_limit=24576, max_output=limit)
                  for name,model,limit in [('alpha','original',123),('beta','selected',512)]})
    (home/'.hand/config.json').write_text(json.dumps(config))
    env = dict(os.environ, HOME=str(home), XDG_CONFIG_HOME=str(home/'.config'), TERM='xterm-256color')
    for key in list(env):
        if key.endswith('_API_KEY') or key in ('OPENAI_ACCESS_TOKEN','ANTHROPIC_AUTH_TOKEN'):
            env.pop(key)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',24,80,0,0))
    process = None
    raw = bytearray(); phase = 'startup'; error = None
    try:
        process = subprocess.Popen([str(binary)], cwd=workspace, env=env, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave); slave = None
        started = time.monotonic()
        while time.monotonic()-started < 20:
            if select.select([master],[],[],.02)[0]:
                try:
                    chunk = os.read(master,65536)
                except OSError:
                    chunk = b''
                raw.extend(chunk)
                if len(raw)>4<<20:
                    raise RuntimeError('PTY capture exceeded limit')
                visible = ANSI.sub(b'',bytes(raw))
                if phase=='startup' and b'Type a message...' in visible:
                    if termios.tcgetattr(master)[3]&termios.ECHO:
                        raise RuntimeError('input echo enabled')
                    os.write(master,b'/profile\r'); phase='picker'
                elif phase=='picker' and b'Choose profile' in visible and b'Configured context:' in visible:
                    if requests:
                        raise RuntimeError('opening picker called provider')
                    os.write(master,{'select':b'\x1b[B\r','escape':b'\x1b','cancel':b'\x03'}[decision]); phase='closed'
                elif phase=='closed':
                    ready = b'profile switched to beta' in visible if decision=='select' else b'Type a message' in ANSI.sub(b'',chunk)
                    if ready:
                        os.write(master,b'answer a short question\r'); phase='request'
                elif phase=='request' and b'PROFILE_REQUEST_FINISHED' in visible and b'[result] Completed:' in visible:
                    os.write(master,b'/exit\r'); phase='exit'
            if process.poll() is not None:
                break
        if process.poll() is None or process.returncode!=0 or phase!='exit':
            raise RuntimeError('journey did not exit cleanly: '+phase)
        expected = dict(model='selected' if decision=='select' else 'original', output=512 if decision=='select' else 123, authorization=None)
        if requests != [expected]:
            raise RuntimeError('request did not use selected or retained profile: '+repr(requests))
    except (OSError, ValueError, RuntimeError) as exc:
        error = str(exc)
    finally:
        if process is not None:
            if process.poll() is None:
                os.killpg(process.pid,signal.SIGKILL); process.wait(timeout=5)
            try:
                os.killpg(process.pid,signal.SIGKILL)
            except ProcessLookupError:
                pass
        if slave is not None:
            os.close(slave)
        os.close(master); server.shutdown(); server.server_close(); thread.join(timeout=5)
        (directory/'terminal.pty').write_bytes(raw)
        (directory/'requests.json').write_text(json.dumps(requests,indent=2)+'\n')
    return dict(decision=decision,status='failed' if error else 'passed',error=error,phase=phase,requests=requests,exit_code=process.returncode if process else None)


if __name__=='__main__':
    raise SystemExit(main(trial_runner=trial,decisions=('select','escape','cancel')))

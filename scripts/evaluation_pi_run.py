"""Bounded Pi RPC process ownership; no model-call authorisation or task scoring."""
from contextlib import ExitStack
import hashlib
import json
import math
import os
from pathlib import Path
import selectors
import signal
import subprocess
import time
from evaluation_pi_events import PiRunObserver


def run_pi(argv, workspace, evidence, environment, prompt, provider, model,
           timeout_seconds=60, max_output_bytes=16 << 20, cancelled=None):
    if not isinstance(argv, list) or not argv or any(not isinstance(a, str) or '\0' in a for a in argv) or not argv[0]:
        raise ValueError('explicit argv required')
    if not isinstance(environment, dict) or any(not isinstance(k, str) or not k or '=' in k or '\0' in k or not isinstance(v, str) or '\0' in v for k,v in environment.items()):
        raise ValueError('explicit environment required')
    if not isinstance(prompt, str) or not prompt or len(prompt.encode()) > 1 << 20:
        raise ValueError('bounded nonempty prompt required')
    if type(timeout_seconds) not in (int,float) or not 0 < timeout_seconds <= 86400 or not math.isfinite(timeout_seconds):
        raise ValueError('bounded deadline required')
    observer = PiRunObserver('prompt-1','stats-1',provider,model,max_bytes=max_output_bytes)
    workspace,evidence = Path(workspace),Path(evidence)
    if not workspace.is_absolute() or workspace != workspace.resolve() or not workspace.is_dir() or not evidence.is_absolute() or evidence != evidence.resolve() or workspace == evidence or workspace in evidence.parents:
        raise ValueError('canonical workspace and separate evidence required')
    if cancelled is not None and cancelled.is_set():
        raise InterruptedError('cancelled before launch')
    evidence.mkdir(mode=0o700)
    paths = {name:evidence/(name+'.log') for name in ('stdout','stderr')}
    pending = (json.dumps(dict(id='prompt-1',type='prompt',message=prompt))+'\n').encode()
    process=None;outcome=None;joined=False;total=0;stats_sent=False;signalled=False;cleanup_deadline=None
    start=time.monotonic()
    def kill_group():
        nonlocal signalled
        if process is not None and not signalled:
            try:os.killpg(process.pid,signal.SIGKILL)
            except ProcessLookupError:pass
            signalled=True
    with ExitStack() as stack:
        logs={name:stack.enter_context(path.open('xb')) for name,path in paths.items()}
        selector=stack.enter_context(selectors.DefaultSelector())
        try:
            process=subprocess.Popen(argv,cwd=workspace,env=environment,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,start_new_session=True)
            for name in ('stdin','stdout','stderr'):
                pipe=getattr(process,name);stack.callback(pipe.close);os.set_blocking(pipe.fileno(),False)
                selector.register(pipe,selectors.EVENT_WRITE if name=='stdin' else selectors.EVENT_READ,name)
            while True:
                now=time.monotonic();code=process.poll()
                if outcome is None:
                    if cancelled is not None and cancelled.is_set():outcome='cancelled'
                    elif now-start >= timeout_seconds:outcome='timed_out'
                    elif code is not None:outcome='completed' if code==0 else 'failed'
                    if outcome is not None:
                        kill_group();cleanup_deadline=now+2
                        if not process.stdin.closed:
                            try:selector.unregister(process.stdin)
                            except KeyError:pass
                            process.stdin.close()
                if outcome is not None and code is not None and not selector.get_map():
                    joined=True;break
                if cleanup_deadline is not None and now>=cleanup_deadline:
                    outcome='cleanup_failed';break
                for key,_ in selector.select(0.02):
                    if key.data=='stdin':
                        try:
                            written=os.write(key.fileobj.fileno(),pending[:65536]);pending=pending[written:]
                            if not pending:selector.unregister(key.fileobj)
                        except BrokenPipeError:
                            selector.unregister(key.fileobj);key.fileobj.close()
                        continue
                    data=os.read(key.fileobj.fileno(),65536)
                    if not data:
                        selector.unregister(key.fileobj);continue
                    available=max_output_bytes-total;kept=data[:available];logs[key.data].write(kept);total+=len(kept)
                    if key.data=='stdout':observer.feed(kept)
                    if len(data)>available and outcome not in ('timed_out','cancelled','cleanup_failed'):
                        outcome='output_limited';kill_group();cleanup_deadline=cleanup_deadline or time.monotonic()+2
                        if not process.stdin.closed:
                            try:selector.unregister(process.stdin)
                            except KeyError:pass
                            process.stdin.close()
                    if outcome is None and observer.ready_for_stats and not stats_sent and not process.stdin.closed:
                        pending=(json.dumps(dict(id='stats-1',type='get_session_stats'))+'\n').encode()
                        selector.register(process.stdin,selectors.EVENT_WRITE,'stdin');stats_sent=True
                    if observer.stats is not None and not process.stdin.closed:
                        try:selector.unregister(process.stdin)
                        except KeyError:pass
                        process.stdin.close()
            kill_group();process.wait(timeout=2)
            for log in logs.values():log.flush();os.fsync(log.fileno())
        finally:
            try:kill_group()
            finally:
                if process is not None:
                    if process.poll() is None:process.kill()
                    process.wait(timeout=2)
    observation=observer.finish()
    report=dict(status='observed_unverified' if outcome=='completed' and joined and observation['status']=='observed_unverified' else 'incomplete',
                observation=observation,process=dict(status=outcome,exit_code=process.returncode,process_and_pipes_joined=joined,
                output_bytes=total,elapsed_seconds=time.monotonic()-start,argv=argv,timeout_seconds=timeout_seconds,max_output_bytes=max_output_bytes,
                logs={name:dict(path=str(path),sha256=hashlib.sha256(path.read_bytes()).hexdigest()) for name,path in paths.items()}),
                limitation='Process groups are not container isolation; caller owns model approval, billing and independent scoring.')
    (evidence/'pi-run.json').write_text(json.dumps(report,indent=2)+'\n')
    return report

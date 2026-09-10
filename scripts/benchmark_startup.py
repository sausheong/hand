#!/usr/bin/env python3
"""Credential-free PTY startup baseline; records behaviour, not release readiness."""
import argparse
from datetime import datetime, timezone
import errno
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
sys.dont_write_bytecode = True
from evidence import source_snapshot

ROOT = Path(__file__).resolve().parents[1]
ANSI = re.compile(rb'\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]')
MARKER = b'HAND_STARTUP_PROBE'


def percentile(values, fraction):
    if not values:
        return None
    return sorted(values)[math.ceil(len(values) * fraction) - 1]


def trial(binary, fixture, case, output, index, timeout, optional_mcp=False):
    with tempfile.TemporaryDirectory(prefix='hand-startup-') as temporary:
        home = Path(temporary) / 'home'
        workspace = Path(temporary) / 'workspace'
        home.mkdir(); workspace.mkdir(); (home / '.hand').mkdir()
        config = dict(model='local/fixture', base_url='http://127.0.0.1:1/v1')
        if case != 'none':
            config['mcp_servers'] = [dict(name='fixture', command=str(fixture) if case != 'failed' else str(home/'missing-server'),
                                          args=['-delay', '1s'] if case == 'slow' else [])]
        if optional_mcp and case != 'none':
            config['mcp_servers'][0]['optional'] = True
        (home / '.hand/config.json').write_text(json.dumps(config))
        env = dict(os.environ, HOME=str(home), TERM='xterm-256color',
                   XDG_CONFIG_HOME=str(home/'.config'), COLUMNS='120', LINES='40')
        for key in list(env):
            if key.endswith('_API_KEY') or key in ('OPENAI_ACCESS_TOKEN', 'ANTHROPIC_AUTH_TOKEN'):
                env.pop(key)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',40,120,0,0))
        started = time.monotonic()
        process = subprocess.Popen([str(binary)], cwd=workspace, env=env,
                                   stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
        transcript = bytearray()
        sent = False
        ready_ms = None
        failure = None
        status = usage = None
        try:
            while time.monotonic() - started < timeout:
                readable, _, _ = select.select([master], [], [], 0.02)
                if readable:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError as error:
                        if error.errno != errno.EIO:
                            raise
                        chunk = b''
                    if len(transcript)+len(chunk) > 4<<20:
                        failure = 'terminal capture exceeded 4 MiB'
                        break
                    transcript.extend(chunk)
                    visible = ANSI.sub(b'', bytes(transcript))
                    if not sent and b'Type a message...' in visible:
                        if termios.tcgetattr(master)[3] & termios.ECHO:
                            failure = 'terminal echo remains enabled; cannot prove application input'
                            break
                        os.write(master, MARKER)
                        sent = True
                    if sent and ready_ms is None and MARKER in visible:
                        ready_ms = (time.monotonic()-started)*1000
                        os.write(master, b'\x03')
                pid, current_status, current_usage = os.wait4(process.pid, os.WNOHANG)
                if pid:
                    status, usage = current_status, current_usage
                    break
            if status is None:
                failure = failure or 'startup or shutdown deadline exceeded'
                os.killpg(process.pid, signal.SIGKILL)
                _, status, usage = os.wait4(process.pid, 0)
            process.returncode = os.waitstatus_to_exitcode(status)
        finally:
            if process.returncode is None:
                try: os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError: pass
                _, status, usage = os.wait4(process.pid, 0)
                process.returncode = os.waitstatus_to_exitcode(status)
            # Clean any descendants even if the main process exited before them.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            os.close(master)
        path = output / f'{case}-{index:03d}.pty'
        path.write_bytes(transcript)
        expected = (ready_ms is None and process.returncode != 0) if case == 'failed' and not optional_mcp else (ready_ms is not None and process.returncode == 0)
        return dict(case=case, trial=index, startup_to_input_ms=ready_ms,
                    elapsed_ms=(time.monotonic()-started)*1000,
                    exit_code=process.returncode, observation_valid=expected and failure is None,
                    failure=failure, max_rss_bytes=int(usage.ru_maxrss)*(1 if sys.platform=='darwin' else 1024),
                    transcript=path.name, transcript_sha256=hashlib.sha256(transcript).hexdigest())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--source', type=Path, default=ROOT, help='Git checkout to measure; defaults to Hand workspace')
    parser.add_argument('--trials', type=int, default=30)
    parser.add_argument('--optional-mcp', action='store_true', help='expect usable input even when the configured optional MCP server fails')
    parser.add_argument('--cases', nargs='+', choices=['none','fast','slow','failed'], default=['none','fast','slow','failed'])
    parser.add_argument('--timeout', type=float, default=10)
    args = parser.parse_args()
    if args.trials < 1 or args.timeout <= 0:
        parser.error('trials and timeout must be positive')
    root = args.source.resolve()
    output = args.output.resolve()
    if output.is_relative_to(ROOT) or output.is_relative_to(root): parser.error('output must be outside source checkout')
    output.mkdir(parents=True, exist_ok=False)
    before = source_snapshot(root)
    commit = subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
    report = dict(status='running', commit=commit, source=before, source_checkout=str(root),
                  runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  source_status=subprocess.check_output(['git','status','--porcelain'],cwd=root,text=True),
                  recorded_at=datetime.now(timezone.utc).isoformat(),
                  harness=json.loads(subprocess.check_output(['go','list','-m','-json','github.com/sausheong/harness'],cwd=root,text=True)),
                  cpu_count=os.cpu_count(), processor=platform.processor(),
                  platform=platform.platform(), machine=platform.machine(),
                  toolchain=subprocess.check_output(['go','version'],text=True).strip(),
                  terminal=dict(rows=40, columns=120, term='xterm-256color'), trials=[],
                  method='launch to application-rendered input marker with PTY echo disabled',
                  memory_method='wait4 ru_maxrss at process exit; OS process accounting, not aggregate live process-tree RSS',
                  optional_mcp=args.optional_mcp,
                  limitation='Repeated fresh-process input readiness; no model requests or process-tree leak qualification. Required-MCP and optional-MCP cases have distinct failure expectations.')
    if sys.platform == 'darwin':
        report['hardware'] = {key: subprocess.check_output(['sysctl','-n',key],text=True).strip()
                              for key in ['machdep.cpu.brand_string','hw.memsize']}
    (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
    with (output/'build.log').open('w') as log:
        subprocess.run(['go','build','-ldflags=-X main.version=benchmark -X main.buildCommit='+commit+'-development',
                        '-o',str(output/'hand'),'./cmd/hand'],cwd=root,stdout=log,stderr=log,check=True)
        subprocess.run(['go','build','-o',str(output/'mcp-fixture'),'./internal/agentio/testdata/mcpserver'],
                       cwd=root,stdout=log,stderr=log,check=True)
    report['binary_bytes'] = (output/'hand').stat().st_size
    report['binary_sha256'] = hashlib.sha256((output/'hand').read_bytes()).hexdigest()
    for case in args.cases:
        for index in range(args.trials):
            result = trial(output/'hand',output/'mcp-fixture',case,output,index,args.timeout,args.optional_mcp)
            report['trials'].append(result)
            (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
            print(json.dumps(result),flush=True)
            if not result['observation_valid']:
                report['status']='failed'
                (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
                return 1
    report['summary']={case: dict(
        startup_p95_ms=percentile([t['startup_to_input_ms'] for t in report['trials'] if t['case']==case and t['startup_to_input_ms'] is not None],.95),
        max_rss_bytes=max(t['max_rss_bytes'] for t in report['trials'] if t['case']==case)) for case in args.cases}
    report['status']='measured' if source_snapshot(root)['sha256']==before['sha256'] else 'source_changed'
    (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
    return 0 if report['status']=='measured' else 1


if __name__=='__main__':
    raise SystemExit(main())

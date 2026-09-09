#!/usr/bin/env python3
"""Measure shipped Go/Python peer startup, repeated callbacks and resident memory."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import random
import selectors
import shutil
import signal
import subprocess
import tempfile
import time


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--go-extension', type=Path, required=True)
    p.add_argument('--python-source', type=Path, required=True)
    p.add_argument('--python-binary', type=Path, required=True)
    p.add_argument('--trials', type=int, default=30)
    p.add_argument('--out', type=Path, required=True)
    a = p.parse_args()
    assert 30 <= a.trials <= 100
    go, source, python = [x.resolve(strict=True) for x in [a.go_extension, a.python_source, a.python_binary]]
    out = a.out.resolve(); out.mkdir(parents=True, exist_ok=False)
    plan = ['go', 'python'] * a.trials; random.Random(709).shuffle(plan)
    records = []
    report = dict(status='failed', scope='direct reviewed example protocol; excludes Hand startup and container overhead',
                  platform=platform.platform(), machine=platform.machine(), trials_per_language=a.trials,
                  seed=709, page_cache='not flushed; warmed filesystem launches',
                  sources={str(x): sha(x) for x in [go, source, python, Path(__file__)]},
                  go_executable_bytes=go.stat().st_size, python_executable_bytes=python.stat().st_size,
                  python_script_bytes=source.stat().st_size,
                  memory_metric='RSS from ps after ten completed callbacks; not peak, PSS or exclusive memory')
    try:
        for language in plan:
            argv = [str(go)] if language == 'go' else [str(python), '-I', '-B', str(source)]
            with tempfile.TemporaryDirectory(prefix='hand-peer-measure-') as directory:
                start = time.perf_counter_ns()
                child = subprocess.Popen(argv, cwd=directory, env={'HOME': directory, 'PATH': '/usr/bin:/bin'},
                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, bufsize=0, start_new_session=True)
                selector = selectors.DefaultSelector(); selector.register(child.stdout, selectors.EVENT_READ)
                buffer = b''
                def send(frame):
                    child.stdin.write(json.dumps(frame).encode() + b'\n')
                def receive():
                    nonlocal buffer
                    deadline = time.monotonic() + 5
                    while b'\n' not in buffer:
                        assert selector.select(max(0, deadline-time.monotonic())), 'peer response timed out'
                        block = os.read(child.stdout.fileno(), 65536)
                        assert block, 'peer closed stdout'
                        buffer += block
                        assert len(buffer) <= 256*1024, 'frame overflow'
                    line, buffer = buffer.split(b'\n', 1)
                    frame = json.loads(line)
                    assert frame['version'] == 1 and not frame.get('error'), frame
                    return frame
                try:
                    send(dict(version=1, kind='request', id='init', method='initialize', params=dict(version=1)))
                    hello = receive(); assert hello['id'] == 'init' and hello['result']['name'] == 'task-note'
                    startup = (time.perf_counter_ns()-start)/1e6
                    latency = []
                    for n in range(10):
                        tick = time.perf_counter_ns(); request_id = 'command-' + str(n)
                        send(dict(version=1, kind='request', id=request_id, method='command.execute', params=dict(name='note', arguments='')))
                        callback = receive(); assert callback['kind'] == 'request' and callback['method'] == 'user.question'
                        send(dict(version=1, kind='response', id=callback['id'], result=dict(id=callback['params']['id'], cancelled=True)))
                        result = receive(); assert result['id'] == request_id and result['result']['blocks'][0]['text'] == 'Note cancelled.'
                        latency.append((time.perf_counter_ns()-tick)/1e6)
                    rss = subprocess.run(['/bin/ps', '-o', 'rss=', '-p', str(child.pid)], capture_output=True, text=True, check=True, timeout=5)
                    resident_kib = int(rss.stdout.strip())
                    tick = time.perf_counter_ns(); child.stdin.close(); assert child.wait(timeout=5) == 0
                    records.append(dict(language=language, argv=argv, startup_ms=startup, callback_ms=latency,
                                        rss_kib=resident_kib, shutdown_ms=(time.perf_counter_ns()-tick)/1e6))
                    (out / 'samples.json').write_text(json.dumps(records, indent=2)+'\n')
                finally:
                    if child.poll() is None:
                        os.killpg(child.pid, signal.SIGKILL); child.wait(timeout=5)
                    child.stdin.close(); child.stdout.close(); selector.close()
        def stats(values):
            values = sorted(values)
            return dict(n=len(values), median=values[len(values)//2], p95=values[math.ceil(len(values)*.95)-1], maximum=max(values))
        report['measurements'] = {}
        for language in ['go', 'python']:
            group = [r for r in records if r['language']==language]
            report['measurements'][language] = {key: stats([r[key] for r in group]) for key in ['startup_ms','rss_kib','shutdown_ms']}
            report['measurements'][language]['callback_ms'] = stats([x for r in group for x in r['callback_ms']])
        report.update(status='passed', samples_sha256=sha(out/'samples.json'))
    except Exception as exc:
        report['error'] = repr(exc)
    (out/'run.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status']=='passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

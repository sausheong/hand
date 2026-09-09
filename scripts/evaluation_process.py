"""Bounded local command execution; this does not authorise agent or model calls."""
from contextlib import ExitStack
import hashlib
import math
import os
from pathlib import Path
import selectors
import signal
import subprocess
import time


def run_command(argv, cwd, evidence, timeout_seconds, max_output_bytes, environment, cancelled=None):
    """Run in a new POSIX process group with exclusively created raw logs.

    Evidence must be outside the idle, caller-owned checkout. Environment is an
    explicit allowlist, never inherited implicitly. This is process ownership,
    not isolation: a malicious child can escape its group using setsid().
    """
    if not isinstance(argv, list) or not argv or any(not isinstance(a, str) or '\0' in a for a in argv) or not argv[0]:
        raise ValueError('argv must contain explicit nonempty executable and string arguments')
    if type(timeout_seconds) not in (int, float) or not 0 < timeout_seconds <= 86400 or not math.isfinite(timeout_seconds):
        raise ValueError('timeout must be finite and within one day')
    if type(max_output_bytes) is not int or not 0 < max_output_bytes <= 64 << 20:
        raise ValueError('output cap must be 1-67108864 bytes')
    if not isinstance(environment, dict) or any(not isinstance(k, str) or not k or '=' in k or '\0' in k
            or not isinstance(v, str) or '\0' in v for k, v in environment.items()):
        raise ValueError('environment must be an explicit string mapping')
    cwd, evidence = Path(cwd), Path(evidence)
    if (not cwd.is_absolute() or cwd != cwd.resolve() or not cwd.is_dir()
            or not evidence.is_absolute() or evidence != evidence.resolve()
            or evidence == cwd or cwd in evidence.parents):
        raise ValueError('canonical workspace and separate evidence directory required')
    if cancelled is not None and cancelled.is_set():
        raise InterruptedError('command cancelled before launch')
    evidence.mkdir(mode=0o700)
    paths = {name: evidence/(name+'.log') for name in ['stdout', 'stderr']}
    start = time.monotonic()
    outcome, total, cleanup_deadline, joined = None, 0, None, False
    process = None
    group_signalled = False

    def kill_group():
        nonlocal group_signalled
        if process is not None and not group_signalled:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            group_signalled = True

    with ExitStack() as stack:
        streams = {name: stack.enter_context(path.open('xb')) for name, path in paths.items()}
        selector = stack.enter_context(selectors.DefaultSelector())
        try:
            process = subprocess.Popen(argv, cwd=cwd, env=environment, stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            for name in streams:
                pipe = getattr(process, name)
                stack.callback(pipe.close)
                os.set_blocking(pipe.fileno(), False)
                selector.register(pipe, selectors.EVENT_READ, name)
            while True:
                now = time.monotonic()
                code = process.poll()
                if outcome is None:
                    if cancelled is not None and cancelled.is_set():
                        outcome = 'cancelled'
                    elif now-start >= timeout_seconds:
                        outcome = 'timed_out'
                    elif code is not None:
                        outcome = 'completed' if code == 0 else 'failed'
                    if outcome is not None:
                        kill_group()
                        cleanup_deadline = now + 2
                if outcome is not None and code is not None and not selector.get_map():
                    joined = True
                    break
                if cleanup_deadline is not None and now >= cleanup_deadline:
                    outcome = 'cleanup_failed'
                    break
                for key, _ in selector.select(0.02):
                    data = os.read(key.fileobj.fileno(), 65536)
                    if not data:
                        selector.unregister(key.fileobj)
                        continue
                    available = max_output_bytes-total
                    streams[key.data].write(data[:available])
                    total += min(available, len(data))
                    if len(data) > available and outcome not in ('timed_out', 'cancelled', 'cleanup_failed'):
                        outcome = 'output_limited'
                        kill_group()
                        if cleanup_deadline is None:
                            cleanup_deadline = time.monotonic()+2
            kill_group()
            process.wait(timeout=2)
            for stream in streams.values():
                stream.flush()
                os.fsync(stream.fileno())
        finally:
            try:
                kill_group()
            finally:
                if process is not None:
                    # A failed group signal must not skip direct-child cleanup.
                    # The original error still propagates; this cannot certify
                    # descendant cleanup when the host denied it.
                    if process.poll() is None:
                        process.kill()
                    process.wait(timeout=2)
    return dict(status=outcome, exit_code=process.returncode, argv=argv,
                cwd=str(cwd), elapsed_seconds=time.monotonic()-start,
                output_bytes=total, process_and_pipes_joined=joined,
                timeout_seconds=timeout_seconds, max_output_bytes=max_output_bytes,
                logs={name: dict(path=str(path), sha256=hashlib.sha256(path.read_bytes()).hexdigest())
                      for name, path in paths.items()})

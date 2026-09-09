#!/usr/bin/env python3
"""Exercise structured task state through a real PTY and inspect its journal."""
import argparse
import errno
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import select
import struct
import subprocess
import termios
import time
from check_rpc_client import Client


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    binary, out = args.hand_binary.resolve(strict=True), args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    work, home = out / 'workspace', out / 'home'
    work.mkdir(mode=0o700)
    home.mkdir(mode=0o700)
    master, slave = pty.openpty()
    attrs = termios.tcgetattr(slave)
    attrs[3] &= ~termios.ECHO
    termios.tcsetattr(slave, termios.TCSANOW, attrs)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 45, 140, 0, 0))
    command = [str(binary), '--model', 'local/fixture', '--base-url', 'http://127.0.0.1:1/v1']
    child = subprocess.Popen(command, cwd=work,
                             env={'HOME': str(home), 'PATH': '/usr/bin:/bin', 'TERM': 'xterm-256color'},
                             stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    def send_line(line):
        # Submit separately from text to avoid treating batched input as paste.
        os.write(master, line.encode())
        time.sleep(.05)
        os.write(master, b'\r')

    steps = [
        ('/state', 'Structured task state: 0 items, revision 0'),
        ('/state set', 'Usage: /state'),
        ('/state set goal objective Deliver migration', 'Structured task state updated.'),
        ('/state set choice decision Retain old reader', 'Structured task state updated.'),
        ('/state set pending unresolved_work Test rollback', 'Structured task state updated.'),
        ('/state evidence tests evidence/run.json#abc Unit tests at abc', 'Structured task state updated.'),
        ('/state remove pending', 'Structured task state updated.'),
        ('/state', 'Structured task state: 3 items, revision 5'),
    ]
    raw = bytearray()
    report = {'status': 'failed', 'qualification': 'development', 'command': command,
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'terminal_echo_disabled': True, 'paid_calls': 0, 'model_prompts_sent': 0, 'terminal_size': [140, 45]}
    completed = []
    step, marker, waiting = 0, 0, False
    started = time.monotonic()
    try:
        while time.monotonic() - started < 30:
            if select.select([master], [], [], .05)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError as exc:
                    if exc.errno == errno.EIO:
                        break
                    raise
                if not chunk:
                    break
                raw.extend(chunk)
                if len(raw) > 4 << 20:
                    raise RuntimeError('terminal output exceeded 4 MiB')
                if b'\x1b[6n' in chunk:
                    os.write(master, b'\x1b[1;1R')
                if b'\x1b]11;?' in chunk:
                    os.write(master, b'\x1b]11;rgb:0000/0000/0000\x1b\\')
            if time.monotonic() - started < 2:
                continue
            text = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(raw[marker:])).decode(errors='replace')
            if waiting and step < len(steps) and steps[step][1] in text:
                completed.append(steps[step][0])
                step += 1
                waiting = False
                if step == len(steps):
                    send_line('/quit')
                    waiting = False
            if not waiting and step < len(steps):
                marker = len(raw)
                send_line(steps[step][0])
                waiting = True
            if child.poll() is not None:
                break
        child.wait(timeout=5)
        assert len(completed) == len(steps), f'command journey incomplete at step {step}'
        assert child.returncode == 0, f'CLI exited {child.returncode}'
        rendered = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(raw)).decode(errors='replace')
        for expected in ['objective [goal]: Deliver migration',
                         'decision [choice]: Retain old reader',
                         'Evidence locator: evidence/run.json#abc',
                         'Evidence references do not establish current verification']:
            assert expected in rendered, 'missing rendered state detail: ' + expected
        records = []
        for path in (home / '.hand/sessions/hand').glob('*.jsonl'):
            for line in path.read_text().splitlines():
                record = json.loads(line)
                if record.get('type') == 'annotation':
                    records.append(record['data'])
        def entries(kind):
            return [r['payload'] for r in records if r['kind'] == kind]
        states = entries('harness.context_state')
        assert len(states) == 5, 'invalid command or inspection wrote state'
        assert states[-1] == {'version': 1, 'items': [
            {'id': 'goal', 'kind': 'objective', 'text': 'Deliver migration'},
            {'id': 'choice', 'kind': 'decision', 'text': 'Retain old reader'},
            {'id': 'tests', 'kind': 'verification_reference', 'text': 'Unit tests at abc', 'reference': 'evidence/run.json#abc'},
        ]}, states[-1]
        # A second built Hand process must read the state through its public API.
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'restarted')
        try:
            client.call('hello', 'hello')
            restored = client.call('restored-state', 'context.state')
            assert restored['revision'] == 5 and restored['items'] == states[-1]['items'], restored
        finally:
            client.close()
        report.update(restarted_rpc_state=restored,
                      client_sha256=hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest())
        report.update(status='development_passed', completed_commands=completed, exit_code=child.returncode,
                      persisted_revisions=len(states), final_state=states[-1])
    except Exception as exc:
        report['error'] = repr(exc)
        report['completed_commands'] = completed
        report['pending_step'] = step
    finally:
        if child.poll() is None:
            child.kill()
            child.wait(timeout=5)
        os.close(master)
        (out / 'terminal.bin').write_bytes(raw)
        report['terminal_sha256'] = hashlib.sha256(raw).hexdigest()
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'development_passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

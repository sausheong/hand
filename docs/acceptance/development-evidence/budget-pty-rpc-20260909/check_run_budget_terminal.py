#!/usr/bin/env python3
"""Exercise real PTY run-budget decisions and inspect their durable journal."""
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
        # Separate typing and submission so Enter cannot be swallowed as paste.
        os.write(master, line.encode())
        time.sleep(.05)
        os.write(master, b'\r')

    steps = [
        ('/run-budget', 'No run budget selected.'),
        ('/run-budget tokens repair 0', 'Usage: /run-budget'),
        ('/run-budget tokens repair 100000', 'Run repair absolute token ceiling: 100000'),
        ('/run-budget cost repair USD 1.000000001 strict', 'Run repair absolute cost ceiling: USD 1.000000001'),
        ('/run-budget time repair 600', 'Run repair new wall-clock allowance: 600 seconds'),
        ('/run-budget select repair', 'Selected run budget repair.'),
        ('/run-budget', 'Run budget repair: selected'),
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
        for expected in ['Run budget repair: selected; configured: true',
                         'Tokens: absolute ceiling 100000; committed 0',
                         'Cost: USD ceiling 1.000000001; committed 0',
                         'Deadline:', 'expired: false']:
            assert expected in rendered, 'missing rendered budget detail: ' + expected
        records = []
        for path in (home / '.hand/sessions/hand').glob('*.jsonl'):
            for line in path.read_text().splitlines():
                record = json.loads(line)
                if record.get('type') == 'annotation':
                    records.append(record['data'])
        def entries(kind):
            return [r['payload'] for r in records if r['kind'] == kind]
        tokens = entries('harness.budget.tokens.v1.run.repair')
        costs = entries('harness.budget.cost.v1.run.repair')
        deadlines = entries('harness.budget.deadline.v1.run.repair')
        selected = entries('hand.budget.run.v1')
        assert len(tokens) == 1 and tokens[0]['tokens'] == 100000, 'invalid decision persisted or tokens changed'
        assert len(costs) == 1 and costs[0]['limit_nano'] == 1000000001 and costs[0]['strict']
        assert len(deadlines) == 1, 'inspection renewed deadline'
        assert selected == [{'version': 1, 'id': 'repair'}]
        journals = {path: path.read_bytes() for path in (home / '.hand/sessions/hand').glob('*.jsonl')}
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'restarted')
        try:
            client.call('hello', 'hello')
            restored = client.call('restored-budget', 'budget.run')
            assert restored['id'] == 'repair' and restored['selected'] and restored['configured'], restored
            assert restored['tokens']['limit'] == 100000 and restored['tokens']['committed'] == 0
            assert restored['cost']['limit_nano'] == 1000000001 and restored['cost']['currency'] == 'USD' and restored['cost']['strict']
            assert restored['time']['deadline'] == deadlines[0]['deadline'], 'restart renewed allowance'
        finally:
            client.close()
        assert all(path.read_bytes() == raw for path, raw in journals.items()), 'RPC inspection changed session journal'
        report.update(restarted_budget=restored, inspection_preserved_journal=True,
                      client_sha256=hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest())
        report.update(status='development_passed', completed_commands=completed, exit_code=child.returncode,
                      token_decision=tokens[0], cost_decision=costs[0], deadline=deadlines[0], selected=selected[0])
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

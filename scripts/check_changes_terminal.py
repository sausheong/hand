#!/usr/bin/env python3
"""Exercise /changes in a real PTY against a completed local RPC fixture."""
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


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--hand-binary', type=Path, required=True)
    p.add_argument('--fixture', type=Path, required=True)
    p.add_argument('--out', type=Path, required=True)
    p.add_argument('--restore', action='store_true', help='Preview and confirm a selected restore in the terminal')
    args = p.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    fixture = args.fixture.resolve(strict=True)
    previous = json.loads((fixture / 'run.json').read_text())
    assert previous['status'] == 'passed' and previous['checkpoint_changes']['status'] == 'passed'
    assert previous['binary_sha256'] == hashlib.sha256(binary.read_bytes()).hexdigest()
    transcript = json.loads((fixture / 'client-two.json').read_text())
    session = next(x['request']['params']['id'] for x in transcript if x['request']['method'] == 'session.select')
    run = previous['checkpoint_changes']['run_id']
    args.out.mkdir(parents=True, exist_ok=False)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 45, 140, 0, 0))
    env = {'HOME': str(fixture / 'home'), 'PATH': '/usr/bin:/bin', 'TERM': 'xterm-256color'}
    command = [str(binary), '--model', 'local/fixture', '--base-url', 'http://127.0.0.1:1/v1',
               '--session', session, '--checkpoint-dir', str(fixture / 'checkpoints')]
    child = subprocess.Popen(command, cwd=fixture / 'workspace', env=env,
                             stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    raw = bytearray()
    started = time.monotonic()
    sent = False
    quit_sent = False
    found = False
    confirmed = False
    report = {'status': 'failed', 'qualification': 'development', 'command': command,
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    try:
        while time.monotonic() - started < 20:
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
            if not sent and time.monotonic() - started > 2:
                command_text = '/restore-preview ' + run + ' approved.txt' if args.restore else '/changes ' + run
                os.write(master, (command_text + '\r').encode())
                sent = True
            text = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(raw)).decode(errors='replace')
            if args.restore and not confirmed and 'would remove approved.txt' in text:
                match = re.search(r'/restore-confirm ([0-9a-f]{64})', text)
                if match:
                    assert (fixture / 'workspace' / 'approved.txt').read_bytes() == b'approved once', 'preview changed the file'
                    os.write(master, ('/restore-confirm ' + match.group(1) + '\r').encode())
                    confirmed = True
            if (args.restore and confirmed and 'Selected restore completed.' in text) or (not args.restore and 'added approved.txt' in text and 'Recorded before/after changes' in text):
                found = True
                if not quit_sent:
                    os.write(master, b'/quit\r')
                    quit_sent = True
            if child.poll() is not None:
                break
        if child.poll() is None:
            child.wait(timeout=5)
        assert found, 'expected checkpoint change display absent'
        assert child.returncode == 0, f'terminal exited {child.returncode}'
        if args.restore:
            assert not (fixture / 'workspace' / 'approved.txt').exists(), 'confirmed restore did not remove file'
            events = [json.loads(line)['event'] for line in (fixture / 'checkpoints' / 'restore-journal.jsonl').read_text().splitlines()]
            assert len(events) == 2 and [e['phase'] for e in events] == ['prepared', 'applied']
            recovery = fixture / 'workspace' / events[0]['recovery_name']
            assert recovery.read_bytes() == b'approved once', 'displaced file content missing'
            report.update(status='passed', preview_preserved_file=True, confirmed=True, removed_selected=True, recovery_preserved=True, exit_code=child.returncode)
        else:
            report.update(status='passed', rendered_change='added approved.txt', exit_code=child.returncode)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if child.poll() is None:
            child.kill()
            child.wait(timeout=5)
        os.close(master)
        (args.out / 'terminal.bin').write_bytes(raw)
        report['terminal_sha256'] = hashlib.sha256(raw).hexdigest()
        (args.out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

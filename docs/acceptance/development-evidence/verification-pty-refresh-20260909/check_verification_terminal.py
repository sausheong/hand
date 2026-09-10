#!/usr/bin/env python3
"""Exercise verification confirmation and stale reassessment in a real PTY."""
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
    p.add_argument('--out', type=Path, required=True)
    args = p.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    args.out = args.out.absolute()
    args.out.mkdir(parents=True, exist_ok=False)
    fixture = args.out / 'fixture'
    fixture.mkdir(mode=0o700)
    for name in ['workspace', 'home', 'evidence']:
        (fixture / name).mkdir(mode=0o700)
    (fixture / 'workspace' / 'source').write_text('original source')
    marker = fixture / 'command-executions'
    config = fixture / 'verification.json'
    config.write_text(json.dumps({'directory':str(fixture / 'evidence'), 'profiles':[{'name':'unit','command':['/bin/sh','-c','printf x >> "$1"; cat source', 'verify', str(marker)]}]}))
    master, slave = pty.openpty()
    # Assertions must observe application rendering, not echoed input.
    attrs = termios.tcgetattr(slave)
    attrs[3] &= ~termios.ECHO
    termios.tcsetattr(slave, termios.TCSANOW, attrs)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 45, 140, 0, 0))
    env = {'HOME': str(fixture / 'home'), 'PATH': '/usr/bin:/bin', 'TERM': 'xterm-256color'}
    command = [str(binary), '--model', 'local/fixture', '--base-url', 'http://127.0.0.1:1/v1',
               '--checkpoint-dir', str(fixture / 'checkpoints'), '--verification-config', str(config)]
    child = subprocess.Popen(command, cwd=fixture / 'workspace', env=env,
                             stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    def send_line(line):
        # Separate typing from Enter so a batched read is not treated as paste.
        os.write(master, line.encode())
        time.sleep(.05)
        os.write(master, b'\r')

    raw = bytearray()
    started = time.monotonic()
    sent = False
    quit_sent = False
    found = False
    confirmed = False
    rechecked = False
    evidence_id = None
    listed = False
    deletion_sent = False
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
                send_line('/verify unit')
                sent = True
            text = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(raw)).decode(errors='replace')
            if not confirmed:
                match = re.search(r'/verify-confirm unit ([0-9a-f]{64})', text)
                if match:
                    assert not marker.exists(), 'review executed command'
                    assert not list((fixture / 'evidence').glob('*.json')), 'review saved evidence'
                    send_line('/verify-confirm unit ' + match.group(1))
                    confirmed = True
            if confirmed and not rechecked and 'Verification unit: passed (exit 0)' in text:
                match = re.search(r'Evidence: ([0-9a-f]{64})', text)
                if match:
                    evidence_id = match.group(1)
                    record = fixture / 'evidence' / (evidence_id + '.json')
                    assert hashlib.sha256(record.read_bytes()).hexdigest() == evidence_id
                    (fixture / 'workspace' / 'source').write_text('later user edit')
                    send_line('/verify-check unit ' + evidence_id)
                    rechecked = True
            if rechecked and not listed and 'Verification unit: stale (exit 0)' in text:
                send_line('/verify-list')
                listed = True
            if listed and not deletion_sent and 'Saved verification evidence: 1 records' in text:
                send_line('/verify-delete ' + evidence_id + ' confirm')
                deletion_sent = True
            if deletion_sent and 'Deleted verification record ' + evidence_id in text:
                found = True
                if not quit_sent:
                    send_line('/quit')
                    quit_sent = True
            if child.poll() is not None:
                break
        if child.poll() is None:
            child.wait(timeout=5)
        assert found, 'verification pass/stale terminal journey incomplete'
        assert child.returncode == 0, f'terminal exited {child.returncode}'
        assert evidence_id and (fixture / 'workspace' / 'source').read_text() == 'later user edit'
        assert not (fixture / 'evidence' / (evidence_id + '.json')).exists(), 'record deletion failed'
        assert list((fixture / 'checkpoints').glob('*.json')), 'cleanup removed snapshots'
        assert marker.read_text() == 'x', 'verification command did not execute exactly once'
        report.update(deleted_selected=True, snapshots_retained=True, command_executions=1, terminal_echo_disabled=True)
        report.update(status='passed', reviewed=True, confirmed=True, saved_id=evidence_id, later_edit_stale=True, exit_code=child.returncode, provider_calls=0)
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

#!/usr/bin/env python3
"""Exercise reviewed extension questions through a real PTY and inspect state."""
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
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--go-extension', type=Path, required=True)
    parser.add_argument('--execution-config', type=Path, help='Explicit container fixture configuration')
    args = parser.parse_args()
    binary, out = args.hand_binary.resolve(strict=True), args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    work, home = out / 'workspace', out / 'home'
    work.mkdir(mode=0o700)
    home.mkdir(mode=0o700)
    peer = args.go_extension.resolve(strict=True)
    snapshots = out / 'snapshots'
    request, review = out / 'input.json', out / 'review.json'
    requested = dict(version=1, snapshot_root=str(snapshots), extensions=[dict(identity='examples/terminal-note', launch=dict(name='task-note', executable=str(peer), workspace=str(work), capabilities=['commands', 'questions', 'state', 'context.transform']))])
    execution = None
    before_containers = set()
    container_commands = []
    def containers():
        docker_config = out / 'docker-config'; docker_config.mkdir(mode=0o700, exist_ok=True)
        argv = [execution['docker'], '--config', str(docker_config), '--host', 'unix://' + execution['socket'], 'ps', '-aq', '--filter', 'name=harness-exec-']
        result = subprocess.run(argv, capture_output=True, text=True, timeout=10, env={'HOME': str(home), 'PATH': '/usr/bin:/bin'})
        container_commands.append(dict(argv=argv, exit_code=result.returncode, stdout=result.stdout, stderr=result.stderr))
        (out / 'container-commands.json').write_text(json.dumps(container_commands, indent=2) + '\n')
        assert result.returncode == 0, container_commands[-1]
        return set(result.stdout.split())
    if args.execution_config:
        execution = json.loads(args.execution_config.read_text())
        assert execution['backend'] == 'container'
        configdir = home / '.hand'; configdir.mkdir(mode=0o700)
        (configdir / 'config.json').write_text(json.dumps(dict(execution=execution)))
        requested['container'] = {key: execution[key] for key in ['docker', 'socket', 'image', 'writable', 'network']}
        before_containers = containers()
    request.write_text(json.dumps(requested))
    generated = subprocess.run([str(binary), '--review-extensions', str(request), '--extension-review-output', str(review)], capture_output=True, timeout=15)
    (out / 'review.stderr').write_bytes(generated.stderr)
    if generated.returncode != 0:
        raise RuntimeError(generated.stderr.decode())
    digest = generated.stdout.decode().strip()
    assert digest == hashlib.sha256(review.read_bytes()).hexdigest()
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 45, 140, 0, 0))
    command = [str(binary), '--model', 'local/fixture', '--base-url', 'http://127.0.0.1:1/v1', '--extension-config', str(review), '--approve-extension-config', digest]
    child = subprocess.Popen(command, cwd=work,
                             env={'HOME': str(home), 'PATH': '/usr/bin:/bin', 'TERM': 'xterm-256color'},
                             stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    steps = [
        ('/extension task-note note', 'What task note should be remembered?'),
        ('Remember the native terminal', 'Task note saved.'),
        ('/extension task-note note', 'What task note should be remembered?'),
        ('ESC', 'Note cancelled.'),
        ('/extension task-note note', 'What task note should be remembered?'),
        ('CTRL_C', 'session operation failed:'),
        (f'/reload {review} {digest}', 'Extension reload committed=true; started=1 reused=0 removed=0'),
        ('/extension task-note note', 'What task note should be remembered?'),
        ('ESC', 'Note cancelled.'),
        ('/state', 'Structured task state:'),
    ]
    raw = bytearray()
    report = {'status': 'failed', 'qualification': 'development', 'command': command,
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'paid_calls': 0, 'model_prompts_sent': 0, 'terminal_size': [140, 45], 'peer_sha256': hashlib.sha256(peer.read_bytes()).hexdigest(), 'review_digest': digest}
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
                    os.write(master, b'/quit\r')
                    waiting = False
            if not waiting and step < len(steps):
                marker = len(raw)
                value = steps[step][0]
                os.write(master, b'\x1b' if value == 'ESC' else b'\x03' if value == 'CTRL_C' else (value + '\r').encode())
                waiting = True
            if child.poll() is not None:
                break
        child.wait(timeout=5)
        assert len(completed) == len(steps), f'command journey incomplete at step {step}'
        assert child.returncode == 0, f'CLI exited {child.returncode}'
        rendered = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(raw)).decode(errors='replace')
        records = []
        for path in (home / '.hand/sessions/hand').glob('*.jsonl'):
            for line in path.read_text().splitlines():
                record = json.loads(line)
                if record.get('type') == 'annotation':
                    records.append(record['data'])
        def entries(kind):
            return [r['payload'] for r in records if r['kind'] == kind]
        states = [r['payload'] for r in records if r['kind'].startswith('hand.extension.state.')]
        assert states == [{'version': 1, 'data': {'note': 'Remember the native terminal'}}], states
        assert not snapshots.exists() or not list(snapshots.iterdir()), 'extension snapshot survived terminal shutdown'
        if execution:
            assert not (containers() - before_containers), 'extension container survived terminal shutdown'
            report['container_execution'] = execution
            report['container_cleanup'] = True
        report.update(status='development_passed', completed_commands=completed, exit_code=child.returncode,
                      persisted_revisions=len(states), final_state=states[-1], snapshot_cleanup=True)
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

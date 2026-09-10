#!/usr/bin/env python3
"""Build and execute Linux RPC acceptance tests with an explicit pinned image."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import uuid
from evidence import source_snapshot


def dependency_snapshot(directory):
    """Fingerprint local worktrees and immutable module-cache directories alike."""
    directory = Path(directory).resolve(strict=True)
    entries = []
    for path in sorted(directory.rglob('*')):
        relative = path.relative_to(directory)
        if '.git' in relative.parts:
            continue
        if path.is_symlink():
            payload, mode = os.readlink(path).encode(), 'symlink'
        elif path.is_file():
            payload, mode = path.read_bytes(), oct(path.stat().st_mode & 0o777)
        else:
            continue
        entries.append(dict(path=str(relative), mode=mode, sha256=hashlib.sha256(payload).hexdigest()))
    return dict(directory=str(directory), entries=entries,
                sha256=hashlib.sha256(json.dumps(entries, sort_keys=True, separators=(',', ':')).encode()).hexdigest())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True, help='Existing Docker sha256 image ID')
    parser.add_argument('--docker', default='docker')
    parser.add_argument('--arch', choices=['arm64', 'amd64'], required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image):
        parser.error('image must be an immutable sha256 image ID')
    root = Path(__file__).resolve().parent.parent
    out = args.out.absolute()
    if out == root or root in out.parents:
        parser.error('output must be outside the checkout to keep fingerprints stable')
    out.mkdir(parents=True, exist_ok=False)
    report = dict(status='failed', qualification='development', image=args.image,
                  architecture=args.arch, race_instrumented=False, commands=[])
    report['source_before'] = source_snapshot(root)
    env = {**os.environ, 'GOOS': 'linux', 'GOARCH': args.arch, 'CGO_ENABLED': '0'}

    def command(argv, label, timeout=180):
        started = time.monotonic()
        with (out / (label + '.log')).open('wb') as stream:
            result = subprocess.run(argv, cwd=root, env=env, stdout=stream,
                                    stderr=subprocess.STDOUT, timeout=timeout)
        report['commands'].append(dict(argv=argv, exit_code=result.returncode,
                                       seconds=time.monotonic()-started, log=label+'.log'))
        if result.returncode:
            raise RuntimeError(label + ' failed; inspect raw log')
        return (out / (label + '.log')).read_text()

    def container(label, flags, test_binary="rpc.test"):
        name = 'hand-rpc-acceptance-' + uuid.uuid4().hex
        argv = [args.docker, 'run', '--rm', '--name', name, '--network', 'none',
                '--read-only', '--tmpfs', '/tmp:rw,exec,size=128m,mode=1777',
                '--mount', f'type=bind,source={out / test_binary},target=/test,readonly',
                '--mount', f'type=bind,source={out / "note"},target=/note,readonly',
                '--mount', f'type=bind,source={out / "hand"},target=/hand,readonly',
                '-e', 'HAND_TEST_RPC_NOTE_BINARY=/note',
                '-e', 'HAND_TEST_RPC_FRAME_BINARY=/hand', '--entrypoint', '/test', args.image] + flags
        try:
            return command(argv, label, 240)
        finally:
            # Timeout must not leave the runner's container behind.
            subprocess.run([args.docker, 'rm', '-f', name], stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=15)

    try:
        module = json.loads(command(['go', 'list', '-m', '-json', 'github.com/sausheong/harness'], 'harness-module'))
        dependency = Path(module['Dir']).resolve(strict=True)
        report['harness_module'] = module
        report['harness_before'] = dependency_snapshot(dependency)
        report['go_environment'] = json.loads(command(['go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'GOWORK'], 'go-environment'))
        command(['go', 'test', '-c', '-o', str(out / 'rpc.test'), './internal/rpc'], 'build-tests')
        command(['go', 'build', '-o', str(out / 'note'), './examples/extensions/go-task-note'], 'build-note')
        command(['go', 'build', '-o', str(out / 'hand'), './cmd/hand'], 'build-hand')
        command(['go', 'test', '-c', '-o', str(out / 'hand.test'), './cmd/hand'], 'build-hand-tests')
        report['binaries'] = {name: hashlib.sha256((out/name).read_bytes()).hexdigest()
                              for name in ['rpc.test', 'note', 'hand', 'hand.test']}
        listing = container('inventory', ['-test.list', '^Test'])
        inventory = [line for line in listing.splitlines() if line.startswith('Test')]
        if not inventory or len(set(inventory)) != len(inventory):
            raise RuntimeError('empty or duplicate test inventory')
        full = container('full', ['-test.v', '-test.timeout=3m'])
        passed = re.findall(r'^--- PASS: (\S+)', full, re.M)
        if set(passed) != set(inventory) or '--- FAIL:' in full or '--- SKIP:' in full or not full.rstrip().endswith('PASS'):
            raise RuntimeError('full test results do not match inventory')
        name = 'TestSlowConsumerReconnectRecoversExactTerminalWithoutReplay'
        repeated = container('repeated', ['-test.v', '-test.timeout=3m', '-test.count=20', '-test.run', '^'+name+'$'])
        if re.findall(r'^--- PASS: (\S+)', repeated, re.M) != [name]*20 or '--- FAIL:' in repeated or '--- SKIP:' in repeated:
            raise RuntimeError('required 20 repetitions did not pass')
        report.update(inventory=inventory, full_test_records=len(re.findall(r'^\s*--- PASS:', full, re.M)), repetitions=20)
        framing_name = 'TestBinaryRPCRejectsOversizedFrameBeforeProvider'
        framing = container('framing', ['-test.v', '-test.timeout=1m', '-test.run', '^'+framing_name+'$'], 'hand.test')
        if re.findall(r'^--- PASS: (\S+)', framing, re.M) != [framing_name] or '--- FAIL:' in framing or '--- SKIP:' in framing or not framing.rstrip().endswith('PASS'):
            raise RuntimeError('built Hand framing regression did not pass')
        report['built_client_framing_passed'] = True
        report['source_after'] = source_snapshot(root)
        report['harness_after'] = dependency_snapshot(dependency)
        if report['harness_before'] != report['harness_after']:
            raise RuntimeError('Harness source changed during qualification')
        if report['source_before'] != report['source_after']:
            raise RuntimeError('source changed during qualification')
        report['status'] = 'development_passed'
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        report['artifacts'] = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                               for p in out.iterdir() if p.is_file() and p.name != 'run.json'}
        (out/'run.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({k: report[k] for k in ['status', 'full_test_records', 'repetitions', 'error'] if k in report}))
    return 0 if report['status'] == 'development_passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

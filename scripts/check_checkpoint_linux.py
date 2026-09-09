#!/usr/bin/env python3
"""Run Linux checkpoint tests without privileged access or a Docker socket mount."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import uuid
from check_linux_full import summarise_go_log
from check_transcript_performance import snapshot

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image):
        parser.error('pinned local image ID required')
    out = args.output.resolve()
    if out == ROOT or ROOT in out.parents:
        parser.error('output must be outside checkout')
    out.mkdir(parents=True, exist_ok=False)
    name = 'hand-checkpoint-' + uuid.uuid4().hex
    report = dict(status='failed', commands=[], limitations=[
        'Linux arm64 checkpoint package only, not full Linux qualification',
        'Cross-compiled without race instrumentation; runtime executed in container',
        'No Docker socket mounted, no network, non-root user'])
    before = snapshot(ROOT)
    env = dict(os.environ, GOOS='linux', GOARCH='arm64', CGO_ENABLED='0')

    def run(argv, label, timeout=300):
        start = time.monotonic()
        with (out/(label+'.log')).open('wb') as stream:
            result = subprocess.run(argv, cwd=ROOT, env=env, stdout=stream,
                                    stderr=subprocess.STDOUT, timeout=timeout)
        report['commands'].append(dict(argv=argv, exit_code=result.returncode, seconds=time.monotonic()-start))
        if result.returncode:
            raise RuntimeError(label+' failed')

    try:
        run(['docker', 'image', 'inspect', args.image], 'image')
        info = json.loads((out/'image.log').read_text())[0]
        if info['Os'] != 'linux' or info['Architecture'] != 'arm64':
            raise ValueError('Linux arm64 image required')
        run(['go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED'], 'toolchain')
        run(['go', 'test', '-c', '-covermode=atomic', '-coverpkg=./internal/checkpoints', '-o', str(out/'checkpoint.test'), './internal/checkpoints'], 'build')
        report['binary_sha256'] = hashlib.sha256((out/'checkpoint.test').read_bytes()).hexdigest()
        argv = ['go', 'tool', 'test2json', '-t', '-p', 'github.com/sausheong/hand/internal/checkpoints',
                'docker', 'run', '--rm', '--name', name, '--network', 'none', '--read-only',
                '--user', str(os.getuid())+':'+str(os.getgid()), '--tmpfs', '/tmp:rw,exec,size=256m,mode=1777',
                '--mount', f'type=bind,source={out},target=/evidence', '--entrypoint', '/evidence/checkpoint.test',
                args.image, '-test.v=test2json', '-test.count=1', '-test.timeout=120s', '-test.coverprofile=/evidence/coverage.out']
        run(argv, 'suite', 180)
        summary = summarise_go_log((out/'suite.log').read_text())
        report.update(summary)
        if not summary['test_records'].get('pass') or any(summary['test_records'].get(k) for k in ('fail', 'skip')) or summary['unfinished_tests']:
            raise RuntimeError('tests failed, skipped, absent or incomplete')
        report['status'] = 'development_passed'
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as exc:
        report['error'] = str(exc)
    finally:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
        report['source_before'], report['source_after'] = before, snapshot(ROOT)
        report['sources_unchanged'] = report['source_before'] == report['source_after']
        if not report['sources_unchanged']:
            report['status'] = 'failed'
        (out/'report.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({k: report.get(k) for k in ('status', 'test_records', 'error', 'sources_unchanged')}))
    return int(report['status'] != 'development_passed')


if __name__ == '__main__':
    raise SystemExit(main())

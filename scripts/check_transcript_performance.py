#!/usr/bin/env python3
"""Run the required transcript workload with source and machine provenance."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import time

sys.dont_write_bytecode = True
from evidence import source_snapshot
from transcript_performance import summarise

ROOT = Path(__file__).resolve().parents[1]


def snapshot(root):
    if (root / '.git').exists():
        return source_snapshot(root)
    entries = []
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            payload, mode = os.readlink(path).encode(), 'symlink'
        elif path.is_file():
            payload, mode = path.read_bytes(), oct(path.stat().st_mode & 0o777)
        else:
            continue
        entries.append(dict(path=str(path.relative_to(root)), mode=mode, sha256=hashlib.sha256(payload).hexdigest()))
    if not entries:
        raise ValueError('empty dependency source inventory')
    raw = json.dumps(entries, sort_keys=True, separators=(',', ':')).encode()
    return dict(sha256=hashlib.sha256(raw).hexdigest(), entries=entries)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.is_relative_to(ROOT):
        parser.error('output must be outside the source checkout')
    output.mkdir(parents=True, exist_ok=False)
    report = dict(status='failed', commands=[], scope='In-process UI refresh, accepted key and View; excludes terminal display.')
    roots = []
    before = {}

    def run(argv, filename):
        with (output / filename).open('wb') as stdout, (output / (filename + '.stderr')).open('wb') as stderr:
            result = subprocess.run(argv, cwd=ROOT, stdout=stdout, stderr=stderr, timeout=1200)
        report['commands'].append(dict(argv=argv, exit_code=result.returncode, stdout=filename, stderr=filename+'.stderr'))
        if result.returncode:
            raise RuntimeError(f'{filename}: exit {result.returncode}')
        return (output / filename).read_text()

    try:
        report['commit'] = run(['git', 'rev-parse', 'HEAD'], 'commit.txt').strip()
        report['clean_source'] = not run(['git', 'status', '--porcelain'], 'status.txt').strip()
        report['toolchain'] = json.loads(run(['go', 'env', '-json', 'GOOS', 'GOARCH', 'GOVERSION', 'CGO_ENABLED', 'GOWORK', 'GOMOD'], 'go-env.json'))
        dependency = json.loads(run(['go', 'list', '-m', '-json', 'github.com/sausheong/harness'], 'harness-module.json'))
        report['harness_module'] = dependency
        harness = Path(dependency.get('Replace', dependency)['Dir']).resolve()
        roots = [ROOT, harness]
        # A released module has no Git checkout. Hash every module file there;
        # local development checkouts additionally include deleted/untracked files.
        for root in roots:
            before[str(root)] = snapshot(root)
        report['machine'] = dict(platform=platform.platform(), architecture=platform.machine(), logical_cpus=os.cpu_count())
        if platform.system() == 'Darwin':
            report['machine']['cpu'] = run(['sysctl', '-n', 'machdep.cpu.brand_string'], 'cpu.txt').strip()
            report['machine']['memory_bytes'] = int(run(['sysctl', '-n', 'hw.memsize'], 'memory.txt').strip())
        elif platform.system() == 'Linux':
            report['machine']['cpuinfo'] = Path('/proc/cpuinfo').read_text()
            report['machine']['meminfo'] = Path('/proc/meminfo').read_text()
        else:
            raise RuntimeError('unsupported performance reference platform')
        command = ['go', 'test', '-run', '^$', '-bench', '^BenchmarkTranscript10000Blocks$', '-benchtime=1000x', '-count=30', './internal/tui']
        start = time.monotonic()
        raw = run(command, 'benchmark.log')
        report['seconds'] = time.monotonic() - start
        report['measurement'] = summarise(raw)
        report['status'] = report['measurement']['status']
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        report['error'] = str(error)
    finally:
        after = {}
        for root in roots:
            try:
                after[str(root)] = snapshot(root)
            except (OSError, ValueError, subprocess.SubprocessError) as error:
                report['snapshot_error'] = str(error)
        report['sources_unchanged'] = bool(before) and before == after
        if not report['sources_unchanged']:
            report['status'] = 'failed'
        (output / 'sources.json').write_text(json.dumps(dict(before=before, after=after), indent=2)+'\n')
        report['artifacts'] = [dict(path=p.name, sha256=hashlib.sha256(p.read_bytes()).hexdigest()) for p in sorted(output.iterdir()) if p.is_file()]
        report['limitations'] = ['Development workload evidence only; final candidate, released Harness and full acceptance review remain required.']
        (output / 'report.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps(dict(status=report['status'], report=str(output/'report.json'), sources_unchanged=report['sources_unchanged'])))
    return 0 if report['status'] == 'development_workload_passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

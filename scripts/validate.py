#!/usr/bin/env python3
"""Offline validation. Records real commands and test events; never claims full acceptance."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
sys.dont_write_bytecode = True
from evidence import source_snapshot, coverage_metrics
from coverage_changes import changed_lines, changed_coverage, classify_changed_sources
from critical_coverage import critical_coverage

ROOT = Path(__file__).resolve().parents[1]


def check_events(events, expected):
    passed, skipped, failed = set(), set(), set()
    started = set()
    packages = set()
    for event in events:
        name, package = event.get('Test'), event.get('Package')
        action = event.get('Action')
        if name:
            key = (package, name)
            if action == 'run': started.add(key)
            if action == 'pass': passed.add(key)
            if action == 'skip': skipped.add(key)
            if action == 'fail': failed.add(key)
        elif action == 'pass':
            packages.add(package)
        elif action == 'fail':
            failed.add((package, '<package>'))
    missing = expected - passed
    return dict(passed=len(passed), skipped=sorted(skipped), failed=sorted(failed),
                missing=sorted(missing), packages=sorted(packages),
                unfinished=sorted(started - passed - skipped - failed),
                missing_packages=sorted({package for package, _ in expected} - packages))


def inventory_names(listing):
    # go test -list emits only runnable examples; fuzz targets run their seed
    # corpus during an ordinary test invocation. Benchmarking is separate.
    return {line for line in listing.splitlines()
            if re.fullmatch(r'(?:Test|Example|Fuzz)\w*', line)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--baseline', default='e6dd263', help='recorded acceptance baseline')
    args = parser.parse_args()
    output = args.output.resolve()
    if output.is_relative_to(ROOT):
        parser.error('evidence output must be outside the source checkout')
    output.mkdir(parents=True, exist_ok=False)
    os.environ['PYTHONDONTWRITEBYTECODE'] = '1'
    report = dict(status='failed', commands=[])
    def run(argv, filename):
        print('+ ' + ' '.join(argv), flush=True)
        with (output / filename).open('w') as log, (output / (filename + '.stderr')).open('w') as errors:
            result = subprocess.run(argv, cwd=ROOT, stdout=log, stderr=errors, text=True)
        report['commands'].append(dict(argv=argv, exit_code=result.returncode, log=filename,
                                       stderr_log=filename + '.stderr'))
        if result.returncode:
            raise RuntimeError(f'{filename}: command exited {result.returncode}')
        return (output / filename).read_text()
    try:
        report['commit'] = run(['git', 'rev-parse', 'HEAD'], 'commit.txt').strip()
        report['source_status'] = run(['git', 'status', '--porcelain'], 'source-status.txt')
        before = source_snapshot(ROOT)
        (output / 'source-files.json').write_text(json.dumps(before, indent=2) + '\n')
        report['source_sha256'] = before['sha256']
        report['clean_source'] = not bool(report['source_status'].strip())
        report['toolchain'] = run(['go', 'version'], 'go-version.txt').strip()
        env = run(['go', 'env', '-json', 'GOOS', 'GOARCH', 'GOVERSION'], 'go-env.json')
        report['platform'] = json.loads(env)
        files = sorted(str(p.relative_to(ROOT)) for p in ROOT.rglob('*.go')
                       if '.git' not in p.parts and 'vendor' not in p.parts)
        unformatted = run(['gofmt', '-l', *files], 'format.txt').strip()
        if unformatted: raise RuntimeError('unformatted Go files: ' + unformatted)
        run(['go', 'build', './...'], 'build.txt')
        run(['go', 'vet', './...'], 'vet.txt')
        packages = run(['go', 'list', './...'], 'packages.txt').splitlines()
        expected = set()
        for i, package in enumerate(packages):
            listing = run(['go', 'test', '-list', '^(Test|Example|Fuzz)', package], f'inventory-{i}.txt')
            expected.update((package, line) for line in inventory_names(listing))
        if not expected: raise RuntimeError('test inventory is empty')
        (output / 'inventory.json').write_text(json.dumps(sorted(expected), indent=2) + '\n')
        # Keep raw JSON even when Go exits nonzero, then inspect events below.
        test_run_error = None
        try:
            run(['go', 'test', '-p=1', '-count=1', '-timeout=15m', '-race', '-json',
                 '-covermode=atomic', '-coverpkg=./...', '-coverprofile=' + str(output / 'coverage.out'), './...'], 'tests.jsonl')
        except RuntimeError as exc:
            test_run_error = str(exc)
            report['test_command_error'] = test_run_error
        finally:
            events = [json.loads(line) for line in (output / 'tests.jsonl').read_text().splitlines()
                      if line.startswith('{')]
            report['tests'] = check_events(events, expected)
        # Failed/skipped executions still locate coverage gaps. Preserve those
        # measurements when Go produced a profile, without letting them pass.
        test_gate_failed = bool(test_run_error) or any(
            report['tests'][key] for key in ('skipped', 'failed', 'missing', 'unfinished', 'missing_packages'))
        report['coverage_from_qualified_tests'] = not test_gate_failed
        run(['go', 'tool', 'cover', '-func=' + str(output / 'coverage.out')], 'coverage.txt')
        report['coverage'] = coverage_metrics((output / 'coverage.out').read_text())
        report['baseline'] = run(['git', 'rev-parse', '--verify', args.baseline + '^{commit}'], 'baseline.txt').strip()
        module = run(['go', 'list', '-m'], 'module.txt').strip()
        source_scope = classify_changed_sources(ROOT, changed_lines(ROOT, report['baseline']),
            json.loads((ROOT / 'docs/acceptance/coverage-scope.json').read_text()))
        report['changed_coverage'] = changed_coverage((output / 'coverage.out').read_text(), module,
                                                     source_scope['production_changes'], root=ROOT)
        report['changed_coverage']['excluded_tests'] = source_scope['excluded_tests']
        (output / 'changed-coverage.json').write_text(json.dumps(report['changed_coverage'], indent=2) + '\n')
        report['critical_coverage'] = critical_coverage(
            {module: (output / 'coverage.out').read_text()},
            json.loads((ROOT / 'docs/acceptance/critical-coverage-map.json').read_text()),
            roots={module: ROOT})
        (output / 'critical-coverage.json').write_text(json.dumps(report['critical_coverage'], indent=2) + '\n')
        if test_gate_failed:
            raise RuntimeError(test_run_error or 'missing, skipped or failed tests; inspect report')
        if report['changed_coverage']['missing_profile_files']:
            raise RuntimeError('changed production files missing from coverage profile')
        run([sys.executable, '-m', 'unittest', 'discover', '-s', 'scripts', '-p', 'test_*.py'], 'runner-tests.txt')
        run([sys.executable, '-m', 'unittest', 'discover', '-s', 'docs/acceptance', '-p', 'test_*.py'], 'checker-tests.txt')
        binary = output / 'hand'
        candidate = report['commit'] + ('' if report['clean_source'] else '-dirty')
        run(['go', 'build', '-ldflags=-X main.version=validation -X main.buildCommit=' + candidate,
             '-o', str(binary), './cmd/hand'], 'binary-build.txt')
        with tempfile.TemporaryDirectory(prefix='hand-smoke-') as home:
            env = dict(os.environ, HOME=home)
            for key in list(env):
                if key.endswith('_API_KEY'): env.pop(key)
            for flag in ('--version', '--help'):
                result = subprocess.run([str(binary), flag], cwd=home, env=env,
                                        capture_output=True, text=True, timeout=10)
                content = result.stdout + result.stderr
                (output / (flag[2:] + '.txt')).write_text(content)
                if result.returncode: raise RuntimeError(f'{flag} failed without credentials')
                if flag == '--version' and report['commit'] not in content:
                    raise RuntimeError('binary does not identify candidate commit')
            if list(Path(home).iterdir()):
                raise RuntimeError('help/version created configuration without a run')
        if source_snapshot(ROOT)['sha256'] != before['sha256']:
            raise RuntimeError('source changed during validation; evidence is not a consistent candidate')
        report['status'] = 'passed'
    except (RuntimeError, OSError, ValueError, subprocess.SubprocessError) as exc:
        report['error'] = str(exc)
        print(str(exc), file=sys.stderr)
    finally:
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
        print('Evidence: ' + str(output), flush=True)
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    sys.exit(main())

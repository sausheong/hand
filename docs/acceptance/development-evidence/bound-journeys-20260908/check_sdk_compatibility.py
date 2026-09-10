#!/usr/bin/env python3
"""Build a genuine external SDK consumer and compare three Hand interfaces.

Development replacements are explicit and never produce released-package evidence.
The fixture uses a local HTTP server and makes no external model calls.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--version', help='Released Hand module version')
    mode.add_argument('--checkout', type=Path, help='Development Hand checkout')
    parser.add_argument('--harness-checkout', type=Path)
    parser.add_argument('--fixture', type=Path, required=True)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    if args.version and not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', args.version):
        parser.error('--version must be an explicit release version')
    if args.version and args.harness_checkout:
        parser.error('released mode cannot use a Harness replacement')
    if args.checkout and not args.harness_checkout:
        parser.error('development mode requires an explicit Harness checkout')
    fixture, binary = args.fixture.resolve(strict=True), args.hand_binary.resolve(strict=True)
    out = args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    module = out / 'consumer'
    module.mkdir()
    shutil.copyfile(fixture, module / 'main.go')
    env = os.environ.copy()
    env.update(GOWORK='off', GOFLAGS='')
    report = {'status': 'failed', 'qualification': 'released' if args.version else 'development',
              'hand_version': args.version, 'binary': str(binary),
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'fixture_sha256': hashlib.sha256(fixture.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'commands': [],
              'scope': 'external module build and local completion/cancellation equivalence only'}

    def run(label, command):
        result = subprocess.run(command, cwd=module, env=env, capture_output=True, timeout=180)
        stdout, stderr = out / (label + '.stdout'), out / (label + '.stderr')
        stdout.write_bytes(result.stdout)
        stderr.write_bytes(result.stderr)
        report['commands'].append({'argv': command, 'cwd': str(module), 'exit_code': result.returncode,
                                   'stdout': str(stdout), 'stderr': str(stderr),
                                   'stdout_sha256': hashlib.sha256(result.stdout).hexdigest(),
                                   'stderr_sha256': hashlib.sha256(result.stderr).hexdigest()})
        if result.returncode:
            raise RuntimeError(f'{label} exited {result.returncode}; see {stderr}')
        return result.stdout.decode()

    try:
        run('init', ['go', 'mod', 'init', 'example.com/hand-sdk-compatibility'])
        run('require', ['go', 'mod', 'edit', '-require=github.com/sausheong/hand@' + (args.version or 'v0.0.0')])
        if args.checkout:
            for name, path in [('hand', args.checkout), ('harness', args.harness_checkout)]:
                resolved = str(path.resolve(strict=True))
                report[name + '_checkout'] = resolved
                run('replace-' + name, ['go', 'mod', 'edit', '-replace=github.com/sausheong/' + name + '=' + resolved])
        run('build', ['go', 'build', '-mod=mod', '-o', str(module / 'consumer'), '.'])
        modules_text = run('modules', ['go', 'list', '-m', '-json', 'github.com/sausheong/hand', 'github.com/sausheong/harness'])
        decoder, position, modules = json.JSONDecoder(), 0, []
        while position < len(modules_text):
            if not modules_text[position:].strip():
                break
            position += len(modules_text[position:]) - len(modules_text[position:].lstrip())
            value, position = decoder.raw_decode(modules_text, position)
            modules.append(value)
        if args.version:
            selected = {m['Path']: m for m in modules}
            for name in ['hand', 'harness']:
                dependency = selected['github.com/sausheong/' + name]
                if dependency.get('Replace') or not dependency.get('Version'):
                    raise RuntimeError('released dependency is replaced or unversioned: ' + name)
            if selected['github.com/sausheong/hand']['Version'] != args.version:
                raise RuntimeError('resolved Hand version differs from requested release')
        build_info = run('binary-build-info', ['go', 'version', '-m', str(binary)])
        binary_version = run('binary-version', [str(binary), '--version']).strip()
        if args.version:
            if not re.fullmatch(re.escape(args.version) + r' \(commit [0-9a-f]{40}\)', binary_version):
                raise RuntimeError('binary version/commit does not match the requested release')
            harness_version = selected['github.com/sausheong/harness']['Version']
            expected = 'dep\tgithub.com/sausheong/harness\t' + harness_version + '\t'
            if expected not in build_info or '\n\t=>\t' in build_info:
                raise RuntimeError('binary Harness version differs or contains development replacements')
        journey = json.loads(run('journey', [str(module / 'consumer'), str(binary)]))
        if journey.get('status') != 'passed' or journey.get('provider_calls') != 6 or journey.get('cancelled_provider_requests') != 3:
            raise RuntimeError('compatibility journey did not pass')
        report['journey'] = journey
        report['status'] = 'passed'
    except (OSError, subprocess.SubprocessError, ValueError, RuntimeError, KeyError) as exc:
        report['error'] = str(exc)
    finally:
        report['go_mod_sha256'] = hashlib.sha256((module / 'go.mod').read_bytes()).hexdigest() if (module / 'go.mod').exists() else None
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'status': report['status'], 'qualification': report['qualification'], 'report': str(out / 'run.json')}))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    sys.exit(main())

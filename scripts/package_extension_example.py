#!/usr/bin/env python3
"""Build a shipped native extension example and obtain its verified package pin."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

EXAMPLES = {
    'task-note': ('go-task-note', ['commands', 'questions', 'state', 'context.transform']),
    'tool-policy': ('go-tool-policy', ['policy.check']),
    'verification-hook': ('go-verification-hook', ['commands', 'lifecycle']),
    'tool-viewer': ('go-tool-viewer', ['commands', 'presentation']),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-root', required=True, type=Path)
    parser.add_argument('--hand-binary', required=True, type=Path)
    parser.add_argument('--example', required=True, choices=EXAMPLES)
    parser.add_argument('--out', required=True, type=Path, help='new output directory containing package and evidence')
    args = parser.parse_args()
    source = args.source_root.resolve(strict=True)
    hand = args.hand_binary.resolve(strict=True)
    out = args.out.resolve(); out.mkdir(parents=True, exist_ok=False)
    package = out / 'package'; package.mkdir(mode=0o700)
    folder, capabilities = EXAMPLES[args.example]
    executable = package / args.example
    report = dict(status='failed', example=args.example, source_root=str(source),
                  hand_binary_sha256=hashlib.sha256(hand.read_bytes()).hexdigest(),
                  script_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  extension_executions=0)
    commands = []

    def run(command, timeout):
        environment = dict(os.environ, GOPROXY='off')
        result = subprocess.run(list(map(str, command)), cwd=source, env=environment,
                                capture_output=True, text=True, timeout=timeout)
        commands.append(dict(arguments=list(map(str, command)), exit_code=result.returncode,
                             stdout=result.stdout, stderr=result.stderr))
        (out / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        if result.returncode:
            raise RuntimeError('command failed; see commands.json')
        return result.stdout

    try:
        run(['go', 'build', '-trimpath', '-buildvcs=false', '-o', executable,
             './examples/extensions/' + folder], 120)
        executable.chmod(0o700)
        digest = hashlib.sha256(executable.read_bytes()).hexdigest()
        manifest = dict(schema=1, name=args.example, version='1.0.0',
                        compatibility=dict(minimum_hand='0.1.0', extension_protocol=1),
                        files=[dict(path=args.example, kind='extension', sha256=digest,
                                    size=executable.stat().st_size, executable=True)],
                        extensions=[dict(name=args.example, entrypoint=args.example, capabilities=capabilities)])
        (package / 'hand-package.json').write_text(json.dumps(manifest, indent=2) + '\n')
        inspected = json.loads(run([hand, 'packages', 'inspect', '--source', package], 30))
        if inspected['manifest'] != manifest:
            raise RuntimeError('manifest round-trip differs')
        report.update(status='passed', package_directory=str(package),
                      package_digest=inspected['package_digest'], executable_sha256=digest,
                      manifest_sha256=hashlib.sha256((package / 'hand-package.json').read_bytes()).hexdigest(),
                      capabilities=capabilities,
                      limitations=['Native build for the selected Go environment; not a signed release or cross-platform archive.',
                                   'Pin identifies these built bytes. Reproduce with the same source, Go toolchain and dependencies.',
                                   'Verification-hook additionally requires host Git; no example is executed during packaging.'])
    except Exception as exc:
        report['error'] = repr(exc)
    (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

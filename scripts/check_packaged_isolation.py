#!/usr/bin/env python3
"""Run the RPC isolation journey using a CLI and worker extracted from one archive.

Uses a local fixture provider. The explicit execution config supplies a cached
image and local Docker socket; no image is pulled or real provider called.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile

from smoke_dist import verify_worker


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--execution-config', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    target = platform.system().lower() + '-' + {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine(), platform.machine())
    prefix = f'hand-{args.version}-{target}'
    config = json.loads(args.execution_config.read_text())
    if config.get('backend') != 'container' or config.get('network', False):
        raise ValueError('fixture requires container mode with networking disabled')
    output = args.out.resolve()
    output.mkdir(parents=True, exist_ok=False)
    with tarfile.open(args.archive) as tar:
        manifest = verify_worker(tar, prefix, target, args.version, args.commit)
        for name in ['hand', 'hand-tool-worker-linux']:
            member = tar.getmember(prefix + '/' + name)
            if not member.isfile() or not member.mode & 0o111:
                raise ValueError('invalid executable: ' + name)
            path = output / name
            path.write_bytes(tar.extractfile(member).read())
            path.chmod(0o700)
    smoke_home = output / 'identity-home'
    smoke_home.mkdir()
    identity = subprocess.run([str(output / 'hand'), '--version'], cwd=smoke_home,
                              env={'HOME': str(smoke_home), 'PATH': os.defpath},
                              capture_output=True, text=True, timeout=10, check=True)
    if args.version not in identity.stdout or args.commit not in identity.stdout:
        raise ValueError('packaged CLI release identity mismatch')
    if list(smoke_home.iterdir()):
        raise ValueError('version check created user state')
    config['worker'] = str(output / 'hand-tool-worker-linux')
    config['worker_sha256'] = manifest['sha256']
    config_path = output / 'execution.json'
    config_path.write_text(json.dumps(config, indent=2) + '\n')
    runner = Path(__file__).with_name('check_rpc_client.py').resolve()
    command = [sys.executable, str(runner), '--hand-binary', str(output / 'hand'),
               '--execution-config', str(config_path), '--out', str(output / 'rpc')]
    result = subprocess.run(command, capture_output=True, text=True, timeout=180)
    (output / 'stdout.log').write_text(result.stdout)
    (output / 'stderr.log').write_text(result.stderr)
    report = {'status': 'passed' if result.returncode == 0 else 'failed',
              'archive': str(args.archive.resolve()),
              'archive_sha256': hashlib.sha256(args.archive.read_bytes()).hexdigest(),
              'worker': manifest, 'target': target, 'command': command,
              'returncode': result.returncode,
              'runner_sha256': hashlib.sha256(runner.read_bytes()).hexdigest(),
              'wrapper_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'rpc_evidence': str(output / 'rpc/run.json')}
    (output / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return result.returncode


if __name__ == '__main__':
    sys.exit(main())

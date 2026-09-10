#!/usr/bin/env python3
"""Build Hand archives with a Linux container worker for the matching architecture."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

TARGETS = ('darwin-amd64', 'darwin-arm64', 'linux-amd64', 'linux-arm64')


def build(checkout, destination, version, commit):
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]*', version):
        raise ValueError('version must be a safe archive filename component')
    if not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise ValueError('commit must be a full Git SHA')
    checkout = Path(checkout).resolve()
    destination = Path(destination).resolve()
    # A fresh output directory prevents stale archives from entering a release.
    destination.mkdir(parents=True, exist_ok=False)
    sums = []
    with tempfile.TemporaryDirectory(prefix='hand-release-build-') as temp:
        temp = Path(temp)
        workers = {}
        for arch in ('amd64', 'arm64'):
            worker = temp / ('worker-' + arch)
            env = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH=arch)
            subprocess.run(['go', 'build', '-trimpath', '-o', str(worker),
                            './cmd/hand-tool-worker'], cwd=checkout, env=env, check=True)
            workers[arch] = worker.read_bytes()
        for target in TARGETS:
            system, arch = target.split('-')
            name = f'hand-{version}-{target}'
            root = temp / name
            root.mkdir()
            env = dict(os.environ, CGO_ENABLED='0', GOOS=system, GOARCH=arch)
            subprocess.run(['go', 'build', '-trimpath', '-ldflags',
                            f'-X main.version={version} -X main.buildCommit={commit}',
                            '-o', str(root / 'hand'), './cmd/hand'],
                           cwd=checkout, env=env, check=True)
            worker = root / 'hand-tool-worker-linux'
            worker.write_bytes(workers[arch])
            worker.chmod(0o755)
            digest = hashlib.sha256(workers[arch]).hexdigest()
            (root / 'WORKER.json').write_text(json.dumps({
                'schema_version': 1, 'file': worker.name, 'os': 'linux',
                'arch': arch, 'sha256': digest, 'commit': commit,
                'version': version, 'protocol_version': 1,
            }, indent=2) + '\n')
            for filename in ('README.md', 'LICENSE'):
                (root / filename).write_bytes((checkout / filename).read_bytes())
            archive = destination / (name + '.tar.gz')
            with tarfile.open(archive, 'w:gz') as tar:
                tar.add(root, arcname=name)
            sums.append(hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name)
    (destination / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--checkout', type=Path, default=Path.cwd())
    parser.add_argument('--dist', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--commit', required=True)
    args = parser.parse_args()
    build(args.checkout, args.dist, args.version, args.commit)


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Check archive contents/checksums and execute the host's binary without credentials."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import struct
import tarfile
import tempfile


def verify_worker(tar, prefix, target, version, commit):
    member = tar.getmember(prefix + '/hand-tool-worker-linux')
    if not member.isfile() or not member.mode & 0o111:
        raise ValueError('worker must be a regular executable')
    data = tar.extractfile(member).read()
    arch = target.split('-')[1]
    # Check the actual ELF header, not just a manifest label.
    machine = {'amd64': 62, 'arm64': 183}[arch]
    if len(data) < 64 or data[:6] != b'\x7fELF\x02\x01' or struct.unpack_from('<H', data, 18)[0] != machine:
        raise ValueError('worker is not Linux ELF for target architecture')
    manifest = json.load(tar.extractfile(prefix + '/WORKER.json'))
    expected = dict(schema_version=1, file='hand-tool-worker-linux', os='linux',
                    arch=arch, sha256=hashlib.sha256(data).hexdigest(),
                    commit=commit, version=version, protocol_version=1)
    if manifest != expected:
        raise ValueError('worker manifest does not match executable or release')
    return manifest


def verify_contents(tar, prefix, target, require_worker=False):
    members = tar.getmembers()
    names = [member.name for member in members]
    if len(names) != len(set(names)):
        raise ValueError('duplicate archive member')
    required = {prefix + '/' + name for name in ('hand', 'README.md', 'LICENSE')}
    worker = {prefix + '/' + name for name in ('hand-tool-worker-linux', 'WORKER.json')}
    present = set(names)
    if require_worker or present & worker:
        required |= worker
    if present - {prefix} != required:
        raise ValueError('unexpected or missing archive member')
    for member in members:
        if member.name == prefix:
            if not member.isdir():
                raise ValueError('archive root must be a directory')
        elif not member.isfile() or member.size == 0:
            raise ValueError('archive content must be a nonempty regular file')
    cli = tar.getmember(prefix + '/hand')
    if not cli.mode & 0o111:
        raise ValueError('CLI must be executable')
    with tar.extractfile(cli) as stream:
        header = stream.read(64)
    system, arch = target.split('-')
    if system == 'linux':
        machine = {'amd64': 62, 'arm64': 183}[arch]
        valid = (len(header) == 64 and header[:6] == b'\x7fELF\x02\x01'
                 and struct.unpack_from('<H', header, 18)[0] == machine)
    else:
        cpu = {'amd64': 0x01000007, 'arm64': 0x0100000c}[arch]
        valid = (len(header) >= 32 and header[:4] == b'\xcf\xfa\xed\xfe'
                 and struct.unpack_from('<I', header, 4)[0] == cpu)
    if not valid:
        raise ValueError('CLI binary does not match target OS/architecture')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--dist', type=Path, required=True)
    p.add_argument('--version', required=True)
    p.add_argument('--commit', required=True)
    p.add_argument('--require-worker', action='store_true')
    args = p.parse_args()
    sums = {}
    for line in (args.dist / 'SHA256SUMS').read_text().splitlines():
        value, filename = line.split()
        if filename in sums: raise ValueError('duplicate checksum entry')
        sums[filename] = value
    targets = ['darwin-amd64', 'darwin-arm64', 'linux-amd64', 'linux-arm64']
    names = {f'hand-{args.version}-{target}.tar.gz' for target in targets}
    if set(sums) != names: raise ValueError('checksum inventory does not match targets')
    host = platform.system().lower() + '-' + {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine(), platform.machine())
    checked = []
    for target in targets:
        prefix = f'hand-{args.version}-{target}'
        archive = args.dist / (prefix + '.tar.gz')
        if hashlib.sha256(archive.read_bytes()).hexdigest() != sums[archive.name]:
            raise ValueError('checksum mismatch: ' + archive.name)
        with tarfile.open(archive) as tar:
            verify_contents(tar, prefix, target, args.require_worker)
            if args.require_worker:
                verify_worker(tar, prefix, target, args.version, args.commit)
            native = target == host
            if native:
                with tempfile.TemporaryDirectory(prefix='hand-dist-smoke-') as temp:
                    root = Path(temp)
                    binary = root / 'hand'
                    binary.write_bytes(tar.extractfile(prefix + '/hand').read())
                    binary.chmod(0o700)
                    home = root / 'home'
                    home.mkdir()
                    env = {k: v for k, v in os.environ.items() if not k.endswith('_API_KEY')}
                    env['HOME'] = str(home)
                    for flag in ['--version', '--help']:
                        r = subprocess.run([str(binary), flag], cwd=home, env=env,
                                           capture_output=True, text=True, timeout=10, check=True)
                        if flag == '--version' and r.stdout.strip() != f'{args.version} (commit {args.commit})':
                            raise ValueError('incorrect release identity')
                    if list(home.iterdir()): raise ValueError('smoke run created state')
        checked.append(dict(target=target, checksum='passed', contents='passed', worker='passed' if args.require_worker else 'not_checked', native_smoke=native))
    print(json.dumps(dict(archives=checked), indent=2))


if __name__ == '__main__': main()

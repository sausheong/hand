"""Reject misleading release archive layouts and CLI target labels."""
import io
import struct
import tarfile
import unittest
from smoke_dist import verify_contents


def header(target):
    data = bytearray(64)
    system, arch = target.split('-')
    if system == 'linux':
        data[:6] = b'\x7fELF\x02\x01'
        struct.pack_into('<H', data, 18, {'arm64': 183, 'amd64': 62}[arch])
    else:
        data[:4] = b'\xcf\xfa\xed\xfe'
        struct.pack_into('<I', data, 4, {'arm64': 0x0100000c, 'amd64': 0x01000007}[arch])
    return data


def archive(target='linux-arm64', mutation=None):
    entries = []
    root = tarfile.TarInfo('release')
    root.type = tarfile.DIRTYPE
    entries.append((root, b''))
    for name in ['hand', 'README.md', 'LICENSE', 'hand-tool-worker-linux', 'WORKER.json']:
        content = header(target) if name == 'hand' else b'fixture'
        info = tarfile.TarInfo('release/' + name)
        info.size, info.mode = len(content), 0o755
        entries.append((info, content))
    if mutation:
        mutation(entries)
    data = io.BytesIO()
    with tarfile.open(fileobj=data, mode='w') as tar:
        for info, content in entries:
            tar.addfile(info, io.BytesIO(content))
    data.seek(0)
    return tarfile.open(fileobj=data)


class ArchiveContentsTests(unittest.TestCase):
    def test_all_targets(self):
        for target in ['linux-amd64', 'linux-arm64', 'darwin-amd64', 'darwin-arm64']:
            with self.subTest(target=target), archive(target) as tar:
                verify_contents(tar, 'release', target, True)

    def test_wrong_target(self):
        for target in ['linux-amd64', 'darwin-arm64']:
            with self.subTest(target=target), archive() as tar:
                with self.assertRaisesRegex(ValueError, 'OS/architecture'):
                    verify_contents(tar, 'release', target, True)

    def test_duplicate(self):
        with archive(mutation=lambda e: e.append(e[1])) as tar:
            with self.assertRaisesRegex(ValueError, 'duplicate'):
                verify_contents(tar, 'release', 'linux-arm64', True)

    def test_unexpected_paths(self):
        for name in ['../escape', '/absolute', 'release/../escape', 'release/extra']:
            def change(entries):
                info = tarfile.TarInfo(name)
                info.size = 1
                entries.append((info, b'x'))
            with self.subTest(name=name), archive(mutation=change) as tar:
                with self.assertRaisesRegex(ValueError, 'unexpected'):
                    verify_contents(tar, 'release', 'linux-arm64', True)

    def test_nonregular(self):
        for kind in [tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.FIFOTYPE]:
            def change(entries):
                entries[1][0].type = kind
                entries[1][0].linkname = 'elsewhere'
                entries[1][0].size = 0
                entries[1] = (entries[1][0], b'')
            with self.subTest(kind=kind), archive(mutation=change) as tar:
                with self.assertRaisesRegex(ValueError, 'regular'):
                    verify_contents(tar, 'release', 'linux-arm64', True)

    def test_missing_worker(self):
        with archive(mutation=lambda e: e.pop()) as tar:
            with self.assertRaisesRegex(ValueError, 'missing'):
                verify_contents(tar, 'release', 'linux-arm64', True)

    def test_nonexecutable_cli(self):
        def change(entries):
            entries[1][0].mode = 0o644
        with archive(mutation=change) as tar:
            with self.assertRaisesRegex(ValueError, 'executable'):
                verify_contents(tar, 'release', 'linux-arm64', True)


if __name__ == '__main__':
    unittest.main()

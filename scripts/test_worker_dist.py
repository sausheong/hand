"""Archive validation rejects wrong-architecture and tampered workers."""
import hashlib
import io
import json
import struct
import tarfile
import unittest
from smoke_dist import verify_worker


class WorkerArchiveTests(unittest.TestCase):
    def archive(self, machine=183, digest=None, mode=0o755):
        data = bytearray(64)
        data[:6] = b'\x7fELF\x02\x01'
        struct.pack_into('<H', data, 18, machine)
        manifest = dict(schema_version=1, file='hand-tool-worker-linux', os='linux',
                        arch='arm64', sha256=digest or hashlib.sha256(data).hexdigest(),
                        commit='a' * 40, version='test', protocol_version=1)
        buf = io.BytesIO()
        with tarfile.open(fileobj=buf, mode='w') as tar:
            for name, content in [('hand-tool-worker-linux', data),
                                  ('WORKER.json', json.dumps(manifest).encode())]:
                info = tarfile.TarInfo('release/' + name)
                info.size, info.mode = len(content), mode
                tar.addfile(info, io.BytesIO(content))
        buf.seek(0)
        return tarfile.open(fileobj=buf)

    def test_valid_worker(self):
        with self.archive() as tar:
            self.assertEqual(verify_worker(tar, 'release', 'darwin-arm64', 'test', 'a'*40)['os'], 'linux')

    def test_wrong_architecture(self):
        with self.archive(machine=62) as tar, self.assertRaisesRegex(ValueError, 'architecture'):
            verify_worker(tar, 'release', 'darwin-arm64', 'test', 'a'*40)

    def test_tampered_digest(self):
        with self.archive(digest='0'*64) as tar, self.assertRaisesRegex(ValueError, 'manifest'):
            verify_worker(tar, 'release', 'darwin-arm64', 'test', 'a'*40)

    def test_not_executable(self):
        with self.archive(mode=0o644) as tar, self.assertRaisesRegex(ValueError, 'executable'):
            verify_worker(tar, 'release', 'darwin-arm64', 'test', 'a'*40)

    def test_wrong_release_identity(self):
        with self.archive() as tar, self.assertRaisesRegex(ValueError, 'manifest'):
            verify_worker(tar, 'release', 'darwin-arm64', 'other', 'a'*40)


if __name__ == '__main__':
    unittest.main()

import hashlib
import os
from pathlib import Path
import tempfile
import unittest

from evaluation_workspace import verify_fixtures


class FixtureIntegrityTests(unittest.TestCase):
    def test_persistent_mutations_and_special_files_fail_without_following_links(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            fixture = root/'oracle_test.go'
            original = b'package fixture\n'
            record = dict(destination=fixture.name, sha256=hashlib.sha256(original).hexdigest())
            fixture.write_bytes(original)
            self.assertEqual(verify_fixtures(root, [record])['status'], 'passed')
            for kind in ('changed', 'deleted', 'symlink', 'fifo', 'directory'):
                with self.subTest(kind=kind):
                    if fixture.is_dir(): fixture.rmdir()
                    elif fixture.exists() or fixture.is_symlink(): fixture.unlink()
                    if kind == 'changed': fixture.write_bytes(b'changed')
                    elif kind == 'symlink':
                        (root/'outside').write_bytes(original)
                        fixture.symlink_to(root/'outside')
                    elif kind == 'fifo': os.mkfifo(fixture)
                    elif kind == 'directory': fixture.mkdir()
                    report = verify_fixtures(root, [record])
                    self.assertEqual(report['status'], 'failed')
                    self.assertEqual(len(report['errors']), 1)

    def test_parent_symlink_is_rejected_even_when_bytes_match(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root/'actual').mkdir()
            (root/'actual/oracle').write_bytes(b'original')
            (root/'alias').symlink_to(root/'actual', target_is_directory=True)
            record = dict(destination='alias/oracle', sha256=hashlib.sha256(b'original').hexdigest())
            self.assertEqual(verify_fixtures(root, [record])['status'], 'failed')


if __name__ == '__main__': unittest.main()

import hashlib
import os
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch

from evaluation_workspace import install_fixtures


class WorkspaceTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.source, self.workspace = self.root/'source', self.root/'workspace'
        self.source.mkdir()
        self.workspace.mkdir()
        self.payload = b'assert actual_result == expected_result\n'
        (self.source/'oracle.py').write_bytes(self.payload)
        self.fixture = dict(source='oracle.py', destination='tests/oracle.py',
                            sha256=hashlib.sha256(self.payload).hexdigest())

    def install(self, fixtures=None):
        return install_fixtures(self.source, self.workspace, fixtures or [self.fixture])

    def test_private_exact_copy_and_exclusive_retry(self):
        report = self.install()
        target = self.workspace/'tests/oracle.py'
        self.assertEqual(target.read_bytes(), self.payload)
        self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)
        self.assertEqual(report['total_bytes'], len(self.payload))
        with self.assertRaises(FileExistsError): self.install()
        self.assertEqual(target.read_bytes(), self.payload)

    def test_all_hashes_verified_before_any_write(self):
        second = dict(self.fixture, destination='second.py', sha256='0'*64)
        with self.assertRaisesRegex(ValueError, 'hash mismatch'): self.install([self.fixture, second])
        self.assertEqual(list(self.workspace.iterdir()), [])

    def test_existing_destination_preflight_preserves_all_files(self):
        (self.workspace/'existing').write_text('preserve me')
        second = dict(self.fixture, destination='existing')
        with self.assertRaises(FileExistsError): self.install([self.fixture, second])
        self.assertEqual((self.workspace/'existing').read_text(), 'preserve me')
        self.assertFalse((self.workspace/'tests').exists())

    def test_destination_symlink_parent_and_leaf_cannot_escape(self):
        outside = self.root/'outside'
        outside.mkdir()
        (outside/'preserved').write_text('keep')
        (self.workspace/'tests').symlink_to(outside, target_is_directory=True)
        with self.assertRaises(OSError): self.install()
        self.assertEqual(list(outside.iterdir()), [outside/'preserved'])
        (self.workspace/'tests').unlink()
        (self.workspace/'tests').mkdir()
        (self.workspace/'tests/oracle.py').symlink_to(outside/'preserved')
        with self.assertRaises(FileExistsError): self.install()
        self.assertEqual((outside/'preserved').read_text(), 'keep')

    def test_source_symlink_leaf_and_parent_rejected(self):
        (self.source/'link.py').symlink_to(self.source/'oracle.py')
        (self.source/'linked').symlink_to(self.source, target_is_directory=True)
        for name in ['link.py', 'linked/oracle.py']:
            with self.subTest(name=name), self.assertRaises(OSError):
                self.install([dict(self.fixture, source=name)])
        self.assertEqual(list(self.workspace.iterdir()), [])

    def test_fifo_source_rejected_without_waiting_for_writer(self):
        os.mkfifo(self.source/'pipe')
        with self.assertRaisesRegex(ValueError, 'regular file'):
            self.install([dict(self.fixture, source='pipe')])

    def test_unsafe_names_and_metadata_rejected(self):
        for name in ['../outside', '/absolute', './oracle.py', 'a//b', 'a/../b', '.', '.GIT/config', 'a\\b', 'a\x00b']:
            for key in ['source', 'destination']:
                with self.subTest(name=name, key=key), self.assertRaises(ValueError):
                    self.install([dict(self.fixture, **{key: name})])
        self.assertEqual(list(self.workspace.iterdir()), [])

    def test_overlap_and_duplicate_targets_rejected(self):
        for first, second in [('a', 'a'), ('a', 'a/b'), ('a/b', 'a')]:
            with self.subTest(first=first, second=second), self.assertRaisesRegex(ValueError, 'overlapping'):
                self.install([dict(self.fixture, destination=first), dict(self.fixture, destination=second)])
        self.assertEqual(list(self.workspace.iterdir()), [])

    def test_symlink_root_and_ancestor_rejected(self):
        alias = self.root/'alias'
        alias.symlink_to(self.root, target_is_directory=True)
        for source, workspace in [(alias/'source', self.workspace), (self.source, alias/'workspace')]:
            with self.subTest(source=source, workspace=workspace), self.assertRaises(OSError):
                install_fixtures(source, workspace, [self.fixture])

    def test_byte_caps_checked_before_creating_destinations(self):
        with patch('evaluation_workspace.MAX_FILE_BYTES', 2), self.assertRaises(ValueError): self.install()
        with patch('evaluation_workspace.MAX_TOTAL_BYTES', 2), self.assertRaises(ValueError): self.install()
        self.assertEqual(list(self.workspace.iterdir()), [])

    def test_sync_failure_never_returns_success(self):
        with patch('evaluation_workspace.os.fsync', side_effect=OSError('disk failure')):
            with self.assertRaisesRegex(OSError, 'disk failure'): self.install()
        # The caller must discard a failed workspace, not retry it as a fresh run.
        with self.assertRaises(FileExistsError): self.install()

    def test_leaf_link_inserted_after_preflight_cannot_overwrite_referent(self):
        outside = self.root/'outside'
        outside.write_text('preserve')
        original_open = os.open

        def insert_link(name, flags, *args, **kwargs):
            if flags & os.O_CREAT:
                os.symlink(str(outside), name, dir_fd=kwargs['dir_fd'])
            return original_open(name, flags, *args, **kwargs)

        with patch('evaluation_workspace.os.open', side_effect=insert_link):
            with self.assertRaises(FileExistsError): self.install()
        self.assertEqual(outside.read_text(), 'preserve')


if __name__ == '__main__': unittest.main()

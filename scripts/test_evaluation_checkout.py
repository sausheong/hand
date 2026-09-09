import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from evaluation_checkout import git, prepare_checkout


class CheckoutTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.source = self.root/'source'
        self.source.mkdir()
        self.destination = self.root/'candidate'
        git(self.source, 'init', '--template=', '--quiet')
        (self.source/'main.py').write_bytes(b'print("pinned source")\n')
        (self.source/'run.sh').write_text('#!/bin/sh\nexit 0\n')
        (self.source/'run.sh').chmod(0o755)
        (self.source/'.gitattributes').write_text('main.py export-ignore\nrun.sh export-subst\n')
        self.commit = self.commit_source()

    def commit_source(self):
        git(self.source, 'add', '.')
        git(self.source, '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
            'commit', '--quiet', '-m', 'fixture')
        return git(self.source, 'rev-parse', 'HEAD').decode().strip()

    def test_pinned_bytes_modes_and_independent_objects(self):
        (self.source/'main.py').write_text('uncommitted change')
        (self.source/'untracked').write_text('do not copy')
        report = prepare_checkout(self.source, self.commit, self.destination)
        self.assertEqual((self.destination/'main.py').read_bytes(), b'print("pinned source")\n')
        self.assertFalse((self.destination/'untracked').exists())
        self.assertEqual((self.destination/'run.sh').stat().st_mode & 0o777, 0o755)
        self.assertEqual(git(self.destination, 'rev-parse', 'HEAD').decode().strip(), self.commit)
        for entry in report['files']:
            self.assertEqual(hashlib.sha256((self.destination/entry['path']).read_bytes()).hexdigest(), entry['sha256'])
        shutil.rmtree(self.source)
        self.assertEqual(git(self.destination, 'cat-file', '-t', self.commit).strip(), b'commit')
        self.assertEqual(git(self.destination, 'status', '--porcelain'), b'')

    def test_existing_destination_preserved(self):
        self.destination.mkdir()
        (self.destination/'keep').write_text('unchanged')
        with self.assertRaises(FileExistsError): prepare_checkout(self.source, self.commit, self.destination)
        self.assertEqual((self.destination/'keep').read_text(), 'unchanged')
        self.assertFalse((self.destination/'.git').exists())

    def test_prepared_index_accepts_patch_without_status_refresh(self):
        (self.source/'main.py').write_text('print("changed")\n')
        patch_bytes=git(self.source,'diff','--binary','--full-index')
        patch_file=self.root/'change.patch';patch_file.write_bytes(patch_bytes)
        prepare_checkout(self.source,self.commit,self.destination)
        git(self.destination,'apply','--check','--index',str(patch_file))
        git(self.destination,'apply','--index',str(patch_file))
        self.assertEqual((self.destination/'main.py').read_text(),'print("changed")\n')
        self.assertTrue(git(self.destination,'diff','--cached','--name-only').strip())

    def test_source_symlink_is_explicit_failure_not_omission(self):
        (self.source/'link').symlink_to('/outside')
        commit = self.commit_source()
        with self.assertRaisesRegex(ValueError, 'symlinks or submodules'):
            prepare_checkout(self.source, commit, self.destination)
        self.assertFalse(self.destination.exists())

    def test_global_hooks_filters_templates_and_environment_not_executed(self):
        marker = self.root/'must-not-exist'
        hooks = self.root/'hooks'; hooks.mkdir()
        hook = hooks/'post-checkout'; hook.write_text('#!/bin/sh\ntouch "'+str(marker)+'"\n'); hook.chmod(0o755)
        config = self.root/'global-config'
        config.write_text('[core]\n hooksPath = '+str(hooks)+'\n[filter "unsafe"]\n smudge = touch '+str(marker)+'\n')
        (self.source/'.gitattributes').write_text('main.py filter=unsafe\n')
        commit = self.commit_source()
        with patch.dict(os.environ, {'GIT_CONFIG_GLOBAL': str(config), 'GIT_TEMPLATE_DIR': str(hooks),
                                      'GIT_DIR': '/missing', 'GIT_WORK_TREE': '/missing'}):
            prepare_checkout(self.source, commit, self.destination)
        self.assertFalse(marker.exists())
        self.assertEqual((self.destination/'main.py').read_bytes(), b'print("pinned source")\n')
        self.assertFalse((self.destination/'.git/hooks/post-checkout').exists())

    def test_missing_or_noncommit_pin_creates_no_destination(self):
        for commit in ['HEAD', '0'*40, git(self.source, 'rev-parse', 'HEAD:main.py').decode().strip()]:
            with self.subTest(commit=commit), self.assertRaises((ValueError, subprocess.SubprocessError)):
                prepare_checkout(self.source, commit, self.destination)
        self.assertFalse(self.destination.exists())

    def test_byte_limits_fail_before_creating_destination(self):
        for limit in ['MAX_SOURCE_BYTES', 'MAX_BLOB_BYTES']:
            with patch('evaluation_checkout.'+limit, 1), self.assertRaises(ValueError):
                prepare_checkout(self.source, self.commit, self.destination)
        self.assertFalse(self.destination.exists())

    def test_destination_symlink_ancestor_rejected(self):
        alias = self.root/'alias'; alias.symlink_to(self.root, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'symlink'):
            prepare_checkout(self.source, self.commit, alias/'candidate')
        self.assertFalse(self.destination.exists())


if __name__ == '__main__': unittest.main()

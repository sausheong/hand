import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
from evaluation_sources import inventory, verify_sources
from test_evaluation_manifest import ROOT


class SourceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git('init')
        (self.root/'main.go').write_text('package main\nfunc main() {}\n')
        (self.root/'helper space.py').write_text('print("fixture")\n')
        (self.root/'README.md').write_text('fixture documentation\n')
        (self.root/'alias.py').symlink_to('/not/a/real/target')
        self.git('add', '.')
        self.git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'fixture')
        self.commit = self.git('rev-parse', 'HEAD').decode().strip()

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], stderr=subprocess.DEVNULL)

    def test_committed_inventory_ignores_dirty_worktree_and_does_not_follow_links(self):
        before = inventory(self.root, self.commit)
        (self.root/'main.go').write_text('dirty source\n')
        (self.root/'untracked.js').write_text('throw new Error("never execute")\n')
        with patch.dict(os.environ, {'GIT_DIR': '/missing/repository', 'GIT_WORK_TREE': '/missing/worktree'}):
            after = inventory(self.root, self.commit)
        self.assertEqual(before, after)
        self.assertEqual(after['regular_files'], 3)
        self.assertEqual(after['language_extensions']['Python']['files'], 1)
        self.assertEqual(set(after['language_extensions']), {'Go', 'Python'})
        self.assertEqual(after['special_entries'][0]['path'], 'alias.py')
        self.assertEqual(after['special_entries'][0]['mode'], '120000')
        self.assertEqual((self.root/'main.go').read_text(), 'dirty source\n')

    def test_missing_and_noncommit_pins_rejected(self):
        for pin in ['HEAD', '0'*40, self.git('rev-parse', 'HEAD:main.go').decode().strip()]:
            with self.subTest(pin=pin), self.assertRaises((ValueError, subprocess.SubprocessError)):
                inventory(self.root, pin)

    def test_missing_mappings_remain_incomplete_and_revisions_are_deduplicated(self):
        manifest = json.loads((ROOT/'calibration.json').read_text())
        for task in manifest['tasks']:
            task['repository'] = dict(url='local-fixture', commit=self.commit)
        missing = verify_sources(manifest, ROOT, {})
        self.assertEqual(missing['status'], 'incomplete')
        self.assertEqual(len(missing['missing_sources']), 1)
        found = verify_sources(manifest, ROOT, {'local-fixture': str(self.root)})
        self.assertEqual(found['status'], 'local_sources_verified')
        self.assertEqual(found['catalogue_status'], 'draft')
        self.assertEqual(len(found['sources']), 1)
        self.assertEqual(len(found['tasks']), len(manifest['tasks']))
        with self.assertRaises(ValueError):
            verify_sources(manifest, ROOT, {'local-fixture': 'relative-path'})


if __name__ == '__main__': unittest.main()

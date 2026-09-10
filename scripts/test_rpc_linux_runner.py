"""Evidence fingerprint regressions, independent of Docker availability."""
from pathlib import Path
import tempfile
import unittest
from check_rpc_linux import dependency_snapshot


class DependencyFingerprintTests(unittest.TestCase):
    def test_module_cache_without_git_detects_edits_deletions_and_mode(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source.go'
            source.write_text('package dependency\n')
            source.chmod(0o644)
            baseline = dependency_snapshot(root)
            self.assertEqual(baseline, dependency_snapshot(root))
            source.write_text('package changed\n')
            self.assertNotEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])
            source.write_text('package dependency\n')
            source.chmod(0o755)
            self.assertNotEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])
            source.chmod(0o644)
            self.assertEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])
            source.unlink()
            self.assertNotEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])

    def test_nested_new_source_is_included_but_git_metadata_is_not(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'go.mod').write_text('module example.test/dependency\n')
            baseline = dependency_snapshot(root)
            (root / '.git').mkdir()
            (root / '.git' / 'index').write_bytes(b'metadata')
            self.assertEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])
            (root / 'nested').mkdir()
            (root / 'nested' / 'new.go').write_text('package nested\n')
            changed = dependency_snapshot(root)
            self.assertNotEqual(baseline['sha256'], changed['sha256'])
            self.assertIn('nested/new.go', [entry['path'] for entry in changed['entries']])

    def test_symlink_target_identity_is_recorded_without_following(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            link = root / 'link'
            link.symlink_to('missing-one')
            baseline = dependency_snapshot(root)
            self.assertEqual(baseline['entries'][0]['mode'], 'symlink')
            link.unlink()
            link.symlink_to('missing-two')
            self.assertNotEqual(baseline['sha256'], dependency_snapshot(root)['sha256'])


if __name__ == '__main__':
    unittest.main()

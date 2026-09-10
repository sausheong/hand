import tempfile
from pathlib import Path
import unittest

from check_transcript_performance import snapshot


class TranscriptProvenanceTests(unittest.TestCase):
    def test_module_content_membership_and_mode_changes_are_visible(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(ValueError):
                snapshot(root)
            source = root / 'module.go'
            source.write_text('package sample\n')
            initial = snapshot(root)
            self.assertEqual(initial, snapshot(root))
            source.write_text('package changed\n')
            self.assertNotEqual(initial['sha256'], snapshot(root)['sha256'])
            source.write_text('package sample\n')
            extra = root / 'go.mod'
            extra.write_text('module sample\n')
            self.assertNotEqual(initial['sha256'], snapshot(root)['sha256'])
            extra.unlink()
            self.assertEqual(initial, snapshot(root))
            source.chmod(0o700)
            self.assertNotEqual(initial['sha256'], snapshot(root)['sha256'])

    def test_symlink_target_is_recorded_without_reading_outside_module(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            link = root / 'source'
            link.symlink_to('/missing/first')
            first = snapshot(root)
            self.assertEqual(first['entries'][0]['mode'], 'symlink')
            link.unlink()
            link.symlink_to('/missing/second')
            self.assertNotEqual(first['sha256'], snapshot(root)['sha256'])

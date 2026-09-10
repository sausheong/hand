import unittest
import subprocess
import tempfile
from pathlib import Path
from evidence import coverage_metrics, source_snapshot


class CoverageTests(unittest.TestCase):
    def test_generated_python_cache_does_not_change_source_fingerprint(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(['git', 'init', str(root)], check=True, capture_output=True)
            (root/'.gitignore').write_bytes((Path(__file__).resolve().parents[1]/'.gitignore').read_bytes())
            source = root/'runner.py'
            source.write_text('print("source")\n')
            before = source_snapshot(root)
            (root/'__pycache__').mkdir()
            (root/'__pycache__/runner.cpython-314.pyc').write_bytes(b'generated cache')
            (root/'runner.pyc').write_bytes(b'legacy cache')
            self.assertEqual(source_snapshot(root), before)
            source.write_text('print("changed source")\n')
            self.assertNotEqual(source_snapshot(root)['sha256'], before['sha256'])

    def test_weighted_total_not_average_packages(self):
        r=coverage_metrics('mode: atomic\nm/a/a.go:1.1,2.2 9 1\nm/b/b.go:1.1,2.2 1 0\n')
        self.assertEqual(r['total']['percentage'],90)

    def test_duplicate_blocks_count_once(self):
        r=coverage_metrics('mode: atomic\nm/a.go:1.1,2.2 3 0\nm/a.go:1.1,2.2 3 1\n')
        self.assertEqual(r['total']['statements'],3)
        self.assertEqual(r['total']['percentage'],100)

    def test_conflicting_counts_rejected(self):
        with self.assertRaises(ValueError):
            coverage_metrics('mode: atomic\nm/a.go:1.1,2.2 3 0\nm/a.go:1.1,2.2 4 1\n')

    def test_empty_is_unknown(self):
        self.assertIsNone(coverage_metrics('mode: atomic\n')['total']['percentage'])

    def test_negative_rejected(self):
        with self.assertRaises(ValueError): coverage_metrics('mode: atomic\nm/a.go:1.1,2.2 3 -1\n')

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from evaluation_baseline import verify_baseline
from evaluation_checkout import git
from evaluation_go_tests import verify_test_events
from evaluation_manifest import validate

DRIVER = Path(__file__).with_name('evaluation_unittest.py').resolve()
CATALOGUE = DRIVER.parent.parent/'docs/acceptance/evaluation/calibration.json'


class PythonVerificationTests(unittest.TestCase):
    def run_oracle(self, code, expected='test_oracle.Oracle.test_value'):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'test_oracle.py').write_text(code)
            command = subprocess.run([sys.executable, '-I', str(DRIVER), '--start-directory', str(root)],
                                     capture_output=True, text=True, timeout=10)
        execution = dict(status='completed' if command.returncode == 0 else 'failed',
                         exit_code=command.returncode, process_and_pipes_joined=True)
        report = verify_test_events(command.stdout, execution,
                                   [dict(package='python.unittest', test=expected)])
        return command, report

    def test_successful_subtests_and_prints_preserve_json(self):
        command, report = self.run_oracle('''import unittest
class Oracle(unittest.TestCase):
 def test_value(self):
  print('ordinary test output')
  for value in (1, 2):
   with self.subTest(value=value): self.assertGreater(value, 0)
''')
        self.assertEqual(report['status'], 'passed', report)
        self.assertIn('ordinary test output', command.stderr)

    def test_failed_subtest_and_cleanup_fail(self):
        for body in ["with self.subTest(value=1): self.assertEqual(1, 2)",
                     "self.addCleanup(lambda: self.fail('cleanup failed'))"]:
            with self.subTest(body=body):
                command, report = self.run_oracle('import unittest\nclass Oracle(unittest.TestCase):\n def test_value(self):\n  '+body+'\n')
                self.assertNotEqual(command.returncode, 0)
                self.assertEqual(report['status'], 'failed')

    def test_skip_expected_failure_and_unexpected_success_fail(self):
        for decorator, body in [("unittest.skip('not available')", 'pass'),
                                ('unittest.expectedFailure', 'self.fail()'),
                                ('unittest.expectedFailure', 'pass')]:
            command, report = self.run_oracle('import unittest\nclass Oracle(unittest.TestCase):\n @'+decorator+'\n def test_value(self): '+body+'\n')
            self.assertNotEqual(command.returncode, 0)
            self.assertEqual(report['status'], 'failed')

    def test_empty_selection_and_import_error_fail(self):
        for code in ['', 'raise RuntimeError("cannot import oracle")\n']:
            command, report = self.run_oracle(code)
            self.assertNotEqual(command.returncode, 0)
            self.assertEqual(report['status'], 'failed')
            self.assertTrue(report['missing'])

    def test_class_setup_failure_cannot_hide_inventory(self):
        command, report = self.run_oracle('''import unittest
class Oracle(unittest.TestCase):
 @classmethod
 def setUpClass(cls): raise RuntimeError('setup failed')
 def test_value(self): pass
''')
        self.assertNotEqual(command.returncode, 0)
        self.assertEqual(report['status'], 'failed')
        self.assertTrue(report['missing'])

    def test_missing_expected_test_rejects_zero_exit(self):
        command, report = self.run_oracle('import unittest\nclass Oracle(unittest.TestCase):\n def test_different(self): pass\n')
        self.assertEqual(command.returncode, 0)
        self.assertEqual(report['status'], 'failed')
        self.assertTrue(report['missing'])

    def test_pinned_baseline_and_candidate_use_python_oracle(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            source = root/'source'; source.mkdir()
            git(source, 'init', '--template=', '--quiet')
            (source/'value.py').write_text('VALUE = 41\n')
            git(source, 'add', '.')
            git(source, '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '--quiet', '-m', 'fixture')
            commit = git(source, 'rev-parse', 'HEAD').decode().strip()
            (root/'oracle.py').write_text('import unittest\nfrom value import VALUE\nclass Oracle(unittest.TestCase):\n def test_value(self): self.assertEqual(VALUE, 42)\n')
            (root/'driver.py').write_bytes(DRIVER.read_bytes())
            manifest = json.loads(CATALOGUE.read_text())
            task = manifest['tasks'][0]; manifest['tasks'] = [task]
            task['repository'] = dict(url='local-python-test-fixture', commit=commit)
            task['working_directory'] = '.'
            task['fixtures'] = [dict(source=name, destination=dest,
                                     sha256=hashlib.sha256((root/name).read_bytes()).hexdigest())
                                for name, dest in [('oracle.py', 'test_oracle.py'), ('driver.py', 'driver.py')]]
            task['verification'] = [dict(argv=['python3', '-I', 'driver.py', '--start-directory', '.'],
                                         timeout_seconds=10, expected_exit=0, result_format='python_unittest_json',
                                         expected_tests=[dict(package='python.unittest', test='test_oracle.Oracle.test_value')])]
            path = root/'manifest.json'; path.write_text(json.dumps(manifest))
            environment = dict(PATH=os.environ['PATH'], PYTHONDONTWRITEBYTECODE='1')
            before = verify_baseline(path, task['id'], source, root/'before', environment)
            self.assertEqual(before['status'], 'baseline_tests_failed')
            (source/'value.py').write_text('VALUE = 42\n')
            patch = root/'fix.patch'; patch.write_bytes(git(source, 'diff', '--binary'))
            after = verify_baseline(path, task['id'], source, root/'after', environment, patch)
            self.assertEqual(after['status'], 'candidate_tests_passed', after)
            task['verification'][0]['argv'].remove('-I')
            with self.assertRaises(ValueError): validate(manifest, root)


if __name__ == '__main__': unittest.main()

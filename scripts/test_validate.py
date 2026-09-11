import unittest
import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
from unittest.mock import patch
import validate
from validate import check_events, platform_changes
from critical_coverage import GROUPS


class InventoryTests(unittest.TestCase):
    def test_platform_changes_excludes_other_os_and_architecture(self):
        changes = {
            'plain.go': [(1, 1)],
            'rename_linux.go': [(1, 1)],
            'rename_darwin.go': [(1, 1)],
            'peer_linux_amd64.go': [(1, 1)],
            'peer_linux_arm64.go': [(1, 1)],
        }
        self.assertEqual(platform_changes(changes, 'linux', 'amd64'), {
            'plain.go': [(1, 1)],
            'rename_linux.go': [(1, 1)],
            'peer_linux_amd64.go': [(1, 1)],
        })

    def test_missing_cannot_pass(self):
        self.assertEqual(check_events([], {('p', 'TestRequired')})['missing'], [('p', 'TestRequired')])

    def test_skip_cannot_pass(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action='skip')], {('p', 'TestRequired')})
        self.assertTrue(result['skipped'])
        self.assertTrue(result['missing'])

    def test_failure_not_erased_by_later_pass(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action=a) for a in ['fail', 'pass']], {('p', 'TestRequired')})
        self.assertTrue(result['failed'])

    def test_full_success(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action='pass'),
                               dict(Package='p', Action='pass')], {('p', 'TestRequired')})
        self.assertFalse(result['missing'])
        self.assertFalse(result['failed'])
        self.assertFalse(result['missing_packages'])

    def test_inventory_includes_fuzz_seeds_and_runnable_examples(self):
        listing = 'Test\nTestRequired\nFuzzFrame\nExampleClient\nExample\nBenchmarkParse\nok example/module 0.1s\n'
        self.assertEqual(validate.inventory_names(listing),
                         {'Test', 'TestRequired', 'FuzzFrame', 'ExampleClient', 'Example'})

    def test_unfinished_subtest_cannot_hide_behind_parent_pass(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action='pass'),
                               dict(Package='p', Test='TestRequired/unfinished', Action='run'),
                               dict(Package='p', Action='pass')], {('p', 'TestRequired')})
        self.assertEqual(result['unfinished'], [('p', 'TestRequired/unfinished')])

    def test_package_must_finish_after_named_tests(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action='pass')], {('p', 'TestRequired')})
        self.assertEqual(result['missing_packages'], ['p'])

    def test_missing_fuzz_or_example_is_a_required_gap(self):
        expected = {('p', 'TestRequired'), ('p', 'FuzzFrame'), ('p', 'ExampleClient')}
        result = check_events([dict(Package='p', Test='TestRequired', Action='pass'),
                               dict(Package='p', Action='pass')], expected)
        self.assertEqual(result['missing'], [('p', 'ExampleClient'), ('p', 'FuzzFrame')])

    def test_skipped_subtest_is_detected(self):
        result = check_events([dict(Package='p', Test='TestRequired', Action='pass'),
                               dict(Package='p', Test='TestRequired/case', Action='skip')], {('p', 'TestRequired')})
        self.assertTrue(result['skipped'])

    def test_failed_or_skipped_run_retains_coverage_without_passing(self):
        # Even exit zero and named-test passes cannot qualify a log missing
        # its package completion event (the final case).
        for action, exit_code in [('fail', 1), ('skip', 0), ('pass', 1), ('pass', 0)]:
            with self.subTest(action=action, exit_code=exit_code), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                root, output = base / 'source', base / 'evidence'
                mapping = root / 'docs/acceptance/critical-coverage-map.json'
                mapping.parent.mkdir(parents=True)
                (mapping.parent / 'coverage-scope.json').write_text(json.dumps(
                    {'schema_version': 1, 'test_fixtures': []}))
                mapping.write_text(json.dumps({'schema_version': 1, 'groups': {
                    name: {'sources': {'fixture': ['production.go']}, 'pending_capabilities': []}
                    for name in GROUPS}}))

                def run(argv, **kwargs):
                    text, code = '', 0
                    if argv[:2] == ['git', 'rev-parse']:
                        text = 'a' * 40 + '\n'
                    elif argv[:2] == ['go', 'version']:
                        text = 'go version fixture\n'
                    elif argv[:2] == ['go', 'env']:
                        text = json.dumps({'GOOS': 'linux', 'GOARCH': 'amd64'})
                    elif argv[:2] == ['go', 'list']:
                        text = 'fixture\n'
                    elif argv[:3] == ['go', 'test', '-list']:
                        text = 'TestRequired\n'
                    elif argv[:2] == ['go', 'test'] and '-race' not in argv:
                        # The separate, uninstrumented speed gate succeeds;
                        # these cases exercise failure of the coverage suite.
                        text = 'ok fixture\n'
                    elif argv[:2] == ['go', 'test']:
                        (output / 'coverage.out').write_text('mode: atomic\nfixture/production.go:1.1,2.1 1 1\n')
                        text = json.dumps(dict(Package='fixture', Test='TestRequired', Action=action)) + '\n'
                        code = exit_code
                    elif argv[0] not in ('git', 'go', 'gofmt'):
                        self.fail('failed test gate continued to downstream qualification')
                    kwargs['stdout'].write(text)
                    return subprocess.CompletedProcess(argv, code)

                with patch.object(validate, 'ROOT', root), patch.object(validate.subprocess, 'run', side_effect=run), \
                        patch.object(validate, 'source_snapshot', return_value={'sha256': 'snapshot'}), \
                        patch.object(validate, 'changed_lines', return_value={'production.go': [(1, 2)]}), \
                        patch.object(validate.sys, 'argv', ['validate', '--output', str(output)]), \
                        contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                    self.assertEqual(validate.main(), 1)
                report = json.loads((output / 'report.json').read_text())
                self.assertEqual(report['status'], 'failed')
                self.assertFalse(report['coverage_from_qualified_tests'])
                self.assertEqual(report['coverage']['total']['percentage'], 100)
                self.assertTrue((output / 'critical-coverage.json').is_file())
                self.assertTrue((output / 'changed-coverage.json').is_file())


if __name__ == '__main__': unittest.main()

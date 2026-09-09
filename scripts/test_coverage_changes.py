import subprocess
import hashlib
import tempfile
import unittest
from pathlib import Path
from coverage_changes import changed_lines, changed_coverage, classify_changed_sources


class ChangedCoverageTests(unittest.TestCase):
    def test_input_cannot_spoof_zero_counter_metadata(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root/'spoof.go').write_text('package p\nvar text = `\n'
                'var HandChangedCoverage = struct {\n'
                'Count [0]uint32\nPos [3 * 0]uint32\nNumStmt [0]uint16\n}\n`\n'
                'func F() int { return 1 }\n')
            (root/'types.go').write_text('package p\ntype Message struct{}\n')
            (root/'link.go').symlink_to(root/'types.go')
            report = changed_coverage('mode: atomic\n', 'm',
                {'spoof.go': [(1, 20)], 'link.go': [(1, 2)]}, root=root)
            self.assertEqual(report['missing_profile_files'], ['link.go', 'spoof.go'])
            self.assertEqual(report['zero_counter_sources'], [])
            self.assertIsNone(report['percentage'])

    def test_real_instrumenter_distinguishes_declarations_from_missing_code(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            sources = {
                'types.go': 'package p\ntype Message struct { Text string }\n',
                'platform_other.go': '//go:build never\n\npackage p\nfunc F() int { return 1 }\n',
                'initializer.go': 'package p\nvar x = func() int { return 1 }()\n',
                'invalid.go': 'package p\nfunc broken(\n',
            }
            for name, source in sources.items():
                (root/name).write_text(source)
            changes = {name: [(1, 9)] for name in [*sources, 'absent.go']}
            report = changed_coverage('mode: atomic\n', 'm', changes, root=root)
            self.assertEqual(report['missing_profile_files'],
                             ['absent.go', 'initializer.go', 'invalid.go', 'platform_other.go'])
            self.assertEqual([p['path'] for p in report['zero_counter_sources']], ['types.go'])
            self.assertEqual(report['zero_counter_sources'][0]['sha256'],
                             hashlib.sha256((root/'types.go').read_bytes()).hexdigest())
            self.assertIsNone(report['percentage'])
            # Adding executable code invalidates the earlier zero-counter proof.
            (root/'types.go').write_text('package p\nfunc F() int { return 1 }\n')
            report = changed_coverage('mode: atomic\n', 'm', {'types.go': [(1, 9)]}, root=root)
            self.assertEqual(report['missing_profile_files'], ['types.go'])
            self.assertEqual(report['zero_counter_sources'], [])

    def test_only_pinned_fixture_excluded_platform_and_unclassified_files_remain(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            helper = root / 'internal/testdata/helper.go'
            helper.parent.mkdir(parents=True)
            helper.write_text('package main\nfunc main() {}\n')
            entry = dict(path='internal/testdata/helper.go',
                         sha256=hashlib.sha256(helper.read_bytes()).hexdigest(), reason='Test driver')
            changes = {name: [(1, 2)] for name in [entry['path'], 'production.go',
                'platform_linux.go', 'internal/testdata/unclassified.go']}
            result = classify_changed_sources(root, changes, dict(schema_version=1, test_fixtures=[entry]))
            self.assertEqual(set(result['production_changes']), set(changes) - {entry['path']})
            self.assertEqual(result['excluded_tests'], [dict(entry, changed_lines=[(1, 2)])])
            report = changed_coverage('mode: atomic\nm/production.go:1.1,2.1 1 1\n',
                                      'm', result['production_changes'])
            self.assertEqual(report['missing_profile_files'],
                             ['internal/testdata/unclassified.go', 'platform_linux.go'])
            self.assertEqual(report['status'], 'missing_profile_files')
            helper.write_text('package main\nfunc main() { panic("changed") }\n')
            with self.assertRaisesRegex(ValueError, 'changed since classification'):
                classify_changed_sources(root, changes, dict(schema_version=1, test_fixtures=[entry]))

    def test_scope_rejects_production_paths_unsafe_paths_and_missing_reasons(self):
        for path, reason in [('production.go', 'coverage'), ('../testdata/a.go', 'fixture'),
                             ('/testdata/a.go', 'fixture'), ('testdata/a.go', ''),
                             ('testdata/*.go', 'wildcard')]:
            with self.subTest(path=path), tempfile.TemporaryDirectory() as folder:
                with self.assertRaises(ValueError):
                    classify_changed_sources(Path(folder), {}, dict(schema_version=1,
                        test_fixtures=[dict(path=path, sha256='0'*64, reason=reason)]))

    def test_scope_rejects_symlink_and_duplicate_entries(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            driver = root / 'testdata/helper.go'
            driver.parent.mkdir()
            driver.write_text('package main\n')
            entry = dict(path='testdata/helper.go', sha256=hashlib.sha256(driver.read_bytes()).hexdigest(), reason='Driver')
            with self.assertRaisesRegex(ValueError, 'duplicate'):
                classify_changed_sources(root, {}, dict(schema_version=1, test_fixtures=[entry, entry]))
            driver.rename(root / 'original.go')
            driver.symlink_to(root / 'original.go')
            with self.assertRaisesRegex(ValueError, 'symlink'):
                classify_changed_sources(root, {}, dict(schema_version=1, test_fixtures=[entry]))

    def test_missed_branch_and_duplicate_blocks(self):
        profile = ('mode: atomic\nm/a.go:1.1,4.2 9 1\nm/a.go:5.1,7.2 1 0\n'
                   'm/a.go:1.1,4.2 9 0\n')
        result = changed_coverage(profile, 'm', {'a.go': [(3, 5)]})
        self.assertEqual(result['percentage'], 90)
        self.assertEqual(result['statements'], 10)
        self.assertEqual(result['status'], 'measured')

    def test_unknown_is_not_zero_or_full_coverage(self):
        result = changed_coverage('mode: atomic\n', 'm', {'new.go': [(1, 5)]})
        self.assertEqual(result['missing_profile_files'], ['new.go'])
        self.assertIsNone(result['percentage'])
        self.assertEqual(result['status'], 'missing_profile_files')

    def test_rejects_foreign_and_conflicting_profiles(self):
        for profile in ['mode: atomic\nother/a.go:1.1,2.2 1 1\n',
                        'mode: atomic\nm/a.go:1.1,2.2 1 1\nm/a.go:1.1,2.2 2 1\n']:
            with self.assertRaises(ValueError):
                changed_coverage(profile, 'm', {'a.go': [(1, 2)]})

    def test_real_git_diff_includes_untracked_and_excludes_tests(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            def git(*args):
                return subprocess.check_output(['git', '-C', folder, *args], stderr=subprocess.DEVNULL)
            git('init')
            (root/'a.go').write_text('package a\nfunc a() int {\n return 1\n}\n')
            git('add', '.')
            git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'base')
            (root/'a.go').write_text('package a\nfunc a() int {\n return 2\n}\n')
            (root/'new.go').write_text('package a\nfunc b() {}\n')
            (root/'a_test.go').write_text('package a\n')
            self.assertEqual(changed_lines(root, 'HEAD'), {'a.go': [(3, 3)], 'new.go': [(1, 2)]})
            with self.assertRaises(subprocess.CalledProcessError):
                changed_lines(root, 'missing-baseline')

import copy
import unittest
import tempfile
from pathlib import Path
from critical_coverage import GROUPS, critical_coverage


def mapping():
    return {'schema_version': 1, 'groups': {name: {
        'sources': {'m': ['a/*.go', 'a/one.go']}, 'pending_capabilities': []
    } for name in GROUPS}}


class CriticalCoverageTests(unittest.TestCase):
    def test_exact_declarations_require_instrumenter_proof(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'types.go').write_text('package sample\ntype Event struct { ID string }\n')
            spec = mapping()
            group = spec['groups']['lifecycle']
            group['sources']['m'].append('types.go')
            profiles = {'m': 'mode: atomic\nm/a/one.go:1.1,2.2 4 1\n'}
            self.assertEqual(critical_coverage(profiles, spec)['lifecycle']['status'], 'incomplete')
            result = critical_coverage(profiles, spec, roots={'m': root})['lifecycle']
            self.assertEqual((result['status'], result['statements'], result['covered']), ('measured', 4, 4))
            proof, = result['zero_counter_sources']
            self.assertEqual((proof['path'], proof['counters']), ('types.go', 0))
            self.assertEqual(len(proof['sha256']), 64)
            group['pending_capabilities'].append('final platform qualification')
            self.assertEqual(critical_coverage(profiles, spec, roots={'m': root})['lifecycle']['status'], 'incomplete')

    def test_executable_missing_unsafe_and_wildcard_sources_remain_gaps(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'types.go').write_text('package sample\ntype Event struct{}\n')
            (root / 'exec.go').write_text('//go:build linux\n\npackage sample\nfunc value() int { return 1 }\n')
            (root / 'alias.go').symlink_to(root / 'types.go')
            for pattern in ['exec.go', 'missing.go', 'alias.go', 'types*.go']:
                with self.subTest(pattern=pattern):
                    spec = mapping()
                    spec['groups']['lifecycle']['sources']['m'].append(pattern)
                    result = critical_coverage({'m': 'mode: atomic\nm/a/one.go:1.1,2.2 4 1\n'}, spec,
                                               roots={'m': root})['lifecycle']
                    self.assertEqual(result['status'], 'incomplete')
                    self.assertIn('m/' + pattern, result['unmatched_patterns'])
                    self.assertEqual(result['zero_counter_sources'], [])

    def test_zero_counters_do_not_replace_a_module_profile_or_measured_statements(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'types.go').write_text('package sample\ntype Event struct{}\n')
            spec = mapping()
            spec['groups']['lifecycle']['sources'] = {'m': ['types.go']}
            for profiles in [{}, {'m': 'mode: atomic\n'}]:
                result = critical_coverage(profiles, spec, roots={'m': root})['lifecycle']
                self.assertEqual(result['status'], 'incomplete')
                self.assertIsNone(result['percentage'])

    def test_overlap_and_duplicate_binaries_do_not_inflate_coverage(self):
        profile = ('mode: atomic\nm/a/one.go:1.1,2.2 9 1\nm/a/one.go:1.1,2.2 9 0\n'
                   'm/a/two.go:1.1,2.2 1 0\nm/unrelated.go:1.1,2.2 100 1\n')
        r = critical_coverage({'m': profile}, mapping())['lifecycle']
        self.assertEqual((r['statements'], r['covered'], r['percentage']), (10, 9, 90))

    def test_missing_module_file_and_implementation_never_report_percentage(self):
        for mutate in [lambda g: g['sources'].update({'absent': ['x.go']}),
                       lambda g: g['sources']['m'].append('absent.go'),
                       lambda g: g['pending_capabilities'].append('not implemented'),
                       lambda g: g.update(sources={})]:
            spec = mapping()
            mutate(spec['groups']['lifecycle'])
            r = critical_coverage({'m': 'mode: atomic\nm/a/one.go:1.1,2.2 1 1\n'}, spec)['lifecycle']
            self.assertEqual(r['status'], 'incomplete')
            self.assertIsNone(r['percentage'])

    def test_all_nine_groups_must_be_accounted_for(self):
        spec = mapping()
        del spec['groups']['budgets']
        with self.assertRaises(ValueError): critical_coverage({}, spec)

    def test_conflicting_profile_rejected(self):
        with self.assertRaises(ValueError):
            critical_coverage({'m': 'mode: atomic\nm/a.go:1.1,2.2 1 1\nm/a.go:1.1,2.2 2 1\n'}, mapping())

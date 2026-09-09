import copy
import json
from pathlib import Path
import tempfile
import unittest
from evaluation_manifest import validate

ROOT=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation'


class EvaluationManifestTests(unittest.TestCase):
    def setUp(self):
        self.manifest=json.loads((ROOT/'calibration.json').read_text())

    def test_real_calibration_fixture_is_draft_not_qualification(self):
        result=validate(self.manifest,ROOT)
        self.assertEqual(result['status'],'draft')
        self.assertEqual(result['planned_runs'],0)
        self.assertTrue(result['qualification_gaps'])

    def test_verification_format_and_inventory_are_frozen(self):
        for change in [lambda c:c.pop('result_format'),
                       lambda c:c.update(result_format='unimplemented'),
                       lambda c:c['argv'].remove('-json'),
                       lambda c:c['argv'].remove('-count=1'),
                       lambda c:c.update(expected_tests=[]),
                       lambda c:c.update(expected_tests=c['expected_tests']*2),
                       lambda c:c.update(expected_tests=[{'package':'fixture','test':''}])]:
            manifest=copy.deepcopy(self.manifest)
            change(manifest['tasks'][0]['verification'][0])
            with self.assertRaises(ValueError): validate(manifest,ROOT)

    def test_missing_commit_duplicate_ids_and_path_escape_fail(self):
        changes=[lambda m:m['tasks'][0]['repository'].update(commit='main'),
                 lambda m:m['tasks'].append(copy.deepcopy(m['tasks'][0])),
                 lambda m:m['tasks'][0]['fixtures'][0].update(destination='../outside.go'),
                 lambda m:m['tasks'][0]['fixtures'][0].update(sha256='0'*64),
                 lambda m:m['per_run_budget'].update(timeout_seconds=True),
                 lambda m:m['per_run_budget'].update(max_cost_usd=float('nan'))]
        for change in changes:
            manifest=copy.deepcopy(self.manifest);change(manifest)
            with self.assertRaises(ValueError):validate(manifest,ROOT)

    def test_noninteger_versions_and_unrepresentable_money_are_rejected(self):
        for version in [True, 1.0, '1', None]:
            with self.subTest(version=version):
                manifest=copy.deepcopy(self.manifest)
                manifest['schema_version']=version
                with self.assertRaisesRegex(ValueError,'unsupported manifest version'):
                    validate(manifest,ROOT)
        for value in [10**1000, float('inf'), float('-inf'), True, -1]:
            for field in ['max_cost_usd','planned_total_usd']:
                with self.subTest(value=str(value),field=field):
                    manifest=copy.deepcopy(self.manifest)
                    target=manifest['per_run_budget'] if field=='max_cost_usd' else manifest
                    target[field]=value
                    with self.assertRaisesRegex(ValueError,'finite nonnegative'):
                        validate(manifest,ROOT)

    def test_fixture_destinations_are_canonical_files_outside_git_metadata(self):
        for destination in ['.', './oracle.go', 'tests//oracle.go', 'tests/oracle.go/',
                            '.git/config', 'nested/.GIT/hooks/post-checkout', 'test\x00.go']:
            with self.subTest(destination=destination):
                manifest=copy.deepcopy(self.manifest)
                manifest['tasks'][0]['fixtures'][0]['destination']=destination
                with self.assertRaisesRegex(ValueError,'safe fixture paths'):
                    validate(manifest,ROOT)
        for parent_first in [True,False]:
            manifest=copy.deepcopy(self.manifest)
            first=manifest['tasks'][0]['fixtures'][0]
            first['destination']='tests' if parent_first else 'tests/oracle.go'
            second=copy.deepcopy(first)
            second['destination']='tests/oracle.go' if parent_first else 'tests'
            manifest['tasks'][0]['fixtures'].append(second)
            with self.assertRaisesRegex(ValueError,'overlapping fixture destinations'):
                validate(manifest,ROOT)

    def test_fixture_sources_cannot_follow_symlinks_even_inside_root(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            (root/'testdata').mkdir()
            manifest=copy.deepcopy(self.manifest)
            for task in manifest['tasks']:
                for fixture in task['fixtures']:
                    destination=root/fixture['source']
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes((ROOT/fixture['source']).read_bytes())
            self.assertEqual(validate(manifest,root)['status'],'draft')
            (root/'alias').symlink_to(root/'testdata',target_is_directory=True)
            manifest['tasks'][0]['fixtures'][0]['source']='alias/write_mode_test.go'
            with self.assertRaisesRegex(ValueError,'symlink components'):
                validate(manifest,root)
            manifest['tasks'][0]['fixtures'][0]['source']='linked.go'
            (root/'linked.go').symlink_to(root/'testdata/write_mode_test.go')
            with self.assertRaisesRegex(ValueError,'symlink components'):
                validate(manifest,root)

    def test_synthetic_qualification_dimensions_and_budget_are_enforced(self):
        # Structural validator fixture only; never model-performance evidence.
        m=copy.deepcopy(self.manifest);m['purpose']='qualification';m['repetitions']=3
        m['agents']=[{'id':name,'repository':'fixture','commit':'a'*40,'model':{'provider':'fixture','id':'fixture','reasoning':'none'}} for name in ['hand','pi']]
        m['modes']=[{'id':mode,'agents':{name:{'tools':['read'],'settings':{},'packages':[]} for name in ['hand','pi']}} for mode in ['defaults','configured']]
        categories=['bug_fix','feature','refactor','tests','discovery','continuation']
        m['tasks']=[]
        for i in range(30):
            task=copy.deepcopy(self.manifest['tasks'][0]);task.update(id=str(i),split='held_out' if i<10 else 'calibration',category=categories[i%6]);m['tasks'].append(task)
        result=validate(m,ROOT)
        self.assertEqual(result['status'],'qualification_inputs_valid')
        self.assertEqual(result['planned_runs'],360)
        m['per_run_budget']['max_cost_usd']=0.07
        m['planned_total_usd']=25.20
        self.assertEqual(validate(m,ROOT)['status'],'qualification_inputs_valid')
        m['planned_total_usd']=25.199999999999996
        self.assertIn('planned total does not cover all per-run caps',validate(m,ROOT)['qualification_gaps'])
        m['per_run_budget']['max_cost_usd']=1
        self.assertIn('planned total does not cover all per-run caps',validate(m,ROOT)['qualification_gaps'])
        self.assertIn('Not granted',result['authorisation'])

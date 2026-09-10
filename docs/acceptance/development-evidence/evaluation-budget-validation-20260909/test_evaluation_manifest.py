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

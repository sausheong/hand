import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
import test_evaluation_schedule as fixtures
from evaluation_schedule import schedule
from evaluation_results import analyse


class ResultTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        # Synthetic protocol fixture only: never an evaluation corpus or result.
        self.manifest = fixtures.ScheduleTests().fixture()
        self.plan = schedule(self.manifest, fixtures.ROOT)
        artifact = self.root / 'synthetic.txt'
        artifact.write_text('Synthetic reporter verification fixture; no agent ran.\n')
        pin = hashlib.sha256(artifact.read_bytes()).hexdigest()
        self.results = dict(schema_version=1, manifest_sha256=self.plan['manifest_sha256'], runs=[])
        for run in self.plan['runs']:
            hand = run['agent']['id'] == 'hand'
            self.results['runs'].append(dict(run_id=run['id'], outcome='completed' if hand else 'failed',
                tests='passed' if hand else 'failed', review='passed', regressions=0, cost_usd=.5,
                elapsed_seconds=10, interventions=1, recovery_attempted=False, recovery_succeeded=None,
                evidence=[dict(path='synthetic.txt', sha256=pin)]))

    def report(self):
        return analyse(self.manifest, fixtures.ROOT, self.results, self.root)

    def test_complete_records_keep_modes_splits_and_paired_task_clusters(self):
        result = self.report()
        self.assertEqual(result['status'], 'records_complete_unreviewed')
        self.assertEqual(result['recorded_runs'], 360)
        self.assertEqual(result['missing_run_ids'], [])
        self.assertEqual(result['groups']['defaults/all/hand']['verified_success_rate'], 1)
        self.assertEqual(result['groups']['defaults/all/pi']['verified_success_rate'], 0)
        self.assertEqual(result['groups']['defaults/all/hand']['cost_per_verified_success_usd'], .5)
        self.assertIsNone(result['groups']['defaults/all/pi']['cost_per_verified_success_usd'])
        self.assertEqual(result['paired_comparisons']['defaults/all']['task_clusters'], 30)
        self.assertEqual(result['paired_comparisons']['defaults/held_out']['task_clusters'], 10)
        self.assertEqual(result['paired_comparisons']['defaults/all']['interval_95'], [1, 1])
        self.assertEqual(result, self.report())

    def test_missing_runs_cannot_improve_denominator(self):
        missing = self.plan['runs'][0]
        self.results['runs'].pop(0)
        result = self.report()
        self.assertEqual(result['status'], 'incomplete')
        self.assertEqual(result['missing_run_ids'], [missing['id']])
        key = missing['mode'] + '/all/' + missing['agent']['id']
        self.assertIsNone(result['groups'][key]['verified_success_rate'])
        self.assertIsNone(result['groups'][key]['cost_per_verified_success_usd'])
        self.assertEqual(result['paired_comparisons'][missing['mode']+'/all']['status'], 'incomplete')

    def test_unverified_regressions_and_failures_stay_in_cost_denominator(self):
        hand = next(r for r in self.plan['runs'] if r['agent']['id'] == 'hand')
        record = next(r for r in self.results['runs'] if r['run_id'] == hand['id'])
        record.update(tests='skipped', review='missing', regressions=1, outcome='timed_out', cost_usd=2,
                      recovery_attempted=True, recovery_succeeded=False)
        result = self.report()
        group = result['groups'][hand['mode']+'/all/hand']
        self.assertEqual(group['verified_successes'], 89)
        self.assertEqual(group['recorded_outcomes']['timed_out'], 1)
        self.assertEqual(group['recorded_regressions'], 1)
        self.assertEqual(group['incomplete_verifications'], 1)
        self.assertAlmostEqual(group['cost_per_verified_success_usd'], (89*.5+2)/89)
        self.assertEqual(group['recovery_success_rate'], 0)
        self.assertEqual(len(result['cost_cap_violations']), 1)

    def test_unknown_cost_and_recovery_are_not_zero(self):
        run = self.plan['runs'][0]
        self.results['runs'][0].update(cost_usd=None, recovery_attempted=True, recovery_succeeded=None)
        group = self.report()['groups'][run['mode']+'/all/'+run['agent']['id']]
        self.assertEqual(group['unknown_cost_runs'], 1)
        self.assertIsNone(group['total_cost_usd'])
        self.assertIsNone(group['cost_per_verified_success_usd'])
        self.assertIsNone(group['recovery_success_rate'])

    def test_duplicate_unknown_and_changed_identity_fail(self):
        original = copy.deepcopy(self.results)
        self.results['runs'].append(copy.deepcopy(self.results['runs'][0]))
        with self.assertRaises(ValueError): self.report()
        self.results = copy.deepcopy(original)
        self.results['runs'][0]['run_id'] = 'unknown'
        with self.assertRaises(ValueError): self.report()
        self.results = original
        self.manifest['agents'][0]['model']['id'] = 'changed'
        with self.assertRaisesRegex(ValueError, 'frozen manifest'): self.report()

    def test_bad_measurements_paths_symlinks_and_hashes_fail(self):
        original = copy.deepcopy(self.results['runs'][0])
        changes = [dict(cost_usd=float('nan')), dict(regressions=True), dict(elapsed_seconds=-1),
                   dict(recovery_succeeded=True), dict(interventions=1.2)]
        for change in changes:
            self.results['runs'][0] = dict(copy.deepcopy(original), **change)
            with self.subTest(change=change), self.assertRaises(ValueError): self.report()
        self.results['runs'][0] = copy.deepcopy(original)
        self.results['runs'][0]['evidence'][0]['path'] = '../synthetic.txt'
        with self.assertRaises(ValueError): self.report()
        self.results['runs'][0] = copy.deepcopy(original)
        (self.root/'synthetic.txt').rename(self.root/'original.txt')
        (self.root/'synthetic.txt').symlink_to(self.root/'original.txt')
        with self.assertRaisesRegex(ValueError, 'symlink'): self.report()
        (self.root/'synthetic.txt').unlink()
        (self.root/'synthetic.txt').write_text('changed')
        with self.assertRaisesRegex(ValueError, 'digest mismatch'): self.report()


if __name__ == '__main__': unittest.main()

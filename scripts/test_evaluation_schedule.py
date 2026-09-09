import copy
import json
from collections import Counter
from pathlib import Path
import unittest
from evaluation_schedule import schedule

ROOT=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation'

class ScheduleTests(unittest.TestCase):
    def fixture(self):
        # Structural scheduler test only. These repeated tasks are NOT a corpus.
        m=json.loads((ROOT/'calibration.json').read_text())
        m.update(purpose='qualification',repetitions=3,planned_total_usd=360)
        m['per_run_budget']['max_cost_usd']=1
        m['agents']=[dict(id=name,repository='test-fixture',commit='a'*40,
            model=dict(provider='fixture',id='fixture',reasoning='none')) for name in ['hand','pi']]
        m['modes']=[dict(id=mode,agents={name:dict(tools=['read'],settings={},packages=[]) for name in ['hand','pi']}) for mode in ['defaults','configured']]
        seed=copy.deepcopy(m['tasks'][0]);m['tasks']=[]
        for n in range(30):
            task=copy.deepcopy(seed);task.update(id=str(n),split='held_out' if n<10 else 'calibration',
                category=['bug_fix','feature','refactor','tests','discovery','continuation'][n%6]);m['tasks'].append(task)
        return m

    def test_every_pair_balanced_isolated_and_heldout_last(self):
        m=self.fixture();s=schedule(m,ROOT)
        self.assertEqual(s['run_count'],360);self.assertEqual(s['pair_count'],180)
        self.assertEqual(len({r['id'] for r in s['runs']}),360)
        self.assertEqual([r['position'] for r in s['runs']],list(range(360)))
        first=Counter((p['phase'],p['mode'],p['agent_order'][0]) for p in s['pairs'])
        for phase in ['calibration','held_out']:
            for mode in ['defaults','configured']:
                self.assertLessEqual(abs(first[phase,mode,'hand']-first[phase,mode,'pi']),1)
        for index,pair in enumerate(s['pairs']):
            a,b=s['runs'][2*index:2*index+2]
            self.assertEqual([a['id'],b['id']],pair['run_ids'])
            self.assertEqual(a['task_sha256'],b['task_sha256'])
            self.assertEqual(a['budget'],b['budget'])
            self.assertEqual({a['agent']['id'],b['agent']['id']},{'hand','pi'})
        phases=[r['phase'] for r in s['runs']]
        self.assertNotIn('calibration',phases[phases.index('held_out'):])
        s['runs'][0]['budget']['max_requests']=1
        self.assertEqual(m['per_run_budget']['max_requests'],20)
        self.assertEqual(s['runs'][1]['budget']['max_requests'],20)
        self.assertEqual(s['status'],'planned_not_authorised')

    def test_deterministic_and_input_changes_invalidate_identity(self):
        m=self.fixture();a=schedule(m,ROOT);self.assertEqual(a,schedule(m,ROOT))
        m['agents'][0]['model']['id']='changed';b=schedule(m,ROOT)
        self.assertNotEqual(a['manifest_sha256'],b['manifest_sha256'])
        self.assertFalse({r['id'] for r in a['runs']} & {r['id'] for r in b['runs']})

    def test_incomplete_or_underfunded_plan_cannot_be_scheduled(self):
        with self.assertRaises(ValueError):schedule(json.loads((ROOT/'calibration.json').read_text()),ROOT)
        m=self.fixture();m['planned_total_usd']=359
        with self.assertRaises(ValueError):schedule(m,ROOT)

if __name__=='__main__':unittest.main()

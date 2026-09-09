import gzip
import json
from pathlib import Path
import unittest
from evaluation_hand_events import HandRunObserver


class HandObserverTests(unittest.TestCase):
    def raw(self):
        return gzip.decompress((Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/hand-jsonl-development-20260909/gateway-read.jsonl.gz').read_bytes())

    def events(self):
        return [json.loads(line) for line in self.raw().split(b'\n') if line]

    def result(self, events):
        o = HandRunObserver()
        o.feed(b''.join((json.dumps(e)+'\n').encode() for e in events))
        return o.finish()

    def test_actual_binary_stream_in_chunks(self):
        o = HandRunObserver()
        for byte in self.raw():
            o.feed(bytes([byte]))
        r = o.finish()
        self.assertEqual(r['status'], 'observed_unverified', r)
        self.assertEqual(r['outcome'], 'completed')
        self.assertEqual(r['tool_results'], 1)
        self.assertEqual(r['tool_errors'], 0)

    def test_dropped_reordered_duplicated_and_truncated_events(self):
        e = self.events()
        for events in (e[:-1], e[1:], e[:3]+e[4:], e[:2]+[e[1]]+e[2:], e+[e[-1]], [e[1],e[0]]+e[2:]):
            self.assertEqual(self.result(events)['status'], 'incomplete')
        o = HandRunObserver();o.feed(self.raw().rstrip(b'\n'))
        self.assertEqual(o.finish()['status'], 'incomplete')

    def test_identity_version_and_sequence_rejected(self):
        for key, value in [('request_id','other'),('session_id','other'),('run_id','other'),('event_id',self.events()[0]['event_id']),('version',True),('sequence',True),('timestamp','2026-01-01')]:
            e=self.events();e[1][key]=value
            self.assertEqual(self.result(e)['status'],'incomplete',(key,value))

    def test_tool_mismatch_and_missing_result_rejected(self):
        for field,value in [('id','other'),('name','other')]:
            e=self.events();e[3]['payload']['tool'][field]=value
            self.assertEqual(self.result(e)['status'],'incomplete')
        e=self.events();e[3]['kind']='text'
        self.assertEqual(self.result(e)['status'],'incomplete')
        e=self.events();e[3]['payload']['result']['error']='read failed'
        r=self.result(e)
        self.assertEqual(r['status'],'observed_unverified')
        self.assertEqual(r['tool_errors'],1)  # Observation preserves failure, never scores success.

    def test_outcomes_remain_distinct(self):
        for outcome in ['verification_failed','budget_exhausted','cancelled','infrastructure_error']:
            e=self.events();e[-1]['payload']['status']=outcome
            r=self.result(e);self.assertEqual(r['status'],'observed_unverified');self.assertEqual(r['outcome'],outcome)
        e=self.events();e[-1]['payload']['status']='success'
        self.assertEqual(self.result(e)['status'],'incomplete')

    def test_strict_json_and_limits_poison_observation(self):
        for bad in [b'{}\n',b'{"version":1,"version":1}\n',b'{"a":NaN}\n',b'\xff\n',b'\n']:
            o=HandRunObserver();o.feed(bad+self.raw());self.assertEqual(o.finish()['status'],'incomplete')
        o=HandRunObserver(max_bytes=4)
        with self.assertRaises(ValueError):o.feed(b'12345')
        self.assertEqual(o.finish()['status'],'incomplete')
        with self.assertRaises(ValueError):o.feed(b'\n')


if __name__=='__main__':unittest.main()

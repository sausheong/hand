import unittest
from transcript_performance import summarise


def fixture(runs=30,events=1000,latency=1):
    # Parser fixtures only: these values never represent product performance.
    block=(f'BenchmarkTranscript10000Blocks-16 {events} 1000 ns/op {latency} p95-ms\n'
           'latency_ms_sorted=[1]\nlatency_ms_sorted=['+' '.join([str(latency)]*events)+']\n')
    return block*runs+'PASS\n'


class TranscriptPerformanceTests(unittest.TestCase):
    def test_full_workload_and_threshold(self):
        result=summarise(fixture())
        self.assertEqual(result['status'],'development_workload_passed')
        self.assertEqual(result['total_events'],30000)
        self.assertEqual(result['calibration_groups'],30)
        self.assertEqual(summarise(fixture(latency=101))['status'],'incomplete_or_failed')

    def test_short_workloads_do_not_qualify(self):
        for raw in [fixture(runs=29),fixture(events=999)]:
            self.assertFalse(summarise(raw)['workload_sufficient'])

    def test_failed_missing_and_disagreeing_raw_data_rejected(self):
        raw=fixture()
        for invalid in [raw.replace('PASS','FAIL'),raw.replace('PASS',''),
                        raw.replace('1000 ns/op 1 p95-ms','1000 ns/op 2 p95-ms',1),
                        raw.replace('latency_ms_sorted=[1 1','latency_ms_sorted=[nan 1',1),
                        raw.replace('latency_ms_sorted=[1 1','latency_ms_sorted=[1',1)]:
            with self.assertRaises(ValueError): summarise(invalid)

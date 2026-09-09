import gzip
import json
from pathlib import Path
import sys
import tempfile
import unittest
from evaluation_hand_run import run_hand


class HandRunTests(unittest.TestCase):
    def setUp(self):
        temporary=tempfile.TemporaryDirectory();self.addCleanup(temporary.cleanup)
        self.root=Path(temporary.name).resolve();self.workspace=self.root/'workspace';self.workspace.mkdir()
        fixture=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/hand-jsonl-development-20260909/gateway-read.jsonl.gz'
        self.raw=gzip.decompress(fixture.read_bytes())
        self.number=0

    def run_fixture(self, raw=None, tail='', **options):
        self.number+=1
        source=self.root/f'fixture-{self.number}.jsonl';source.write_bytes(self.raw if raw is None else raw)
        code='import sys,time; from pathlib import Path; sys.stdout.buffer.write(Path(sys.argv[1]).read_bytes()); sys.stdout.flush(); '+tail
        return run_hand([sys.executable,'-c',code,str(source)],self.workspace,self.root/f'run-{self.number}',{},
                        options.get('timeout_seconds',5),options.get('max_output_bytes',16384))

    def test_recorded_stream_with_successful_process(self):
        r=self.run_fixture()
        self.assertEqual(r['status'],'observed_unverified',r)
        self.assertTrue(r['process']['process_and_pipes_joined'])
        self.assertEqual(r['observation']['tool_results'],1)
        self.assertEqual(json.loads((self.root/'run-1/hand-run.json').read_text()),r)

    def test_success_terminal_cannot_override_process_failure(self):
        r=self.run_fixture(tail='sys.exit(7)')
        self.assertEqual(r['status'],'incomplete')
        self.assertEqual(r['process']['exit_code'],7)

    def test_exit_zero_cannot_override_missing_terminal(self):
        raw=b'\n'.join(self.raw.split(b'\n')[:-2])+b'\n'
        self.assertEqual(self.run_fixture(raw)['status'],'incomplete')

    def test_timeout_after_terminal_is_not_complete(self):
        r=self.run_fixture(tail='time.sleep(60)',timeout_seconds=0.2)
        self.assertEqual(r['status'],'incomplete')
        self.assertEqual(r['process']['status'],'timed_out')
        self.assertTrue(r['process']['process_and_pipes_joined'])
        self.assertEqual(r['observation']['outcome'],'completed')

    def test_output_flood_after_terminal_is_capped(self):
        r=self.run_fixture(tail='sys.stderr.write("x"*1000000)',max_output_bytes=8000)
        self.assertEqual(r['status'],'incomplete')
        self.assertEqual(r['process']['status'],'output_limited')
        self.assertEqual(r['process']['output_bytes'],8000)
        self.assertTrue(r['process']['process_and_pipes_joined'])

    def test_failed_outcomes_require_matching_exit(self):
        for outcome,code in [('verification_failed',3),('budget_exhausted',4),('cancelled',130),('infrastructure_error',5)]:
            with self.subTest(outcome=outcome):
                events=[json.loads(line) for line in self.raw.split(b'\n') if line]
                events[-1]['payload']['status']=outcome
                raw=b''.join((json.dumps(e)+'\n').encode() for e in events)
                r=self.run_fixture(raw,tail=f'sys.exit({code})')
                self.assertEqual(r['status'],'observed_unverified',r)
                self.assertEqual(r['observation']['outcome'],outcome)
                self.assertEqual(self.run_fixture(raw)['status'],'incomplete')


if __name__=='__main__':unittest.main()

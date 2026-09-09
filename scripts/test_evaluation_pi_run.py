import json
from pathlib import Path
import sys
import tempfile
import unittest
from evaluation_pi_run import run_pi
from test_evaluation_pi_events import transcript


class PiRunTests(unittest.TestCase):
    def setUp(self):
        t=tempfile.TemporaryDirectory();self.addCleanup(t.cleanup)
        self.root=Path(t.name).resolve();self.workspace=self.root/'workspace';self.workspace.mkdir()

    def invoke(self, code, **kwargs):
        return run_pi([sys.executable,'-c',code],self.workspace,self.root/'evidence',{},'test prompt','fixture','frozen-model',**kwargs)

    def client(self, tail=''):
        e=transcript()
        return ('import sys,json,time; p=json.loads(sys.stdin.readline()); assert p["type"]=="prompt"; '
                'events='+repr(e)+'; '
                'sys.stdout.write("".join(json.dumps(e)+"\\n" for e in events[:-1])); sys.stdout.flush(); '
                's=json.loads(sys.stdin.readline()); assert s["id"]=="stats-1"; '
                'print(json.dumps(events[-1]),flush=True); assert sys.stdin.read()==""; '+tail)

    def test_prompt_settlement_statistics_and_join(self):
        r=self.invoke(self.client())
        self.assertEqual(r['status'],'observed_unverified',r)
        self.assertEqual(r['observation']['outcome'],'completed')
        self.assertTrue(r['process']['process_and_pipes_joined'])

    def test_exit_failure_after_valid_stream_is_incomplete(self):
        r=self.invoke(self.client('sys.exit(7)'))
        self.assertEqual(r['status'],'incomplete');self.assertEqual(r['process']['exit_code'],7)

    def test_timeout_after_statistics_is_incomplete(self):
        r=self.invoke(self.client('time.sleep(60)'),timeout_seconds=0.2)
        self.assertEqual(r['status'],'incomplete');self.assertEqual(r['process']['status'],'timed_out')
        self.assertTrue(r['process']['process_and_pipes_joined'])

    def test_stderr_flood_is_capped_and_joined(self):
        r=self.invoke('import os;\nwhile True:os.write(2,b"x"*65536)',max_output_bytes=1000)
        self.assertEqual(r['process']['status'],'output_limited');self.assertEqual(r['process']['output_bytes'],1000)
        self.assertTrue(r['process']['process_and_pipes_joined'])

    def test_nonreading_stdin_does_not_block_deadline(self):
        r=run_pi([sys.executable,'-c','import time; time.sleep(60)'],self.workspace,self.root/'evidence',{},'x'*(1<<20),'fixture','frozen-model',timeout_seconds=0.1)
        self.assertEqual(r['process']['status'],'timed_out');self.assertTrue(r['process']['process_and_pipes_joined'])

    def test_truncated_stream_cannot_pass_exit_zero(self):
        r=self.invoke('print("{",end="")')
        self.assertEqual(r['status'],'incomplete');self.assertEqual(r['process']['exit_code'],0)


if __name__=='__main__':unittest.main()

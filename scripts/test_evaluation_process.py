import os
from pathlib import Path
import sys
import tempfile
import threading
import time
import unittest

from evaluation_process import run_command


class ProcessTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.workspace = self.root/'workspace'; self.workspace.mkdir()
        self.evidence = self.root/'evidence'

    def run_python(self, code, **overrides):
        options = dict(argv=[sys.executable, '-c', code], cwd=self.workspace, evidence=self.evidence,
                       timeout_seconds=5, max_output_bytes=4096, environment={})
        options.update(overrides)
        return run_command(**options)

    def test_exact_arguments_separate_logs_and_explicit_environment(self):
        report = self.run_python('import os,sys; print(repr(sys.argv[1])); print(os.getenv("EXPLICIT")); print(os.getenv("PATH")); print("error",file=sys.stderr)',
            argv=[sys.executable, '-c', 'import os,sys; print(repr(sys.argv[1])); print(os.getenv("EXPLICIT")); print(os.getenv("PATH")); print("error",file=sys.stderr)', 'a; touch forbidden'],
            environment={'EXPLICIT': 'allowed'})
        self.assertEqual(report['status'], 'completed')
        self.assertTrue(report['process_and_pipes_joined'])
        self.assertEqual((self.evidence/'stdout.log').read_text(), "'a; touch forbidden'\nallowed\nNone\n")
        self.assertEqual((self.evidence/'stderr.log').read_text(), 'error\n')
        self.assertFalse((self.workspace/'forbidden').exists())

    def test_failure_preserves_exit_code_and_output(self):
        report = self.run_python('import sys; print("failure evidence"); sys.exit(7)')
        self.assertEqual((report['status'], report['exit_code']), ('failed', 7))
        self.assertEqual((self.evidence/'stdout.log').read_text(), 'failure evidence\n')

    def test_timeout_joins_process_and_pipes(self):
        report = self.run_python('import time; time.sleep(60)', timeout_seconds=0.1)
        self.assertEqual(report['status'], 'timed_out')
        self.assertTrue(report['process_and_pipes_joined'])
        self.assertLess(report['elapsed_seconds'], 3)

    def test_output_limit_cannot_report_success_or_grow_logs(self):
        report = self.run_python('import os;\nwhile True: os.write(1,b"x"*65536)', max_output_bytes=1000)
        self.assertEqual(report['status'], 'output_limited')
        self.assertEqual(report['output_bytes'], 1000)
        self.assertEqual(sum(p.stat().st_size for p in self.evidence.iterdir()), 1000)
        self.assertTrue(report['process_and_pipes_joined'])

    def test_cancellation_joins_running_command(self):
        cancelled = threading.Event()
        timer = threading.Timer(0.1, cancelled.set); timer.start()
        self.addCleanup(timer.join)
        report = self.run_python('import time; time.sleep(60)', cancelled=cancelled)
        self.assertEqual(report['status'], 'cancelled')
        self.assertTrue(report['process_and_pipes_joined'])

    def test_pre_cancel_does_not_launch_or_create_evidence(self):
        cancelled = threading.Event(); cancelled.set()
        with self.assertRaises(InterruptedError): self.run_python('open("bad","w").close()', cancelled=cancelled)
        self.assertFalse((self.workspace/'bad').exists())
        self.assertFalse(self.evidence.exists())

    def test_parent_exit_cleans_descendant_with_detached_output(self):
        child = 'import time; from pathlib import Path; Path("ready").touch(); time.sleep(0.5); Path("late").touch()'
        code = ('import subprocess,sys,time; from pathlib import Path; '
                'subprocess.Popen([sys.executable,"-c",'+repr(child)+'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); '
                '\nwhile not Path("ready").exists(): time.sleep(0.005)')
        report = self.run_python(code)
        self.assertEqual(report['status'], 'completed')
        self.assertTrue((self.workspace/'ready').exists())
        time.sleep(0.6)
        self.assertFalse((self.workspace/'late').exists())

    def test_existing_evidence_never_overwritten(self):
        self.evidence.mkdir(); (self.evidence/'stdout.log').write_text('preserve')
        with self.assertRaises(FileExistsError): self.run_python('print("replace")')
        self.assertEqual((self.evidence/'stdout.log').read_text(), 'preserve')

    def test_bad_limits_fail_before_launch(self):
        for timeout in [True, 0, -1, float('nan'), float('inf'), 10**1000]:
            with self.subTest(timeout=str(timeout)[:20]), self.assertRaises(ValueError):
                self.run_python('print("bad")', timeout_seconds=timeout)
        self.assertFalse(self.evidence.exists())


if __name__ == '__main__': unittest.main()

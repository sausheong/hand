import os
from pathlib import Path
import sys
import tempfile
import unittest
from benchmark_startup import percentile, trial


class StartupProbeTests(unittest.TestCase):
    def test_nearest_rank_and_missing_observation(self):
        self.assertEqual(percentile(list(range(1, 31)), .95), 29)
        self.assertIsNone(percentile([], .95))

    def test_hung_application_times_out_and_is_reaped(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            script = root/'hung'
            script.write_text('#!'+sys.executable+'\nimport time\ntime.sleep(30)\n')
            script.chmod(0o700)
            result = trial(script, script, 'none', root, 0, .2)
            self.assertFalse(result['observation_valid'])
            self.assertIn('deadline', result['failure'])
            self.assertEqual(result['exit_code'], -9)
            self.assertLess(result['elapsed_ms'], 2000)

    def test_terminal_echo_cannot_impersonate_application_readiness(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            script = root/'echo'
            script.write_text('#!'+sys.executable+'\nimport time\nprint("Type a message...",flush=True)\ntime.sleep(30)\n')
            script.chmod(0o700)
            result = trial(script, script, 'none', root, 0, 2)
            self.assertFalse(result['observation_valid'])
            self.assertIn('echo remains enabled', result['failure'])
            self.assertIsNone(result['startup_to_input_ms'])

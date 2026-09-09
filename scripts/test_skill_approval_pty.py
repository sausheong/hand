import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import check_skill_approval_pty as runner


class SkillPTYFailureTests(unittest.TestCase):
    def test_setup_and_build_failures_keep_failed_report(self):
        for stage in ('module', 'build'):
            with self.subTest(stage=stage), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                project, module = base/'project', base/'module'
                project.mkdir(); module.mkdir()
                (project/'go.mod').write_text('module hand\n')
                (module/'go.mod').write_text('module harness\n')
                output = base/'result'

                def command(args, **kwargs):
                    if args[1] == 'list' and stage == 'build':
                        kwargs['stdout'].write(json.dumps({'Dir': str(module)}).encode())
                        return subprocess.CompletedProcess(args, 0)
                    kwargs['stdout'].write(b'fixture build/setup failure\n')
                    raise subprocess.CalledProcessError(1, args)

                with patch.object(runner, 'ROOT', project), patch.object(runner.subprocess, 'run', command), \
                     patch('sys.argv', ['runner', '--output', str(output)]), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(runner.main(), 1)
                report = json.loads((output/'report.json').read_text())
                self.assertEqual(report['status'], 'failed')
                self.assertEqual(report['results'], [])
                self.assertIn('error', report)
                log = output/('build.log' if stage == 'build' else 'harness-module.json')
                self.assertIn('fixture', log.read_text())
                if stage == 'build':
                    self.assertTrue(report['sources_unchanged'])

    def test_released_module_inventory_and_midrun_mutation(self):
        for mutate in (False, True):
            with self.subTest(mutate=mutate), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                project, module = base/'project', base/'module'
                project.mkdir(); module.mkdir()
                (project/'go.mod').write_text('module hand\n')
                source = module/'go.mod'
                source.write_text('module harness\n')
                output = base/'result'

                def command(args, **kwargs):
                    if args[1] == 'list':
                        kwargs['stdout'].write(json.dumps({'Dir': str(module), 'Version': 'v0.0.1'}).encode())
                    elif args[1] == 'build':
                        Path(args[3]).write_bytes(b'fixture binary')
                    else:
                        self.fail('unexpected command: ' + repr(args))
                    return subprocess.CompletedProcess(args, 0)

                def trial(binary, output, decision):
                    if mutate:
                        source.write_text('module changed\n')
                    return dict(decision=decision, status='passed')

                with patch.object(runner, 'ROOT', project), patch.object(runner.subprocess, 'run', command), \
                     patch.object(runner, 'trial', trial), \
                     patch('sys.argv', ['runner', '--output', str(output)]), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(runner.main(), int(mutate))
                report = json.loads((output/'report.json').read_text())
                self.assertEqual(report['status'], 'failed' if mutate else 'development_passed')
                self.assertEqual(report['sources_unchanged'], not mutate)
                self.assertEqual(len(report['results']), 3)
                self.assertTrue(report['source_before'][str(module)]['entries'])

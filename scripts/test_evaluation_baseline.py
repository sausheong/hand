import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest

from evaluation_baseline import verify_baseline
from evaluation_checkout import git
from test_evaluation_manifest import ROOT


class BaselineTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.source = self.root/'source'; self.source.mkdir()
        git(self.source, 'init', '--template=', '--quiet')
        (self.source/'go.mod').write_text('module fixture\n\ngo 1.25\n')
        (self.source/'value.go').write_text('package fixture\nfunc Value() int { return 42 }\n')
        git(self.source, 'add', '.')
        git(self.source, '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
            'commit', '--quiet', '-m', 'fixture')
        commit=git(self.source, 'rev-parse', 'HEAD').decode().strip()
        oracle=b'package fixture\nimport "testing"\nfunc TestOracle(t *testing.T) { if Value()!=42 { t.Fatal("wrong value") } }\n'
        (self.root/'oracle.go').write_bytes(oracle)
        manifest=json.loads((ROOT/'calibration.json').read_text())
        task=manifest['tasks'][0]; manifest['tasks']=[task]
        task['repository']=dict(url='local-test-fixture',commit=commit)
        task['fixtures']=[dict(source='oracle.go',destination='oracle_test.go',sha256=hashlib.sha256(oracle).hexdigest())]
        task['verification']=[dict(argv=['go','test','-json','-count=1','./...'],expected_exit=0,timeout_seconds=60,
            result_format='go_test_json',expected_tests=[dict(package='fixture',test='TestOracle')])]
        self.manifest=self.root/'manifest.json';self.manifest.write_text(json.dumps(manifest))
        self.task=task['id'];self.output=self.root/'output'
        self.environment=dict(PATH=os.environ['PATH'],GOPROXY='off',GOWORK='off',GOTOOLCHAIN='local',
                              GOCACHE=str(self.root/'cache'))

    def run_baseline(self):
        return verify_baseline(self.manifest,self.task,self.source,self.output,self.environment)

    def test_real_go_baseline_produces_reviewable_evidence_and_refuses_overwrite(self):
        report=self.run_baseline()
        self.assertEqual(report['status'],'baseline_tests_passed')
        saved=json.loads((self.output/'report.json').read_text())
        self.assertEqual(saved,report)
        self.assertEqual(saved['checks'][0]['verification']['tests'][0]['test'],'TestOracle')
        self.assertFalse((self.source/'oracle_test.go').exists())
        before=(self.output/'report.json').read_bytes()
        with self.assertRaises(FileExistsError): self.run_baseline()
        self.assertEqual((self.output/'report.json').read_bytes(),before)

    def test_missing_expected_test_fails_despite_zero_exit(self):
        manifest=json.loads(self.manifest.read_text())
        manifest['tasks'][0]['verification'][0]['expected_tests'][0]['test']='TestMissing'
        self.manifest.write_text(json.dumps(manifest))
        report=self.run_baseline()
        self.assertEqual(report['checks'][0]['execution']['exit_code'],0)
        self.assertEqual(report['status'],'baseline_tests_failed')

    def candidate(self, content):
        (self.source/'value.go').write_text(content)
        patch=self.root/'proposal.patch'
        patch.write_bytes(git(self.source, 'diff', '--binary', 'HEAD'))
        return patch

    def test_candidate_is_applied_and_exact_diff_preserved(self):
        patch=self.candidate('package fixture\nfunc Value() int { return 43 }\n')
        report=verify_baseline(self.manifest,self.task,self.source,self.output,self.environment,patch)
        self.assertEqual(report['status'],'candidate_tests_failed')
        self.assertTrue(report['candidate']['applied'])
        self.assertEqual(Path(report['candidate']['patch']).read_bytes(),patch.read_bytes())
        self.assertEqual(report['candidate']['sha256'],hashlib.sha256(patch.read_bytes()).hexdigest())
        self.assertEqual(report['checks'][0]['verification']['tests'][0]['outcome'],'fail')
        self.assertFalse((self.source/'oracle_test.go').exists())
        # A separate fresh checkout accepts a correct implementation refactor.
        patch=self.candidate('package fixture\nfunc Value() int { x := 40; return x + 2 }\n')
        report=verify_baseline(self.manifest,self.task,self.source,self.root/'good',self.environment,patch)
        self.assertEqual(report['status'],'candidate_tests_passed')

    def test_bad_patch_is_retained_without_running_tests(self):
        patch=self.root/'proposal.patch';patch.write_bytes(b'invalid patch\n')
        with self.assertRaises(Exception):
            verify_baseline(self.manifest,self.task,self.source,self.output,self.environment,patch)
        report=json.loads((self.output/'report.json').read_text())
        self.assertEqual(report['status'],'candidate_error')
        self.assertFalse(report['candidate']['applied'])
        self.assertEqual(report['checks'],[])
        self.assertEqual((self.output/'candidate.patch').read_bytes(),patch.read_bytes())

    def test_candidate_cannot_supply_its_own_acceptance_fixture(self):
        (self.source/'oracle_test.go').write_text('package fixture\n')
        git(self.source,'add','oracle_test.go')
        patch=self.root/'proposal.patch';patch.write_bytes(git(self.source,'diff','--cached','--binary'))
        with self.assertRaises(FileExistsError):
            verify_baseline(self.manifest,self.task,self.source,self.output,self.environment,patch)
        report=json.loads((self.output/'report.json').read_text())
        self.assertEqual(report['status'],'candidate_error')
        self.assertEqual(report['checks'],[])

    def test_passing_candidate_that_mutates_oracle_is_rejected(self):
        patch=self.candidate('package fixture\nimport "os"\nfunc Value() int { _ = os.WriteFile("oracle_test.go", []byte("package fixture\\n"), 0600); return 42 }\n')
        report=verify_baseline(self.manifest,self.task,self.source,self.output,self.environment,patch)
        self.assertEqual(report['checks'][0]['execution']['exit_code'],0)
        self.assertEqual(report['checks'][0]['verification']['status'],'passed')
        self.assertEqual(report['status'],'candidate_tests_failed')
        self.assertEqual(report['checks'][0]['fixture_integrity']['status'],'failed')

    def test_preparation_error_is_preserved(self):
        with self.assertRaises(FileNotFoundError):
            verify_baseline(self.manifest,self.task,self.root/'missing',self.output,self.environment)
        report=json.loads((self.output/'report.json').read_text())
        self.assertEqual(report['status'],'baseline_error')
        self.assertEqual(report['checks'],[])


if __name__ == '__main__': unittest.main()

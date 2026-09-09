import json
import unittest

from evaluation_go_tests import verify_go_tests


class GoTestEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.execution = dict(status='completed', exit_code=0, process_and_pipes_joined=True)
        self.expected = [dict(package='fixture', test='TestOracle')]
        self.events = [dict(Action='start', Package='fixture'),
                       dict(Action='run', Package='fixture', Test='TestOracle'),
                       dict(Action='pass', Package='fixture', Test='TestOracle'),
                       dict(Action='pass', Package='fixture')]

    def verify(self, events=None, execution=None):
        return verify_go_tests('\n'.join(json.dumps(e) for e in (self.events if events is None else events)),
                               self.execution if execution is None else execution, self.expected)

    def test_complete_expected_inventory_passes(self):
        self.assertEqual(self.verify()['status'], 'passed')

    def test_zero_exit_with_empty_selection_fails(self):
        result = self.verify([self.events[0], self.events[-1]])
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(result['missing'], self.expected)

    def test_skip_or_failure_cannot_be_hidden_by_package_pass(self):
        for outcome in ['skip', 'fail']:
            events = [dict(e) for e in self.events]
            events[2]['Action'] = outcome
            self.assertEqual(self.verify(events)['status'], 'failed')

    def test_truncated_duplicate_or_unstarted_results_fail(self):
        for events in [self.events[:-1], self.events[:2]+self.events[-1:], self.events+[self.events[-1]],
                       self.events[:3]+[self.events[2],self.events[-1]], self.events[:1]+self.events[2:]]:
            self.assertEqual(self.verify(events)['status'], 'failed')

    def test_valid_pause_continue_and_subtests(self):
        events = self.events[:2]+[
            dict(Action='pause', Package='fixture', Test='TestOracle'),
            dict(Action='cont', Package='fixture', Test='TestOracle'),
            dict(Action='run', Package='fixture', Test='TestOracle/case'),
            dict(Action='pass', Package='fixture', Test='TestOracle/case')]+self.events[2:]
        self.assertEqual(self.verify(events)['status'], 'passed')

    def test_invalid_json_and_event_shapes_fail(self):
        for extra in ['not json', 'null', '[]', '{}', '{"Action":"output","Package":"fixture"}',
                      '{"Action":"fail","Action":"output","Package":"fixture","Output":"hidden"}']:
            output='\n'.join(json.dumps(e) for e in self.events)+'\n'+extra
            self.assertEqual(verify_go_tests(output,self.execution,self.expected)['status'],'failed')

    def test_command_failure_timeout_output_limit_and_unjoined_fail(self):
        for changes in [dict(status='timed_out'),dict(status='output_limited'),dict(status='failed'),
                        dict(exit_code=1),dict(exit_code=False),dict(process_and_pipes_joined=False)]:
            self.assertEqual(self.verify(execution=dict(self.execution,**changes))['status'],'failed')

    def test_expected_inventory_cannot_be_empty_or_duplicate(self):
        for expected in [[],self.expected*2,[dict(package='fixture',test='')]]:
            with self.assertRaises(ValueError): verify_go_tests('',self.execution,expected)


if __name__ == '__main__': unittest.main()

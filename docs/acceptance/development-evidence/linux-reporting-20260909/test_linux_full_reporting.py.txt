import json
import unittest
from check_linux_full import summarise_go_log


def log(*records):
    return '\n'.join(json.dumps(record) for record in records)


class LinuxFullReportingTests(unittest.TestCase):
    def test_failed_suite_preserves_pass_fail_skip_and_package_outcome(self):
        records = [{'Package': 'p', 'Action': 'start'}]
        for name, action in [('TestPass', 'pass'), ('TestFail', 'fail'), ('TestSkip', 'skip')]:
            records += [{'Package': 'p', 'Test': name, 'Action': 'run'},
                        {'Package': 'p', 'Test': name, 'Action': action}]
        records += [{'Package': 'p', 'Action': 'fail'}]
        result = summarise_go_log(log(*records))
        self.assertEqual(result['test_records'], {'pass': 1, 'fail': 1, 'skip': 1})
        self.assertEqual(result['package_records'], {'fail': 1})
        self.assertEqual(result['unfinished_tests'], [])
        self.assertEqual(result['unfinished_packages'], [])

    def test_crash_does_not_turn_started_work_into_completion(self):
        result = summarise_go_log(log({'Package': 'p', 'Action': 'start'},
                                      {'Package': 'p', 'Test': 'TestCrash', 'Action': 'run'}))
        self.assertEqual(result['test_records'], {})
        self.assertEqual(result['unfinished_tests'], [{'package': 'p', 'test': 'TestCrash'}])
        self.assertEqual(result['unfinished_packages'], ['p'])

    def test_build_errors_and_unparsed_output_are_retained_as_missing_evidence(self):
        result = summarise_go_log('compiler error\n[]\n'+log({'Package': 'p', 'Action': 'fail'}))
        self.assertEqual(result['test_records'], {})
        self.assertEqual(result['package_records'], {'fail': 1})
        self.assertEqual(result['unparsed_lines'], 2)


if __name__ == '__main__':
    unittest.main()

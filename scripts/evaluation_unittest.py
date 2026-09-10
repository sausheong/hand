"""Run Python unittest discovery with strict, machine-readable test outcomes.

Invoke with Python -I. Test output goes to stderr; stdout is the event stream.
This is an oracle driver, not a sandbox or proof that a test is trustworthy.
"""
import argparse
import contextlib
import json
import sys
import unittest

PACKAGE = 'python.unittest'


class EventResult(unittest.TestResult):
    def __init__(self, emit):
        super().__init__()
        self.emit = emit
        self.active = set()
        self.outcomes = {}

    def event(self, action, test):
        self.emit(dict(Action=action, Package=PACKAGE, Test=test.id()))

    def startTest(self, test):
        super().startTest(test)
        self.active.add(test.id())
        self.event('run', test)

    def outcome(self, test, action):
        # Class/module setup errors use unittest's ErrorHolder, for which
        # startTest/stopTest are not invoked. Preserve that failure too.
        if test.id() not in self.active:
            self.event('run', test)
            self.event(action, test)
        else:
            previous = self.outcomes.get(test.id())
            self.outcomes[test.id()] = 'fail' if 'fail' in (previous, action) else action

    def addSuccess(self, test):
        super().addSuccess(test)
        self.outcome(test, 'pass')

    def addError(self, test, err):
        super().addError(test, err)
        self.outcome(test, 'fail')

    def addFailure(self, test, err):
        super().addFailure(test, err)
        self.outcome(test, 'fail')

    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.outcome(test, 'skip')

    def addExpectedFailure(self, test, err):
        super().addExpectedFailure(test, err)
        self.outcome(test, 'fail')

    def addUnexpectedSuccess(self, test):
        super().addUnexpectedSuccess(test)
        self.outcome(test, 'fail')

    def addSubTest(self, test, subtest, err):
        super().addSubTest(test, subtest, err)
        if err is not None:
            self.outcome(test, 'fail')

    def stopTest(self, test):
        self.event(self.outcomes.pop(test.id(), 'fail'), test)
        self.active.remove(test.id())
        super().stopTest(test)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--start-directory', required=True)
    parser.add_argument('--pattern', default='test*.py')
    args = parser.parse_args()
    output = sys.stdout

    def emit(event):
        output.write(json.dumps(event, allow_nan=False) + '\n')
        output.flush()

    emit(dict(Action='start', Package=PACKAGE))
    result = EventResult(emit)
    with contextlib.redirect_stdout(sys.stderr):
        suite = unittest.defaultTestLoader.discover(args.start_directory, pattern=args.pattern)
        suite.run(result)
        for test, diagnostic in result.errors + result.failures + result.expectedFailures:
            print(test.id(), diagnostic, file=sys.stderr)
    passed = (result.testsRun > 0 and result.wasSuccessful() and not result.skipped
              and not result.expectedFailures and not result.active)
    emit(dict(Action='pass' if passed else 'fail', Package=PACKAGE))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())

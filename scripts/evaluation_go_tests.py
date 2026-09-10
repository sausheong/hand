"""Validate complete Go test JSON against an explicit test inventory."""
import json


def unique_fields(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('duplicate event field')
        result[key] = value
    return result


def verify_test_events(output, execution, expected):
    if not isinstance(expected, list) or not expected:
        raise ValueError('an explicit expected test inventory is required')
    wanted = set()
    for item in expected:
        if (not isinstance(item, dict) or set(item) != {'package', 'test'}
                or any(not isinstance(v, str) or not v for v in item.values())):
            raise ValueError('each expected test needs package and test names')
        key = item['package'], item['test']
        if key in wanted:
            raise ValueError('duplicate expected test')
        wanted.add(key)
    errors, packages, tests = [], {}, {}
    if (execution.get('status') != 'completed' or type(execution.get('exit_code')) is not int or execution['exit_code'] != 0
            or execution.get('process_and_pipes_joined') is not True):
        errors.append('verification command did not complete successfully with joined output')
    for number, line in enumerate(output.splitlines(), 1):
        try:
            event = json.loads(line, object_pairs_hook=unique_fields)
        except (ValueError, UnicodeDecodeError):
            errors.append(f'line {number}: invalid JSON event')
            continue
        if not isinstance(event, dict):
            errors.append(f'line {number}: event must be an object')
            continue
        action, package, test = event.get('Action'), event.get('Package'), event.get('Test')
        if not isinstance(package, str) or not package or (test is not None and (not isinstance(test, str) or not test)):
            errors.append(f'line {number}: missing package or invalid test identity')
            continue
        if action in ('output', 'build-output'):
            if not isinstance(event.get('Output'), str):
                errors.append(f'line {number}: output must be a string')
            continue
        if action in ('build-start', 'build-fail'):
            if action == 'build-fail': errors.append('package build failed: '+package)
            continue
        if test is None:
            if action == 'start' and package not in packages:
                packages[package] = 'running'
            elif action in ('pass', 'fail', 'skip') and packages.get(package) == 'running':
                packages[package] = action
                if action != 'pass': errors.append('package '+action+': '+package)
            else:
                errors.append(f'line {number}: invalid package transition')
            continue
        key = package, test
        if packages.get(package) != 'running':
            errors.append(f'line {number}: test event outside running package')
        state = tests.get(key)
        if action == 'run' and state is None:
            tests[key] = 'running'
        elif action == 'pause' and state == 'running':
            tests[key] = 'paused'
        elif action == 'cont' and state == 'paused':
            tests[key] = 'running'
        elif action in ('pass', 'fail', 'skip') and state == 'running':
            tests[key] = action
            if action != 'pass': errors.append('test '+action+': '+package+'::'+test)
        else:
            errors.append(f'line {number}: invalid test transition')
    missing = sorted(wanted-set(tests))
    for package, test in missing: errors.append('expected test did not run: '+package+'::'+test)
    for package, state in packages.items():
        if state == 'running': errors.append('package has no terminal result: '+package)
    for (package, test), state in tests.items():
        if state in ('running', 'paused'): errors.append('test has no terminal result: '+package+'::'+test)
    if not packages: errors.append('no package events')
    return dict(status='passed' if not errors else 'failed', errors=errors,
                expected=expected, missing=[dict(package=p, test=t) for p, t in missing],
                packages=packages, tests=[dict(package=p, test=t, outcome=state)
                                          for (p, t), state in sorted(tests.items())],
                limitation='Event validation does not authenticate a test binary, prevent forged output or replace independent code review.')


# Compatibility for existing Go-only callers. The Python unittest driver uses
# the same event transitions, with an explicit python.unittest package identity.
verify_go_tests = verify_test_events

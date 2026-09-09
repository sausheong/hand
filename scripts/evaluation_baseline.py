"""Prepare and verify one pinned baseline task; no agent execution or scoring."""
import argparse
import hashlib
import json
from pathlib import Path

from evaluation_checkout import prepare_checkout, git
from evaluation_go_tests import verify_test_events
from evaluation_manifest import validate
from evaluation_process import run_command
from evaluation_workspace import install_fixtures, verify_fixtures


def verify_baseline(manifest_path, task_id, repository, output, environment, candidate_patch=None):
    manifest_path, output = Path(manifest_path), Path(output)
    raw = manifest_path.read_bytes()
    manifest = json.loads(raw)
    validation = validate(manifest, manifest_path.resolve().parent)
    tasks = [task for task in manifest['tasks'] if task['id'] == task_id]
    if len(tasks) != 1:
        raise ValueError('task must identify exactly one manifest entry')
    if not output.is_absolute() or output != output.resolve():
        raise ValueError('output must be absolute without symlink components')
    task = tasks[0]
    patch = None
    if candidate_patch is not None:
        with Path(candidate_patch).open('rb') as stream:
            patch = stream.read((64 << 20) + 1)
        if not patch or len(patch) > 64 << 20:
            raise ValueError('candidate patch must contain 1 through 64 MiB of bytes')
    output.mkdir(mode=0o700)
    report = dict(schema_version=1, status='preparing', task=task_id,
                  manifest_sha256=hashlib.sha256(raw).hexdigest(),
                  catalogue_status=validation['status'], repository=task['repository'],
                  input_kind='candidate_patch' if patch is not None else 'baseline',
                  environment_keys=sorted(environment), checks=[],
                  limitations=['Offline verification only; no model calls or task-success scoring. Candidate code requires independent review.',
                      'Local command execution is not an isolation sandbox; use only reviewed offline baseline commands.',
                      'Independent review and all comparative acceptance gates remain required.'])
    workspace = output/'workspace'
    try:
        report['preparation'] = prepare_checkout(repository, task['repository']['commit'], workspace)
        if patch is not None:
            saved_patch = output/'candidate.patch'
            saved_patch.write_bytes(patch)
            report['candidate'] = dict(patch=str(saved_patch), bytes=len(patch),
                                       sha256=hashlib.sha256(patch).hexdigest(), applied=False)
            git(workspace, 'apply', '--check', '--index', '--', str(saved_patch))
            git(workspace, 'apply', '--index', '--', str(saved_patch))
            # Match the checkout preparer's explicit unsupported-entry policy.
            # Do not run tests in a workspace containing newly introduced links.
            for entry in git(workspace, 'ls-files', '--stage', '-z').split(b'\0'):
                if entry and entry.split(b' ', 1)[0] not in (b'100644', b'100755'):
                    raise ValueError('candidate contains unsupported symlink or submodule')
            report['candidate'].update(applied=True,
                tree=git(workspace, 'write-tree').decode().strip())
        report['fixtures'] = install_fixtures(manifest_path.resolve().parent, workspace, task['fixtures'])
        cwd = workspace/task['working_directory']
        if cwd != cwd.resolve() or not cwd.is_dir():
            raise ValueError('verification working directory is unavailable or not contained')
        for index, check in enumerate(task['verification']):
            execution = run_command(check['argv'], cwd, output/('check-'+str(index)),
                                    check['timeout_seconds'], 16 << 20, environment)
            integrity = verify_fixtures(workspace, task['fixtures'])
            stdout = Path(execution['logs']['stdout']['path']).read_text()
            verification = verify_test_events(stdout, execution, check['expected_tests'])
            report['checks'].append(dict(execution=execution, verification=verification, fixture_integrity=integrity))
            if integrity['status'] != 'passed':
                # Do not let another command consume an altered oracle.
                break
        prefix = 'candidate' if patch is not None else 'baseline'
        passed = (len(report['checks']) == len(task['verification']) and
                  all(c['verification']['status'] == 'passed' and c['fixture_integrity']['status'] == 'passed'
                      for c in report['checks']))
        report['status'] = prefix + ('_tests_passed' if passed else '_tests_failed')
    except Exception as error:
        report['status'] = 'candidate_error' if patch is not None else 'baseline_error'
        report['error'] = dict(type=type(error).__name__, message=str(error))
        raise
    finally:
        with (output/'report.json').open('x') as stream:
            json.dump(report, stream, indent=2, allow_nan=False)
            stream.write('\n')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('--task', required=True)
    parser.add_argument('--repository', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--environment', type=Path, required=True,
                        help='explicit JSON environment mapping; never include model credentials')
    parser.add_argument('--candidate-patch', type=Path, help='reviewed offline patch; tests passing is not task success')
    args = parser.parse_args()
    report = verify_baseline(args.manifest, args.task, args.repository, args.output,
                             json.loads(args.environment.read_text()), args.candidate_patch)
    print(json.dumps(dict(status=report['status'], report=str(args.output/'report.json'))))
    return 0 if report['status'] in ('baseline_tests_passed', 'candidate_tests_passed') else 1


if __name__ == '__main__': raise SystemExit(main())

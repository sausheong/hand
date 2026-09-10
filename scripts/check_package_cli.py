#!/usr/bin/env python3
"""Exercise package CLI transactions without provider calls or peer execution."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tarfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--hand-binary', required=True)
    parser.add_argument('--example-dir', required=True)
    parser.add_argument('--out', required=True)
    args = parser.parse_args()
    binary = Path(args.hand_binary).resolve()
    root = Path(args.out).resolve()
    root.mkdir(parents=True, exist_ok=False)
    home = root / 'home'
    home.mkdir(mode=0o700)
    source = root / 'source'
    shutil.copytree(Path(args.example_dir).resolve(), source)
    store = root / 'store'
    env = {'HOME': str(home), 'PATH': str(Path(sys.executable).parent) + ':/usr/bin:/bin', 'LANG': 'C', 'LC_ALL': 'C'}
    records = []

    def run(*words, success=True):
        result = subprocess.run([str(binary), 'packages', *map(str, words)], env=env,
                                cwd=root, capture_output=True, text=True, timeout=30)
        records.append({'arguments': list(map(str, words)), 'exit_code': result.returncode,
                        'stdout': result.stdout, 'stderr': result.stderr})
        (root / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
        assert (result.returncode == 0) == success, records[-1]
        return json.loads(result.stdout) if result.stdout.strip() else None

    def review(action, filename, *words):
        result = run(action, '--store', store, '--out', root / filename, *words)
        assert stat.S_IMODE(Path(result['review_file']).stat().st_mode) == 0o600
        return result

    def apply(selected, approval=None, success=True):
        return run('apply', '--review', selected['review_file'], '--approve',
                   approval or selected['approval_digest'], success=success)

    inspected = run('inspect', '--source', source)
    pin = inspected['package_digest']
    name = inspected['manifest']['name']
    initial = review('review-install', 'install.json', '--source', source, '--pin', pin)
    apply(initial, '0' * 64, success=False)
    assert run('list', '--store', store)['lock']['generation'] == 0
    assert apply(initial)['result']['committed']
    apply(initial, success=False)
    state = run('list', '--store', store)['lock']
    assert state['generation'] == 1 and state['packages'][0]['current'] == pin
    assert state['packages'][0]['revisions'][0]['origin']['location'] == str(source)
    manifest_file = source / 'hand-package.json'
    manifest = json.loads(manifest_file.read_text())
    manifest['version'] = '1.1.0'
    manifest['extensions'][0]['capabilities'].append('file.read')
    manifest_file.write_text(json.dumps(manifest, indent=2) + '\n')
    next_pin = run('inspect', '--source', source)['package_digest']
    assert next_pin != pin
    update = review('review-install', 'update.json', '--source', source, '--pin', next_pin)
    apply(update, initial['approval_digest'], success=False)
    assert apply(update)['result']['committed']
    rollback = review('review-rollback', 'rollback.json', '--name', name, '--pin', pin)
    assert apply(rollback)['result']['committed']
    state = run('list', '--store', store)['lock']
    assert state['packages'][0]['current'] == pin
    assert len(state['packages'][0]['revisions']) == 2
    removal = review('review-remove', 'remove.json', '--name', name)
    assert apply(removal)['result']['committed']
    state = run('list', '--store', store)['lock']
    assert state['generation'] == 4 and state['packages'] == []
    assert list((store / 'staging').iterdir()) == []
    archive = root / 'package.tar'
    with tarfile.open(archive, 'w') as bundle:
        for filename in ['hand-package.json', 'main.py']:
            bundle.add(source / filename, arcname=filename, recursive=False)
    archive_pin = hashlib.sha256(archive.read_bytes()).hexdigest()
    git_env = dict(env, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                   GIT_AUTHOR_NAME='Hand fixture', GIT_AUTHOR_EMAIL='hand@example.invalid',
                   GIT_COMMITTER_NAME='Hand fixture', GIT_COMMITTER_EMAIL='hand@example.invalid')
    git_records = []

    def git(*words):
        result = subprocess.run(['git', '-C', str(source), *words], env=git_env,
                                capture_output=True, text=True, timeout=30)
        git_records.append({'arguments': list(words), 'exit_code': result.returncode,
                            'stdout': result.stdout, 'stderr': result.stderr})
        (root / 'git-fixture.json').write_text(json.dumps(git_records, indent=2) + '\n')
        assert result.returncode == 0, git_records[-1]
        return result.stdout.strip()

    git('init', '-q')
    git('add', '--', 'hand-package.json', 'main.py')
    git('commit', '-qm', 'package CLI fixture')
    commit = git('rev-parse', 'HEAD')
    imported = []
    for kind, location, ref_flag, reference in [
            ('archive', archive, '--archive-sha256', archive_pin),
            ('git', source, '--commit', commit)]:
        target_store = root / (kind + '-store')
        selected = run('review-' + kind, '--source', location, '--pin', next_pin,
                       ref_flag, reference, '--store', target_store,
                       '--out', root / (kind + '-review.json'))
        assert selected['review']['source'] == str(location)
        assert apply(selected)['result']['committed']
        installed = run('list', '--store', target_store)['lock']
        revision = installed['packages'][0]['revisions'][0]
        assert revision['origin'] == {'kind': kind, 'location': str(location),
                                      'reference': reference}
        assert revision['digest'] == next_pin
        assert list((target_store / 'staging').iterdir()) == []
        imported.append({'kind': kind, 'origin': revision['origin'], 'digest': next_pin})
    # Runtime approvals below are explicitly owned by this test fixture.
    shown = run('show', '--store', root / 'git-store', '--name', name)
    assert shown['packages'][0]['digest'] == next_pin
    runtimes = run('review-runtimes', '--names', name, '--store', root / 'git-store',
                   '--out', root / 'runtimes.json')
    runtime_path = Path(runtimes['review_file'])
    runtime_document = json.loads(runtime_path.read_text())
    assert len(runtime_document['reviews']) == 1
    assert runtime_document['reviews'][0]['approved_digest'] == ''
    workspace = root / 'workspace'
    workspace.mkdir()
    snapshots = root / 'launch-snapshots'
    startup_path = root / 'extensions.json'
    launch_args = ('review-extensions', '--store', root / 'git-store', '--names', name,
                   '--workspace', workspace, '--snapshots', snapshots,
                   '--runtime-approvals', runtime_path, '--out', startup_path)
    run(*launch_args, success=False)
    assert not startup_path.exists() and not snapshots.exists()
    for entry in runtime_document['reviews']:
        key = entry['package'] + '/' + entry['review']['requirement']['name']
        entry['approved_digest'] = runtimes['review_digests'][key]
    runtime_path.write_text(json.dumps(runtime_document, indent=2) + '\n')
    launched_review = run(*launch_args)
    startup_bytes = startup_path.read_bytes()
    assert hashlib.sha256(startup_bytes).hexdigest() == launched_review['approval_digest']
    assert stat.S_IMODE(startup_path.stat().st_mode) == 0o600
    startup = json.loads(startup_bytes)
    assert startup['identities']['task-note'] == 'package:' + name + ':task-note'
    assert startup['reviews'][0]['arguments'][:2] == ['-I', '-B']
    assert not snapshots.exists()
    run(*launch_args, success=False)
    assert startup_path.read_bytes() == startup_bytes
    git('bundle', 'create', str(root / 'source.bundle'), 'HEAD')
    report = {'status': 'passed', 'provider_calls': 0, 'extension_executions': 0,
              'commands': len(records), 'runtime_probe_approved_by': 'fixture runner',
              'startup_review_sha256': launched_review['approval_digest'],
              'runtime_reviews': runtime_document, 'imports': imported, 'binary': str(binary),
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'initial_package_digest': pin, 'updated_package_digest': next_pin,
              'final_generation': state['generation'],
              'limitations': ['Development binary and local Harness replacement; not final acceptance.',
                              'Local directory, archive and local Git commands exercised; runtime and startup reviews also exercised; remote Git and built-client activation remain pending.']}
    (root / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()

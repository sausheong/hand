#!/usr/bin/env python3
"""Install a real Python package and exercise approved startup through built RPC."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
from check_rpc_client import Client
from check_extension_rpc import poll, question


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--example-dir', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--execution-config', type=Path, help='Explicit container fixture configuration')
    parser.add_argument('--image-interpreter', default='/usr/local/bin/python3')
    args = parser.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    root = args.out.resolve(); root.mkdir(parents=True, exist_ok=False)
    home = root / 'home'; home.mkdir(mode=0o700)
    source = root / 'source'; shutil.copytree(args.example_dir.resolve(strict=True), source)
    workspace = root / 'workspace'; workspace.mkdir()
    store, snapshots = root / 'store', root / 'snapshots'
    env = {'HOME': str(home), 'PATH': str(Path(sys.executable).parent) + ':/usr/bin:/bin'}
    commands, clients = [], []
    container_flags = []
    execution = None
    container_commands = []
    before_containers = set()
    def containers():
        config = root / 'docker-config'; config.mkdir(mode=0o700, exist_ok=True)
        argv = [execution['docker'], '--config', str(config), '--host', 'unix://' + execution['socket'], 'ps', '-aq', '--filter', 'name=harness-exec-']
        result = subprocess.run(argv, env=env, capture_output=True, text=True, timeout=10)
        container_commands.append(dict(argv=argv, exit_code=result.returncode, stdout=result.stdout, stderr=result.stderr))
        (root / 'container-commands.json').write_text(json.dumps(container_commands, indent=2) + '\n')
        assert result.returncode == 0, container_commands[-1]
        return set(result.stdout.split())
    if args.execution_config:
        execution = json.loads(args.execution_config.read_text())
        assert execution['backend'] == 'container'
        configdir = home / '.hand'; configdir.mkdir(mode=0o700)
        (configdir / 'config.json').write_text(json.dumps(dict(execution=execution)))
        mapping = root / 'container.json'
        mapping.write_text(json.dumps(dict(boundary={k: execution[k] for k in ['docker', 'socket', 'image', 'network', 'writable']}, interpreters={'python': args.image_interpreter})))
        container_flags = ['--container', mapping]
        before_containers = containers()
    report = dict(status='failed', qualification='development', binary_sha256=sha(binary),
                  runner_sha256=sha(Path(__file__)), provider_calls=0,
                  limitations=['Installed Python package journey; not final candidate or platform qualification.'])

    def command(*words, accepted=True):
        result = subprocess.run([str(binary), 'packages', *map(str, words)], env=env,
                                cwd=root, capture_output=True, text=True, timeout=30)
        commands.append(dict(arguments=list(map(str, words)), exit_code=result.returncode,
                             stdout=result.stdout, stderr=result.stderr))
        (root / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        if not accepted:
            assert result.returncode != 0, 'unapproved review unexpectedly accepted'
            return None
        assert result.returncode == 0, commands[-1]
        return json.loads(result.stdout)

    try:
        inspected = command('inspect', '--source', source)
        name, pin = inspected['manifest']['name'], inspected['package_digest']
        install = command('review-install', '--source', source, '--pin', pin,
                          '--store', store, '--out', root / 'install.json')
        assert command('apply', '--review', install['review_file'], '--approve',
                       install['approval_digest'])['result']['committed']
        shutil.rmtree(source)
        runtime_file = root / 'runtime.json'
        runtime = command('review-runtimes', *container_flags, '--store', store, '--names', name, '--out', runtime_file)
        document = json.loads(runtime_file.read_text())
        command('review-extensions', *container_flags, '--store', store, '--names', name,
                '--workspace', workspace, '--snapshots', snapshots, '--runtime-approvals', runtime_file,
                '--out', root / 'startup.json', accepted=False)
        assert not (root / 'startup.json').exists(), 'unapproved probe created startup review' 
        for entry in document['reviews']:
            assert entry['approved_digest'] == ''
            # Explicit fixture approval, never a product default.
            entry['approved_digest'] = runtime['review_digests'][entry['package'] + '/' + entry['review']['requirement']['name']]
        runtime_file.write_text(json.dumps(document, indent=2) + '\n')
        launch = command('review-extensions', *container_flags, '--store', store, '--names', name,
                         '--workspace', workspace, '--snapshots', snapshots,
                         '--runtime-approvals', runtime_file, '--out', root / 'startup.json')
        assert sha(Path(launch['review_file'])) == launch['approval_digest']
        flags = ['--extension-config', launch['review_file'], '--approve-extension-config', launch['approval_digest']]
        completed = None
        for index in range(2):
            client = Client(binary, workspace, 'http://127.0.0.1:1/v1', root, 'rpc-' + str(index), checkpoint_flags=flags)
            clients.append(client)
            assert 'extension.command' in client.call('hello', 'hello')['methods']
            params = dict(name='task-note', command='note', arguments='')
            client.call('saved-note', 'extension.command', params)
            if index == 0:
                pending = question(client)
                client.call('answer', 'extension.answer', dict(token=pending['token'], answer=dict(id=pending['question']['id'], text='Installed package RPC note')))
                completed = poll(client, 'saved-note')
                assert not completed.get('error'), completed
                assert completed['result']['blocks'][0]['text'] == 'Task note saved.'
            else:
                assert poll(client, 'saved-note') == completed
                assert client.call('no-replayed-question', 'extension.questions') == []
                client.call('cancel-note', 'extension.command', params)
                question(client); client.call('cancel', 'cancel')
                assert poll(client, 'cancel-note').get('error')
                assert client.call('no-cancelled-question', 'extension.questions') == []
            client.close(); clients.remove(client)
            assert not snapshots.exists() or not list(snapshots.iterdir()), 'snapshot survived RPC disconnect'
            if execution:
                assert not (containers() - before_containers), 'container survived RPC disconnect'
        if execution:
            report.update(container_execution=execution, container_cleanup=True, image_interpreter=args.image_interpreter)
        report.update(status='passed', unapproved_runtime_rejected=True, package_digest=pin, startup_digest=launch['approval_digest'],
                      source_removed_before_review=True, real_question_answer=True,
                      restart_replay=True, cancellation_joined=True, snapshots_removed=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        for client in clients:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (root / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

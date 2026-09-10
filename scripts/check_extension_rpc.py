#!/usr/bin/env python3
"""Exercise reviewed extension startup with built Hand and real Go/Python peers."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
from check_rpc_client import Client


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def poll(client, request_id):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        record = client.call('get-' + request_id, 'request.get', {'id': request_id})
        if record['state'] == 'completed':
            return record['result']
        time.sleep(.01)
    raise RuntimeError('extension completion timed out')


def question(client):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        pending = client.call('questions', 'extension.questions')
        if pending:
            assert len(pending) == 1
            return pending[0]
        time.sleep(.01)
    raise RuntimeError('extension question timed out')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', required=True, type=Path)
    parser.add_argument('--go-extension', required=True, type=Path)
    parser.add_argument('--python-binary', required=True, type=Path)
    parser.add_argument('--python-source', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    binary, go, python, source = [p.resolve(strict=True) for p in (args.hand_binary, args.go_extension, args.python_binary, args.python_source)]
    out = args.out.absolute(); out.mkdir(parents=True, exist_ok=False)
    report = dict(status='failed', qualification='development', binary_sha256=sha(binary), go_sha256=sha(go), python_sha256=sha(python), source_sha256=sha(source), runner_sha256=sha(Path(__file__)), client_runner_sha256=sha(Path(__file__).with_name('check_rpc_client.py')), languages={})
    clients = []
    try:
        for language in ['go', 'python']:
            evidence = out / language; evidence.mkdir()
            workspace = evidence / 'workspace'; workspace.mkdir()
            snapshots = evidence / 'snapshots'
            launch = dict(name='task-note', executable=str(go if language == 'go' else python), workspace=str(workspace), capabilities=['commands', 'questions', 'state', 'context.transform'])
            if language == 'python':
                launch.update(package_dir=str(source.parent), files=[source.name], arguments=['-I', '-B', '${package}/' + source.name])
            input_file, review = evidence / 'input.json', evidence / 'review.json'
            input_file.write_text(json.dumps(dict(version=1, snapshot_root=str(snapshots), extensions=[dict(identity='examples/' + language, launch=launch)])))
            generated = subprocess.run([str(binary), '--review-extensions', str(input_file), '--extension-review-output', str(review)], capture_output=True, timeout=15)
            (evidence / 'review.stderr').write_bytes(generated.stderr)
            assert generated.returncode == 0, generated.stderr.decode()
            digest = generated.stdout.decode().strip(); assert digest == sha(review)
            assert not snapshots.exists(), 'review started extension'
            rejected = subprocess.run([str(binary), '--rpc', '--extension-config', str(review), '--approve-extension-config', '0' * 64], capture_output=True, timeout=15)
            (evidence / 'rejection.stderr').write_bytes(rejected.stderr)
            assert rejected.returncode == 2 and not snapshots.exists(), 'wrong approval executed'
            flags = ['--extension-config', str(review), '--approve-extension-config', digest]
            def connect(name):
                client = Client(binary, workspace, 'http://127.0.0.1:1/v1', evidence, name, checkpoint_flags=flags)
                clients.append(client)
                hello = client.call('hello', 'hello')
                assert all(m in hello['methods'] for m in ['extension.command', 'extension.questions', 'extension.answer'])
                return client
            client = connect('first')
            params = dict(name='task-note', command='note', arguments='')
            client.call('note', 'extension.command', params)
            pending = question(client)
            answer = dict(token=pending['token'], answer=dict(id=pending['question']['id'], text='Remember the CLI regression'))
            first = client.call('answer', 'extension.answer', answer)
            assert client.call('answer', 'extension.answer', answer) == first
            completed = poll(client, 'note'); assert not completed.get('error'), completed
            assert completed['result']['blocks'][0]['text'] == 'Task note saved.'
            client.close(); clients.remove(client)
            assert not list(snapshots.iterdir()), 'snapshot leaked after EOF'
            client = connect('second')
            client.call('note', 'extension.command', params)
            assert poll(client, 'note') == completed
            assert client.call('no-replay-question', 'extension.questions') == []
            def reload_review(request_id, path, approved):
                client.call(request_id, 'extension.reload', dict(path=str(path), digest=approved))
                response = poll(client, request_id)
                assert not response.get('error'), response
                return response['result']
            before = sorted(p.name for p in snapshots.iterdir())
            reused = reload_review('reload-same', review, digest)
            assert reused['completed'] and reused['reload']['reused'] == ['task-note'], reused
            assert sorted(p.name for p in snapshots.iterdir()) == before
            # Generate a real review whose declared capability contract the peer
            # cannot fulfil; its staged handshake must fail without retiring old.
            bad_input, bad_review = evidence / 'bad-input.json', evidence / 'bad-review.json'
            bad_launch = dict(launch, capabilities=['commands'])
            bad_input.write_text(json.dumps(dict(version=1, snapshot_root=str(snapshots), extensions=[dict(identity='examples/' + language, launch=bad_launch)])))
            generated_bad = subprocess.run([str(binary), '--review-extensions', str(bad_input), '--extension-review-output', str(bad_review)], capture_output=True, timeout=15)
            (evidence / 'bad-review.stderr').write_bytes(generated_bad.stderr)
            assert generated_bad.returncode == 0, generated_bad.stderr.decode()
            failed = reload_review('reload-bad', bad_review, generated_bad.stdout.decode().strip())
            assert not failed['completed'] and not failed['reload']['committed'], failed
            assert sorted(p.name for p in snapshots.iterdir()) == before, 'failed staging retired working peer or leaked snapshot'
            client.call('cancel-note', 'extension.command', params); question(client)
            client.call('cancel', 'cancel')
            assert poll(client, 'cancel-note').get('error'), 'cancelled command succeeded'
            assert client.call('no-cancelled-question', 'extension.questions') == []
            recovered = reload_review('reload-cancelled', review, digest)
            assert recovered['completed'] and recovered['reload']['started'] == ['task-note'], recovered
            client.call('recovered-note', 'extension.command', params)
            pending = question(client)
            client.call('recovered-answer', 'extension.answer', dict(token=pending['token'], answer=dict(id=pending['question']['id'], cancelled=True)))
            assert not poll(client, 'recovered-note').get('error')
            empty_input, empty_review = evidence / 'empty-input.json', evidence / 'empty-review.json'
            empty_input.write_text(json.dumps(dict(version=1, snapshot_root=str(snapshots), extensions=[])))
            generated_empty = subprocess.run([str(binary), '--review-extensions', str(empty_input), '--extension-review-output', str(empty_review)], capture_output=True, timeout=15)
            (evidence / 'empty-review.stderr').write_bytes(generated_empty.stderr)
            assert generated_empty.returncode == 0, generated_empty.stderr.decode()
            removed = reload_review('reload-empty', empty_review, generated_empty.stdout.decode().strip())
            assert removed['completed'] and removed['reload']['removed'] == ['task-note'], removed
            assert not list(snapshots.iterdir()), 'remove-all left snapshots'
            client.call('removed-command', 'extension.command', params)
            assert poll(client, 'removed-command').get('error'), 'removed peer still callable'
            client.close(); clients.remove(client)
            assert not list(snapshots.iterdir()), 'snapshot leaked after cancellation'
            report['languages'][language] = dict(status='passed', review_digest=digest, wrong_approval_rejected=True, command_question_answer=True, durable_replay_after_restart=True, cancellation_joined=True, snapshots_removed=True, reload_reuses=True, failed_reload_preserves_peer=True, cancelled_peer_recovers=True, remove_all_joined=True)
        report.update(status='passed', provider_calls=0)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        for client in clients:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

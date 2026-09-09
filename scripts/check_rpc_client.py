#!/usr/bin/env python3
"""Exercise actual Hand RPC with a Python client and local provider fixture."""
import base64
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import selectors
import subprocess
import threading
import time


class Client:
    def __init__(self, binary, home, endpoint, evidence, name, checkpoint_dir=None, checkpoint_flags=None):
        self.stderr = (evidence / (name + '.stderr')).open('wb')
        user_home = evidence / 'home'
        user_home.mkdir(mode=0o700, exist_ok=True)
        extra = ["--checkpoint-dir", str(checkpoint_dir)] if checkpoint_dir else []
        extra += checkpoint_flags or []
        self.process = subprocess.Popen([str(binary), '--rpc', '--model', 'local/fixture', '--base-url', endpoint] + extra,
                                        cwd=home, env={'HOME': str(user_home), 'PATH': '/usr/bin:/bin'},
                                        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.stderr, bufsize=0)
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.process.stdout, selectors.EVENT_READ)
        self.buffer = b''
        self.transcript = []
        self.evidence, self.name = evidence, name

    def call(self, request_id, method, params=None, version=1, expect_error=None):
        request = dict(version=version, id=request_id, method=method, params=params or {})
        self.process.stdin.write(json.dumps(request).encode() + b'\n')
        deadline = time.monotonic() + 15
        while b'\n' not in self.buffer:
            if not self.selector.select(max(0, deadline - time.monotonic())):
                raise RuntimeError('RPC response timed out')
            chunk = os.read(self.process.stdout.fileno(), 4096)
            if not chunk:
                raise RuntimeError('RPC stdout closed before response')
            self.buffer += chunk
            if len(self.buffer) > 1048577:
                raise RuntimeError('RPC response exceeds frame limit')
        line, self.buffer = self.buffer.split(b'\n', 1)
        response = json.loads(line)
        self.transcript.append(dict(request=request, response=response))
        if response.get('version') != 1:
            raise RuntimeError('response version mismatch')
        # Protocol framing errors have no accepted request ID.
        if not expect_error and response.get('request_id') != request_id:
            raise RuntimeError('response identity mismatch')
        if expect_error:
            if response.get('error', {}).get('code') != expect_error:
                raise RuntimeError('wrong protocol error: ' + json.dumps(response))
            return response
        if response.get('error'):
            raise RuntimeError(json.dumps(response['error']))
        return response['result']

    def terminal(self, request_id):
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            record = self.call('lookup', 'request.get', {'id': request_id})
            if record['state'] == 'completed':
                event = record['result']
                assert event['kind'] == 'terminal' and event['request_id'] == request_id
                assert event['run_id'] == record['run_id'] and event['session_id'] == record['session_id']
                return record
            time.sleep(.01)
        raise RuntimeError('terminal not persisted')

    def close(self):
        try:
            if not self.process.stdin.closed:
                self.process.stdin.close()
            try:
                code = self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
                raise RuntimeError('RPC disconnect failed to clean up within five seconds')
            if code != 0:
                raise RuntimeError('RPC exited ' + str(code))
        finally:
            self.selector.close()
            self.process.stdout.close()
            self.stderr.close()
            (self.evidence / (self.name + '.json')).write_text(json.dumps(self.transcript, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--checkpoint-scope', action='store_true', help='Exercise custom exclusions and bounds; requires --checkpoints')
    parser.add_argument('--restore', action='store_true', help='Verify confirmed restore and cross-process replay; requires --checkpoints')
    parser.add_argument('--checkpoints', action='store_true', help='Exercise external checkpoint capture and persisted session associations')
    parser.add_argument('--execution-config', type=Path, help='Explicit container execution configuration for this fixture')
    args = parser.parse_args()
    if (args.restore or args.checkpoint_scope) and not args.checkpoints:
        parser.error("--restore and --checkpoint-scope require --checkpoints")
    binary, out = args.hand_binary.resolve(strict=True), args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    home = out / 'workspace'
    home.mkdir(mode=0o700)
    checkpoint_dir = out / 'checkpoints' if args.checkpoints else None
    checkpoint_flags = []
    if args.checkpoint_scope:
        private = home / 'private'
        private.mkdir(mode=0o700)
        (private / 'token').write_bytes(b'private fixture bytes' * 100)
        checkpoint_flags = ['--checkpoint-exclude', 'private', '--checkpoint-max-entries', '20',
                            '--checkpoint-max-file-bytes', '64', '--checkpoint-max-total-bytes', '128',
                            '--checkpoint-max-snapshots', '4', '--checkpoint-max-store-bytes', '1048576']

    execution_config = None
    if args.execution_config:
        execution_config = json.loads(args.execution_config.read_text())
        config_dir = out / 'home' / '.hand'
        config_dir.mkdir(parents=True, mode=0o700)
        (config_dir / 'config.json').write_text(json.dumps({'execution': execution_config}))
    stop = threading.Event()
    provider_requests, cancelled = [], []
    started = threading.Event()

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            provider_requests.append(body)
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            users = ' '.join(str(m.get('content', '')) for m in body['messages'] if m['role'] == 'user')

            def send(value):
                self.wfile.write(('data: ' + json.dumps(value) + '\n\n').encode())
                self.wfile.flush()

            if 'rpc-hang' in users:
                try:
                    send({'choices': [{'index': 0, 'delta': {'content': 'started'}, 'finish_reason': None}]})
                    started.set()
                    while not stop.wait(.02):
                        self.wfile.write(b': heartbeat\n\n')
                        self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    cancelled.append(True)
                return
            if not any(m['role'] == 'tool' for m in body['messages']):
                send({'choices': [{'index': 0, 'delta': {'tool_calls': [{'index': 0, 'id': 'fixture-write', 'type': 'function', 'function': {'name': 'write_file', 'arguments': json.dumps({'path': 'approved.txt', 'content': 'approved once'})}}]}, 'finish_reason': None}]})
                send({'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'tool_calls'}]})
            else:
                send({'choices': [{'index': 0, 'delta': {'content': 'approval finished'}, 'finish_reason': None}]})
                send({'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'stop'}]})
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()

    server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    endpoint = 'http://127.0.0.1:' + str(server.server_port) + '/v1'
    report = {'status': 'failed', 'qualification': 'development', 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    if execution_config:
        report['execution_config'] = execution_config
    clients = []
    try:
        c = Client(binary, home, endpoint, out, 'client-one', checkpoint_dir, checkpoint_flags)
        clients.append(c)
        hello = c.call('hello', 'hello')
        if execution_config:
            assert 'container:' in hello['execution_boundary'], 'container boundary missing'
        c.call('version', 'hello', version=999, expect_error='unsupported_version')
        c.call('approval-run', 'prompt', {'text': 'rpc-approve'})
        deadline = time.monotonic() + 10
        approval = None
        while time.monotonic() < deadline:
            pending = c.call('pending', 'approval.pending')['approvals']
            if pending:
                approval = pending[0]
                break
            time.sleep(.01)
        assert approval is not None, 'approval was not published'
        assert not (home / 'approved.txt').exists(), 'write happened before approval'
        c.call('steer', 'steer', {'text': 'steer note from Python'})
        queued = c.call('queue', 'followup.enqueue', {'text': 'queued Python follow-up'})
        c.call('approve', 'approval.respond', {'run_id': approval['run_id'], 'approval_id': approval['payload']['approval_id'], 'decision': 'once'})
        terminal = c.terminal('approval-run')
        assert terminal['result']['payload']['status'] == 'completed'
        assert (home / 'approved.txt').read_text() == 'approved once'
        change_page = None
        if checkpoint_dir:
            assert 'checkpoint.changes' in hello['methods']
            checkpoint_session = c.call('checkpoint-session', 'state')['application']['SessionID']
            change_page = c.call('checkpoint-changes', 'checkpoint.changes')
            assert change_page['total'] == 1 and change_page['next'] == 1
            change = change_page['changes'][0]
            assert change['Path'] == 'approved.txt' and change['Before'] is None
            assert change['After']['hash'] == hashlib.sha256(b'approved once').hexdigest()
            c.call('checkpoint-invalid', 'checkpoint.changes', {'offset': -1}, expect_error='checkpoint_unavailable')
        stamp = (home / 'approved.txt').stat().st_mtime_ns
        assert c.call('approval-run', 'prompt', {'text': 'rpc-approve'}) == terminal
        assert (home / 'approved.txt').stat().st_mtime_ns == stamp
        c.call('remove', 'queue.remove', {'id': queued['ID']})
        assert any('steer note from Python' in json.dumps(r) for r in provider_requests), 'steering never reached provider'
        for index in range(2):
            c.call('new-' + str(index), 'session.new')
            started.clear()
            request_id = 'hang-' + str(index)
            c.call(request_id, 'prompt', {'text': 'rpc-hang'})
            assert started.wait(5), 'provider did not start'
            if index == 0:
                c.call('cancel', 'cancel')
                assert c.terminal(request_id)['result']['payload']['status'] == 'cancelled'
            else:
                c.close()
                clients.remove(c)
                c = Client(binary, home, endpoint, out, 'client-two', checkpoint_dir, checkpoint_flags)
                clients.append(c)
                c.call('hello', 'hello')
                assert c.terminal(request_id)['result']['payload']['status'] == 'cancelled'
        assert c.terminal('approval-run') == terminal, 'original result lost after reconnect'
        if change_page:
            c.call('checkpoint-select-original', 'session.select', {'id': checkpoint_session})
            recovered_changes = c.call('checkpoint-reconnected', 'checkpoint.changes', {'run_id': change_page['run_id']})
            assert recovered_changes == change_page, 'checkpoint comparison changed after reconnect'
            report['checkpoint_changes'] = dict(status='passed', run_id=change_page['run_id'], changes=1, recovered=True)

        if args.restore:
            preview = c.call('restore-preview', 'checkpoint.restore_preview', {'run_id': change_page['run_id'], 'paths': ['approved.txt']})
            assert not preview['conflicts'] and preview['actions'][0]['operation'] == 'remove'
            c.call('restore-unconfirmed', 'checkpoint.restore', {'confirmed': False, 'preview': preview}, expect_error='confirmation_required')
            assert (home / 'approved.txt').read_text() == 'approved once'
            params = {'confirmed': True, 'preview': preview}
            restored = c.call('restore-confirmed', 'checkpoint.restore', params)
            assert restored['completed'] and len(restored['files']) == 1 and restored['files'][0]['applied']
            assert not (home / 'approved.txt').exists(), 'confirmed restore failed to remove file'
            recovery = home / restored['files'][0]['recovery_path']
            assert recovery.read_bytes() == b'approved once', 'displaced bytes lost'
            (home / 'approved.txt').write_text('later user edit')
            assert c.call('restore-confirmed', 'checkpoint.restore', params) == restored
            c.close()
            clients.remove(c)
            c = Client(binary, home, endpoint, out, 'client-three', checkpoint_dir, checkpoint_flags)
            clients.append(c)
            c.call('hello', 'hello')
            assert c.call('restore-confirmed', 'checkpoint.restore', params) == restored, 'restore replay lost after restart'
            assert (home / 'approved.txt').read_text() == 'later user edit', 'restore replay overwrote user edit'
            rejected = c.call('restore-stale-new-id', 'checkpoint.restore', params)
            assert not rejected['completed'] and rejected['error'], 'stale preview accepted'
            assert (home / 'approved.txt').read_text() == 'later user edit'
            events = [json.loads(line)['event'] for line in (checkpoint_dir / 'restore-journal.jsonl').read_text().splitlines()]
            assert len(events) == 2 and [event['phase'] for event in events] == ['prepared', 'applied'], 'restore was repeated or not journalled'
            report['checkpoint_restore'] = dict(status='passed', confirmed=True, replay_after_restart=True, later_edit_preserved=True, journal_records=2)

        deadline = time.monotonic() + 3
        while len(cancelled) != 2 and time.monotonic() < deadline:
            time.sleep(.02)
        assert len(cancelled) == 2, 'provider requests not cancelled'
        assert len(provider_requests) == 5, 'expected approval, tool-result, steering continuation and two hanging calls'
        assert any(m.get('role') == 'tool' for m in provider_requests[1]['messages']), 'tool result continuation missing'
        assert provider_requests[2]['messages'][-1]['role'] == 'user' and 'steer note from Python' in str(provider_requests[2]['messages'][-1]['content']), 'steering continuation sequence differs'
        report.update(status='passed', scenarios=['non_go_client_lifecycle', 'approval_once_before_write', 'steering_delivery', 'followup_queue', 'duplicate_request_no_sideeffect_replay', 'version_error', 'cancel_terminal', 'disconnect_cleanup', 'reconnect_terminal_recovery'], provider_calls=len(provider_requests), cancelled_provider_requests=len(cancelled))
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        for c in clients:
            try:
                c.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        stop.set()
        server.shutdown()
        server.server_close()
        thread.join()
        (out / 'provider-requests.json').write_text(json.dumps(provider_requests, indent=2) + '\n')
        if checkpoint_dir and report['status'] == 'passed':
            try:
                snapshots = {}
                for file in checkpoint_dir.glob('*.json'):
                    value = json.loads(file.read_text())
                    assert value['Digest'] == file.stem
                    for digest, content in (value['Content'] or {}).items():
                        assert hashlib.sha256(base64.b64decode(content)).hexdigest() == digest
                    snapshots[file.stem] = value
                assert any(not v['Records'] for v in snapshots.values()), 'missing before snapshot'
                assert any(any(r['path'] == 'approved.txt' and base64.b64decode(v['Content'][r['hash']]) == b'approved once' for r in (v['Records'] or [])) for v in snapshots.values()), 'missing approved write snapshot'
                if args.checkpoint_scope:
                    for snapshot in snapshots.values():
                        assert snapshot['Exclusions'] == ['private'], 'configured scope missing'
                        assert any(o['path'] == 'private' for o in snapshot['Omissions']), 'exclusion not disclosed'
                        assert not any(r['path'].startswith('private/') for r in (snapshot['Records'] or []))
                        assert all(b'private fixture bytes' not in base64.b64decode(b) for b in snapshot['Content'].values()), 'excluded content persisted'
                    assert (home / 'private' / 'token').read_bytes() == b'private fixture bytes' * 100
                    report['checkpoint_scope'] = dict(status='passed', flags=checkpoint_flags, excluded_payload_preserved=True)
                annotations = []
                def walk(value):
                    if isinstance(value, dict):
                        if value.get('kind') == 'hand.checkpoint':
                            annotations.append(value['payload'])
                        for child in value.values(): walk(child)
                    elif isinstance(value, list):
                        for child in value: walk(child)
                for file in (out / 'home').rglob('*.jsonl'):
                    for line in file.read_text().splitlines(): walk(json.loads(line))
                starts = {a['pair']['RunID']: a['pair'] for a in annotations if a['phase'] == 'start'}
                finishes = [a['pair'] for a in annotations if a['phase'] == 'finish']
                assert len(starts) == 3 and len(finishes) == 3, 'missing persisted checkpoint associations'
                assert {p['RunID'] for p in finishes} == set(starts), 'checkpoint runs not fully paired'
                for pair in finishes:
                    assert pair['RunID'] in starts and starts[pair['RunID']]['Before'] == pair['Before']
                    assert pair['Before'] in snapshots
                    assert pair['After'] in snapshots and not pair['Error'], 'incomplete fixture checkpoint'
                report['checkpoints'] = dict(snapshots=len(snapshots), starts=len(starts), finishes=len(finishes), status='passed')
            except Exception as exc:
                report.update(status='failed', checkpoint_error=repr(exc))
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

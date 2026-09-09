#!/usr/bin/env python3
"""Exercise actual Hand RPC with a Python client and local provider fixture."""
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
    def __init__(self, binary, home, endpoint, evidence, name):
        self.stderr = (evidence / (name + '.stderr')).open('wb')
        user_home = evidence / 'home'
        user_home.mkdir(mode=0o700, exist_ok=True)
        self.process = subprocess.Popen([str(binary), '--rpc', '--model', 'local/fixture', '--base-url', endpoint],
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
    args = parser.parse_args()
    binary, out = args.hand_binary.resolve(strict=True), args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    home = out / 'workspace'
    home.mkdir(mode=0o700)
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
    clients = []
    try:
        c = Client(binary, home, endpoint, out, 'client-one')
        clients.append(c)
        c.call('hello', 'hello')
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
                c = Client(binary, home, endpoint, out, 'client-two')
                clients.append(c)
                c.call('hello', 'hello')
                assert c.terminal(request_id)['result']['payload']['status'] == 'cancelled'
        assert c.terminal('approval-run') == terminal, 'original result lost after reconnect'
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
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

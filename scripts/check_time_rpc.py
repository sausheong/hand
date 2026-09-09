#!/usr/bin/env python3
"""Verify actual CLI deadline cancellation, persistence and explicit renewal."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
import time
from check_rpc_client import Client


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    binary, out = args.hand_binary.resolve(strict=True), args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    work = out / 'workspace'
    work.mkdir()
    requests = []
    started = threading.Event()
    disconnected = threading.Event()
    stop = threading.Event()

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            if len(requests) == 1:
                started.set()
                try:
                    while not stop.wait(.05):
                        self.wfile.write(b': deadline fixture heartbeat\n\n')
                        self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    disconnected.set()
                return
            for event in [
                {'choices': [{'index': 0, 'delta': {'content': 'Local budget fixture completed.'}, 'finish_reason': None}]},
                {'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'stop'}],
                 'usage': {'prompt_tokens': 10, 'completion_tokens': 5, 'total_tokens': 15}},
            ]:
                self.wfile.write(('data: ' + json.dumps(event) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()

    server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    endpoint = f'http://127.0.0.1:{server.server_port}/v1'
    report = {'status': 'failed', 'qualification': 'development', 'paid_calls': 0,
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest()}
    client = None
    try:
        client = Client(binary, work, endpoint, out, 'first')
        client.call('hello', 'hello')
        session_id = client.call('new-session', 'session.new')['session_id']
        client.call('time-start', 'budget.time.decide', {'seconds': 2, 'confirmed': True})
        configured = client.call('configured', 'budget.time')
        assert configured['configured'] and not configured['expired']
        begin = time.monotonic()
        client.call('hang', 'prompt', {'text': 'Wait for the deadline in the local fixture.'})
        assert started.wait(3), 'provider did not start before deadline'
        expired = client.terminal('hang')['result']['payload']
        elapsed = time.monotonic() - begin
        assert expired['status'] == 'budget_exhausted' and expired['reason'] == 'session_time_budget'
        assert expired['state'] == 'idle' and not expired['verified']
        assert elapsed < 7, 'deadline did not join within allowance plus five seconds'
        assert disconnected.wait(5), 'provider stream remained connected after expiry'
        state = client.call('expired-state', 'budget.time')
        assert state['expired'] and state['remaining_millis'] == 0
        assert state['deadline'] == configured['deadline']
        assert len(requests) == 1
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'restart')
        client.call('hello', 'hello')
        client.call('select', 'session.select', {'id': session_id})
        restored = client.call('restored', 'budget.time')
        assert restored['expired'] and restored['deadline'] == state['deadline']
        client.call('time-start', 'budget.time.decide', {'seconds': 2, 'confirmed': True})
        assert client.call('after-replay', 'budget.time')['deadline'] == state['deadline']
        client.call('still-expired', 'prompt', {'text': 'Do not renew the deadline implicitly.'})
        assert client.terminal('still-expired')['result']['payload']['reason'] == 'session_time_budget'
        assert len(requests) == 1, 'expired restart dispatched provider'
        client.call('renew', 'budget.time.decide', {'seconds': 60, 'confirmed': True})
        renewed = client.call('renewed', 'budget.time')
        assert not renewed['expired'] and renewed['deadline'] != state['deadline']
        client.call('resumed', 'prompt', {'text': 'Complete the renewed local fixture.'})
        completed = client.terminal('resumed')['result']['payload']
        assert completed['status'] == 'completed' and len(requests) == 2
        client.close()
        client = None
        report.update(status='development_passed', session_id=session_id, expired=expired,
                      configured=configured, restored=restored, renewed=renewed,
                      expiry_elapsed_seconds=elapsed, provider_disconnected=True, clean_exits=2)
    finally:
        try:
            if client is not None:
                client.close()
        finally:
            stop.set()
            server.shutdown()
            thread.join(timeout=5)
            server.server_close()
            (out / 'provider-requests.json').write_text(json.dumps(requests, indent=2) + '\n')
            report['local_provider_requests'] = len(requests)
            (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Check actual CLI token exhaustion, restart and explicit resume locally."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
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

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
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
        client.call('unconfirmed', 'budget.tokens.decide', {'limit': 1}, expect_error='confirmation_required')
        client.call('small-limit', 'budget.tokens.decide', {'limit': 1, 'confirmed': True})
        client.call('exhaust', 'prompt', {'text': 'Complete the local fixture.'})
        exhausted = client.terminal('exhaust')['result']['payload']
        assert exhausted['status'] == 'budget_exhausted' and exhausted['reason'] == 'session_token_budget'
        assert not exhausted.get('verified', False)
        state = client.call('exhausted-state', 'budget.tokens')
        assert state['exhausted'] and state['committed'] == 0 and state['attempts'] == 0
        assert not requests, 'denied request reached the provider'
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'restart')
        client.call('hello', 'hello')
        client.call('select', 'session.select', {'id': session_id})
        assert client.call('restored-state', 'budget.tokens') == state
        client.call('still-exhausted', 'prompt', {'text': 'Try again.'})
        assert client.terminal('still-exhausted')['result']['payload']['status'] == 'budget_exhausted'
        assert not requests, 'restart bypassed exhaustion'
        client.call('resume', 'budget.tokens.decide', {'limit': 100000, 'confirmed': True})
        client.call('small-limit', 'budget.tokens.decide', {'limit': 1, 'confirmed': True})
        assert client.call('after-replay', 'budget.tokens')['limit'] == 100000
        client.call('resumed-run', 'prompt', {'text': 'Complete after explicit resume.'})
        completed = client.terminal('resumed-run')['result']['payload']
        assert completed['status'] == 'completed'
        charged = client.call('charged', 'budget.tokens')
        assert charged['committed'] == 15 and charged['attempts'] == 1 and charged['uncertain_attempts'] == 0
        assert len(requests) == 1
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'charged-restart')
        client.call('hello', 'hello')
        client.call('select-again', 'session.select', {'id': session_id})
        assert client.call('durable-charge', 'budget.tokens') == charged
        client.close()
        client = None
        report.update(status='development_passed', session_id=session_id, exhausted=exhausted,
                      charged=charged, completed=completed, clean_exits=3)
    finally:
        try:
            if client is not None:
                client.close()
        finally:
            server.shutdown()
            thread.join(timeout=5)
            server.server_close()
            (out / 'provider-requests.json').write_text(json.dumps(requests, indent=2) + '\n')
            report['local_provider_requests'] = len(requests)
            (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

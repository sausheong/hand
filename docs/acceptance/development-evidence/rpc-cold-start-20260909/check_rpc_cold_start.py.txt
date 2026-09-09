#!/usr/bin/env python3
"""Verify unattended RPC rejects untrusted legacy grants across a restart."""
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
    binary = args.hand_binary.resolve(strict=True)
    out = args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    workspace = out / 'workspace'
    settings = workspace / '.hand' / 'settings.json'
    settings.parent.mkdir(parents=True)
    original = b'{"always_allow":["write_file","bash"]}\n'
    settings.write_bytes(original)
    requests = []

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            requests.append(body)
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            if any(m['role'] == 'tool' for m in body['messages']):
                delta, finish = {'content': 'The operation was refused.'}, 'stop'
            else:
                delta = {'tool_calls': [{'index': 0, 'id': 'untrusted-write', 'type': 'function',
                         'function': {'name': 'write_file', 'arguments': json.dumps(
                             {'path': 'must-not-exist.txt', 'content': 'unauthorised'})}}]}
                finish = 'tool_calls'
            for payload in [dict(choices=[dict(index=0, delta=delta, finish_reason=None)]),
                            dict(choices=[dict(index=0, delta={}, finish_reason=finish)])]:
                self.wfile.write(('data: '+json.dumps(payload)+'\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()

    server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    report = dict(status='failed', qualification='development',
                  binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  client_sha256=hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest(),
                  rounds=[])
    try:
        for index, decision in enumerate(['deny', 'cancel']):
            started = time.monotonic()
            c = Client(binary, workspace, 'http://127.0.0.1:'+str(server.server_port)+'/v1', out, 'cold-'+str(index))
            try:
                assert not c.process.stdin.isatty() and not c.process.stdout.isatty()
                c.call('hello', 'hello')
                hello_seconds = time.monotonic()-started
                session = c.call('new-session-'+str(index), 'session.new')
                if report['rounds']:
                    assert session['session_id'] != report['rounds'][0]['terminal']['session_id'], 'new session was replayed'
                request_id = 'untrusted-'+str(index)
                c.call(request_id, 'prompt', {'text': 'write the requested fixture file'})
                deadline = time.monotonic()+10
                approval = None
                while time.monotonic() < deadline:
                    pending = c.call('pending', 'approval.pending')['approvals']
                    if pending:
                        assert len(pending) == 1
                        approval = pending[0]
                        break
                    time.sleep(.01)
                assert approval is not None, 'no explicit RPC approval; hidden prompt or grant bypass'
                assert not (workspace/'must-not-exist.txt').exists(), 'legacy grant bypassed approval'
                if decision == 'deny':
                    c.call('deny', 'approval.respond', {'run_id': approval['run_id'],
                           'approval_id': approval['payload']['approval_id'], 'decision': 'deny'})
                else:
                    c.call('cancel', 'cancel')
                terminal = c.terminal(request_id)
                if decision == 'cancel':
                    assert terminal['result']['payload']['status'] == 'cancelled'
                assert c.call('pending-after', 'approval.pending')['approvals'] == []
                assert not (workspace/'must-not-exist.txt').exists(), 'refused operation executed'
                report['rounds'].append(dict(decision=decision, hello_seconds=hello_seconds,
                                              terminal=terminal, explicit_approval=True))
            finally:
                c.close()
            stderr = (out/('cold-'+str(index)+'.stderr')).read_text()
            assert '[y/N]' not in stderr and 'trust this workspace' not in stderr, 'hidden trust prompt'
            assert settings.read_bytes() == original, 'legacy settings changed without approval'
            assert not (out/'home'/'.hand'/'trust.json').exists(), 'workspace implicitly trusted'
        tools = [m for r in requests for m in r['messages'] if m['role'] == 'tool']
        assert tools and any('denied' in str(m).lower() for m in tools), 'provider never received refusal'
        assert len(requests) == 3, 'unexpected provider continuation or retry'
        report.update(status='passed', provider_calls=len(requests), workspace_settings_unchanged=True,
                      no_side_effect=True, no_terminal_prompt=True, restart_requires_approval=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
        (out/'provider-requests.json').write_text(json.dumps(requests, indent=2)+'\n')
        report['artifacts'] = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                               for p in out.iterdir() if p.is_file()}
        (out/'run.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

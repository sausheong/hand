#!/usr/bin/env python3
"""Verify profile-bound authority through a real Hand RPC process and local HTTP fixture."""
import argparse
import hashlib
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import threading
import time
from check_rpc_client import Client


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    binary, out = args.hand_binary.resolve(strict=True), args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    workspace = out / 'workspace'
    workspace.mkdir(mode=0o700)
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
                delta, finish = {'content': 'finished'}, 'stop'
            else:
                content = next(m['content'] for m in reversed(body['messages']) if m['role'] == 'user')
                delta = {'tool_calls': [{'index': 0, 'id': 'profile-write', 'type': 'function', 'function': {
                    'name': 'write_file', 'arguments': json.dumps({'path': 'scoped.txt', 'content': str(content)})}}]}
                finish = 'tool_calls'
            for value in [dict(choices=[dict(index=0, delta=delta, finish_reason=None)]),
                          dict(choices=[dict(index=0, delta={}, finish_reason=finish)])]:
                self.wfile.write(('data: ' + json.dumps(value) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()

    server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
    worker = threading.Thread(target=server.serve_forever)
    worker.start()
    endpoint = f'http://127.0.0.1:{server.server_port}/v1'
    config_dir = out / 'home' / '.hand'
    config_dir.mkdir(parents=True, mode=0o700)
    config = {'profiles': {name: {'provider': 'local', 'model': name, 'endpoint': endpoint}
                           for name in ['first', 'second']}}
    (config_dir / 'config.json').write_text(json.dumps(config))
    report = {'status': 'failed', 'qualification': 'development',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest()}
    client = None
    try:
        client = Client(binary, workspace, endpoint, out, 'profile-client')
        client.call('hello', 'hello')
        grant_id = None
        for index, (profile, decision, expected_prompts) in enumerate([
                ('first', 'always', 1), ('second', 'deny', 1), ('first', None, 0)]):
            client.call(f'profile-{index}', 'profile.select', {'name': profile})
            grants = client.call('inspect', 'permission.list')['grants']
            if index < 2:
                assert not grants, 'unapproved profile inherited authority'
            else:
                assert len(grants) == 1 and grants[0]['ID'] == grant_id, 'original grant not restored'
            client.call(f'session-{index}', 'session.new')
            request_id = f'run-{index}'
            content = f'profile-content-{index}'
            client.call(request_id, 'prompt', {'text': content})
            deadline = time.monotonic() + 15
            prompts = 0
            while time.monotonic() < deadline:
                pending = client.call('pending', 'approval.pending')['approvals']
                for approval in pending:
                    prompts += 1
                    assert decision is not None and prompts == 1, 'unexpected approval request'
                    target = workspace / 'scoped.txt'
                    assert (not target.exists()) if index == 0 else target.read_text() == 'profile-content-0', 'write before approval'
                    client.call(f'answer-{index}', 'approval.respond', {'run_id': approval['run_id'],
                                'approval_id': approval['payload']['approval_id'], 'decision': decision})
                record = client.call('lookup', 'request.get', {'id': request_id})
                if record['state'] == 'completed':
                    break
                time.sleep(.01)
            else:
                raise RuntimeError('profile run did not terminate')
            terminal = client.terminal(request_id)
            assert terminal['result']['payload']['status'] == 'completed', terminal
            assert prompts == expected_prompts, 'wrong approval count'
            assert (workspace / 'scoped.txt').read_text() == ('profile-content-0' if index == 1 else content), 'incorrect file mutation'
            if index == 0:
                grants = client.call('inspect', 'permission.list')['grants']
                assert len(grants) == 1 and grants[0]['Operation'] == 'file.write'
                grant_id = grants[0]['ID']
        assert len(requests) == 6, 'unexpected provider request count'
        assert [r['model'] for r in requests] == ['first', 'first', 'second', 'second', 'first', 'first']
        report.update(status='passed', provider_calls=len(requests), scenarios=[
            'profile_change_reapproval', 'denial_prevents_write', 'return_to_profile_reuses_grant', 'routine_prompt_count'])
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if client is not None:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        server.shutdown()
        server.server_close()
        worker.join()
        (out / 'provider-requests.json').write_text(json.dumps(requests, indent=2) + '\n')
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

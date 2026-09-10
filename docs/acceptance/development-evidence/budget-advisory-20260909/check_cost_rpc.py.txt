#!/usr/bin/env python3
"""Check monetary admission and durable tariff accounting through the actual CLI."""
import argparse
from datetime import datetime, timedelta, timezone
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
        client.call('tokens', 'budget.tokens.decide', {'limit': 100000, 'confirmed': True})
        client.call('strict', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 1000000, 'strict': True, 'confirmed': True})
        client.call('unknown-price', 'prompt', {'text': 'Do not dispatch without a known price.'})
        unknown = client.terminal('unknown-price')['result']['payload']
        assert unknown['status'] == 'budget_exhausted' and unknown['reason'] == 'cost_price_unknown'
        assert not requests
        assert client.call('tokens-after-denial', 'budget.tokens')['committed'] == 0, 'cost denial leaked token reservation'
        now = datetime.now(timezone.utc)
        price = {'provider': 'local', 'model': 'fixture', 'destination': endpoint,
                 'currency': 'USD', 'source': 'scripted local fixture; not a market tariff', 'version': 'fixture-1',
                 'effective_at': (now - timedelta(hours=1)).isoformat(),
                 'expires_at': (now + timedelta(hours=1)).isoformat(),
                 'input_nano_per_million': 1000000, 'output_nano_per_million': 1000000,
                 'cache_write_nano_per_million': 1000000, 'cache_read_nano_per_million': 1000000,
                 'fixed_nano': 0, 'all_charges_bounded': True}
        for label, changed_price in [
            ('expired', dict(price, effective_at=(now - timedelta(hours=2)).isoformat(),
                             expires_at=(now - timedelta(hours=1)).isoformat())),
            ('unbounded', dict(price, all_charges_bounded=False)),
        ]:
            client.call(label + '-tariff', 'budget.prices.set', {'prices': [changed_price], 'confirmed': True})
            client.call(label + '-prompt', 'prompt', {'text': 'Strict admission must refuse ' + label + ' pricing.'})
            outcome = client.terminal(label + '-prompt')['result']['payload']
            assert outcome['status'] == 'budget_exhausted' and outcome['reason'] == 'cost_price_unknown'
            assert not requests, label + ' tariff reached provider'
            assert client.call(label + '-tokens', 'budget.tokens')['committed'] == 0
        client.call('install', 'budget.prices.set', {'prices': [price], 'confirmed': True})
        client.call('tiny', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 1, 'strict': True, 'confirmed': True})
        client.call('exhaust', 'prompt', {'text': 'Do not dispatch beyond the monetary ceiling.'})
        exhausted = client.terminal('exhaust')['result']['payload']
        assert exhausted['status'] == 'budget_exhausted' and exhausted['reason'] == 'session_cost_budget'
        assert not requests
        state = client.call('exhausted-state', 'budget.cost')
        assert state['exhausted'] and state['committed_nano'] == 0
        installed = client.call('installed', 'budget.prices')
        assert len(installed) == 1 and installed[0]['destination'] == endpoint
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'restart')
        client.call('hello', 'hello')
        client.call('select', 'session.select', {'id': session_id})
        assert client.call('restored', 'budget.cost') == state
        assert client.call('restored-prices', 'budget.prices') == installed
        client.call('still-exhausted', 'prompt', {'text': 'Restart does not reset money.'})
        assert client.terminal('still-exhausted')['result']['payload']['reason'] == 'session_cost_budget'
        assert not requests
        client.call('resume', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 1000000, 'strict': True, 'confirmed': True})
        client.call('tiny', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 1, 'strict': True, 'confirmed': True})
        assert client.call('after-replay', 'budget.cost')['limit_nano'] == 1000000
        client.call('resumed-run', 'prompt', {'text': 'Complete one local fixture request.'})
        completed = client.terminal('resumed-run')['result']['payload']
        assert completed['status'] == 'completed'
        charged = client.call('charged', 'budget.cost')
        assert charged['committed_nano'] == 15 and charged['attempts'] == 1
        assert charged['uncertain_attempts'] == 0 and charged['unpriced_attempts'] == 0
        assert len(requests) == 1
        assert client.call('reported-tokens', 'budget.tokens')['committed'] == 15
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'charged-restart')
        client.call('hello', 'hello')
        client.call('select-again', 'session.select', {'id': session_id})
        assert client.call('durable-charge', 'budget.cost') == charged
        assert client.call('durable-prices', 'budget.prices') == installed
        client.call('advisory', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 1000000, 'strict': False, 'confirmed': True})
        client.call('clear-prices', 'budget.prices.set', {'prices': [], 'confirmed': True})
        client.call('advisory-prompt', 'prompt', {'text': 'Complete with explicitly unknown advisory pricing.'})
        advisory_terminal = client.terminal('advisory-prompt')['result']['payload']
        assert advisory_terminal['status'] == 'completed'
        advisory = client.call('advisory-state', 'budget.cost')
        assert advisory['committed_nano'] == 15 and advisory['attempts'] == 2
        assert advisory['unpriced_attempts'] == 1 and advisory['uncertain_attempts'] == 1
        assert not advisory['strict'] and 'Unknown charges are not free' in advisory['disclosure']
        assert len(requests) == 2
        client.close()
        client = None
        client = Client(binary, work, endpoint, out, 'advisory-restart')
        client.call('hello', 'hello')
        client.call('select-advisory', 'session.select', {'id': session_id})
        assert client.call('durable-advisory', 'budget.cost') == advisory
        client.call('advisory-prompt', 'prompt', {'text': 'Complete with explicitly unknown advisory pricing.'})
        assert len(requests) == 2, 'durable replay repeated provider call'
        client.call('restore-prices', 'budget.prices.set', {'prices': [price], 'confirmed': True})
        client.call('strict-after-unknown', 'budget.cost.decide', {'currency': 'USD', 'limit_nano': 2000000, 'strict': True, 'confirmed': True}, expect_error='budget_rejected')
        assert client.call('uncertainty-retained', 'budget.cost') == advisory
        client.close()
        client = None
        report.update(advisory=advisory, advisory_terminal=advisory_terminal,
                      strict_refusal_cases=['missing', 'expired', 'unbounded'], status='development_passed' , session_id=session_id, exhausted=exhausted,
                      charged=charged, completed=completed, unknown_price=unknown, installed_prices=installed, clean_exits=4)
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

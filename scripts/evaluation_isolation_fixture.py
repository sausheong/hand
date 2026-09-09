"""Synthetic upstream and actual Hand process for the container boundary probe.

No provider connection or real credential is used. The gateway and agent run in
separate PID/filesystem namespaces; their sole shared resource is a Unix socket.
"""
import hashlib
import http.client
import json
import os
from pathlib import Path
import select
import signal
import socket
import socketserver
import subprocess
import sys
import threading

MODEL = 'claude-sonnet-4-5-20250929'
AUTHORITY = '127.0.0.1:8080'


def gateway(config):
    from evaluation_budget import EvaluationBudget
    from evaluation_gateway import GatewayController
    from evaluation_http_server import GatewayHTTPServer
    root = Path('/audit'); requests = []; threads = []; failures = []
    evidence = root/'requests'; evidence.mkdir(mode=0o700)
    ledger = EvaluationBudget(root/'budget.sqlite', 'a'*64, 100000000)
    ledger.add_run('probe', 5, 100000, 25000, 100000000, 120); ledger.close()
    contract = dict(schema_version=1, provider='anthropic', model=MODEL, source_sha256='b'*64,
                    billing='text_tokens_with_partitioned_cache', context_tokens=10000,
                    max_output_tokens=4096, max_request_fee_usd='0',
                    price_sets=[dict(input='1', output='2', cache_read='1', cache_write='1')])
    runs = [dict(id='probe', token=config['token'], model=MODEL, contract=contract,
                 allowed_beta_sets=[[], ['interleaved-thinking-2025-05-14']], timeout_seconds=5)]

    def factory(timeout):
        client, upstream = socket.socketpair(); client.settimeout(timeout); upstream.settimeout(5)
        def respond():
            try:
                with upstream.makefile('rb') as stream:
                    first = stream.readline(); headers = {}
                    while True:
                        line = stream.readline()
                        if line in (b'\r\n', b''): break
                        key, value = line.decode().split(':', 1); headers[key.lower()] = value.strip()
                    body = json.loads(stream.read(int(headers['content-length'])))
                authenticated = headers.get('x-api-key') == config['provider_key']
                if not authenticated: raise ValueError('provider credential substitution failed')
                requests.append(dict(provider_credential_substituted=authenticated,
                                     tool_marker_seen='ISOLATED_HAND_MARKER' in json.dumps(body['messages'])))
                events = [
                    dict(type='message_start', message=dict(id='isolation-fixture', type='message', role='assistant', model=MODEL,
                         content=[], stop_reason=None, stop_sequence=None, usage=dict(input_tokens=10, output_tokens=0))),
                    dict(type='content_block_start', index=0, content_block=dict(type='text', text='')),
                    dict(type='content_block_delta', index=0, delta=dict(type='text_delta', text='isolated gateway fixture completed')),
                    dict(type='content_block_stop', index=0),
                    dict(type='message_delta', delta=dict(stop_reason='end_turn', stop_sequence=None), usage=dict(output_tokens=2)),
                    dict(type='message_stop')]
                if len(requests) == 1:
                    events[1]['content_block'] = dict(type='tool_use', id='isolated-read', name='read_file', input={})
                    events[2]['delta'] = dict(type='input_json_delta', partial_json=json.dumps({'path':'example.go'}))
                    events[4]['delta']['stop_reason'] = 'tool_use'
                elif not requests[-1]['tool_marker_seen']:
                    raise ValueError('actual tool result absent from second request')
                raw = ''.join('event: '+e['type']+'\ndata: '+json.dumps(e)+'\n\n' for e in events).encode()
                upstream.sendall(b'HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw)
            except Exception as error: failures.append(type(error).__name__)
            finally: upstream.close()
        thread = threading.Thread(target=respond); thread.start(); threads.append(thread)
        connection = http.client.HTTPConnection('api.anthropic.com', timeout=timeout)
        connection.connect = lambda: setattr(connection, 'sock', client)
        return connection

    class UnixGateway(GatewayHTTPServer):
        address_family = socket.AF_UNIX
        def server_bind(self):
            socketserver.TCPServer.server_bind(self)
            self.server_name = 'isolated-gateway'; self.server_port = 0
    controller = GatewayController(root/'budget.sqlite', 'a'*64, evidence, runs, config['provider_key'], AUTHORITY, connection_factory=factory)
    server = UnixGateway('/socket/gateway.sock', controller, io_timeout=2, intake_timeout=2)
    stop = threading.Event()
    for sig in (signal.SIGTERM, signal.SIGINT): signal.signal(sig, lambda *_: stop.set())
    serving = threading.Thread(target=lambda: server.serve_forever(poll_interval=.05)); serving.start()
    (root/'ready').write_text('ready')
    try: stop.wait(120)
    finally:
        controller.close(); server.shutdown(); server.server_close(); serving.join(3)
        for thread in threads: thread.join(3)
        ledger = EvaluationBudget(root/'budget.sqlite', 'a'*64)
        try: totals = ledger.totals()
        finally: ledger.close()
        (root/'gateway-report.json').write_text(json.dumps(dict(requests=requests, failures=failures,
            requests_charged=totals['requests'], joined=not serving.is_alive() and all(not t.is_alive() for t in threads)), indent=2)+'\n')


def agent(config):
    class Bridge(socketserver.BaseRequestHandler):
        def handle(self):
            with socket.socket(socket.AF_UNIX) as upstream:
                upstream.settimeout(5); upstream.connect('/socket/gateway.sock')
                self.request.settimeout(5)
                while True:
                    ready, _, _ = select.select([upstream, self.request], [], [], 5)
                    if not ready: return
                    for source in ready:
                        data = source.recv(65536)
                        if not data: return
                        (self.request if source is upstream else upstream).sendall(data)
    server = socketserver.ThreadingTCPServer(('127.0.0.1', 8080), Bridge)
    serving = threading.Thread(target=lambda: server.serve_forever(poll_interval=.05)); serving.start()
    result = {}; root = Path('/workspace')
    try:
        # These probes have the same UID, capabilities, mounts and network as Hand.
        forbidden = [config['host_sentinel'], '/private-config/config.json', '/var/run/docker.sock', '/audit/budget.sqlite']
        result['inaccessible_host_and_gateway_paths'] = {p: not Path(p).exists() for p in forbidden}
        observed = []
        for p in Path('/proc').glob('[0-9]*/environ'):
            try: observed.extend(p.read_bytes().split(b'\0'))
            except OSError: pass
        result['provider_secret_absent'] = all(hashlib.sha256(v.partition(b'=')[2]).hexdigest() != config['provider_sha256'] for v in observed)
        result['external_connections_denied'] = {}
        for address in [('1.1.1.1', 443), (config['outside_ip'], 8765)]:
            try:
                with socket.create_connection(address, timeout=1): denied = False
            except OSError: denied = True
            result['external_connections_denied'][str(address)] = denied
        flags = {name: int(Path('/sys/class/net', name, 'flags').read_text().strip(), 16)
                 for _, name in socket.if_nameindex()}
        routes = Path('/proc/net/route').read_text().splitlines()[1:]
        ipv6 = Path('/proc/net/if_inet6').read_text().splitlines()
        result['network_state'] = dict(interface_flags=flags, ipv4_routes=routes, ipv6_addresses=ipv6)
        # Linux creates dormant tunnel devices even in a network=none namespace.
        # They must have no UP flag, and only loopback may have an IPv6 address.
        result['no_routable_external_interface'] = ({n for n, flags in flags.items() if flags & 1} == {'lo'}
            and not routes and all(line.split()[-1] == 'lo' for line in ipv6))
        # Socket access is not authority to change the upstream or run identity.
        body = dict(model=MODEL, max_tokens=4, messages=[dict(role='user', content='probe')])
        cases = [('wrong-token', '/v1/messages', dict(body), 'wrong-token', AUTHORITY),
                 ('wrong-model', '/v1/messages', dict(body, model='wrong-model'), config['token'], AUTHORITY),
                 ('foreign-host', '/v1/messages', dict(body), config['token'], 'api.anthropic.com'),
                 ('absolute-target', 'https://api.anthropic.com/v1/messages', dict(body), config['token'], AUTHORITY)]
        result['gateway_rejections'] = {}
        for name, target, payload, token, host in cases:
            connection = http.client.HTTPConnection('127.0.0.1', 8080, timeout=5)
            connection.request('POST', target, json.dumps(payload), {'Host':host, 'Content-Type':'application/json', 'Anthropic-Version':'2023-06-01', 'X-Api-Key':token})
            response = connection.getresponse(); response.read(); connection.close()
            result['gateway_rejections'][name] = response.status == 400
        home = Path('/tmp/agent-home'); (home/'.hand').mkdir(parents=True, mode=0o700)
        (home/'.hand/config.json').write_text(json.dumps(dict(default_profile='probe', profiles=dict(probe=dict(
            provider='anthropic', model=MODEL, endpoint='http://'+AUTHORITY, credential_env='HAND_FIXTURE_TOKEN',
            context_limit=10000, max_output=4096, input_types=['text'])))))
        env = dict(PATH='/usr/local/bin:/usr/bin:/bin', HOME=str(home), HAND_FIXTURE_TOKEN=config['token'])
        run = subprocess.run(['/opt/hand', '--profile=probe', '--jsonl', '--max-turns=3', '-p', 'Read example.go and report its marker.'], cwd=root, env=env, capture_output=True, timeout=30)
        (root/'hand.jsonl').write_bytes(run.stdout); (root/'hand.stderr').write_bytes(run.stderr)
        events = [json.loads(line) for line in run.stdout.splitlines()]
        terminals = [e for e in events if e.get('kind') == 'terminal']
        tools = [e['payload']['result'] for e in events if 'result' in e.get('payload', {})]
        result['hand_completed'] = run.returncode == 0 and len(terminals) == 1 and terminals[0] == events[-1] and terminals[0]['payload']['status'] == 'completed'
        result['actual_tool_marker_read'] = len(tools) == 1 and not tools[0]['error'] and 'ISOLATED_HAND_MARKER' in tools[0]['output']
        result['passed'] = all(result['inaccessible_host_and_gateway_paths'].values()) and result['provider_secret_absent'] and all(result['external_connections_denied'].values()) and result['no_routable_external_interface'] and all(result['gateway_rejections'].values()) and result['hand_completed'] and result['actual_tool_marker_read']
    finally:
        server.shutdown(); server.server_close(); serving.join(3)
        result['proxy_joined'] = not serving.is_alive()
        (root/'agent-report.json').write_text(json.dumps(result, indent=2)+'\n')
    if not result.get('passed') or not result['proxy_joined']: raise RuntimeError('isolation probe failed')


if __name__ == '__main__':
    config = json.loads(Path(sys.argv[2]).read_text())
    {'gateway':gateway, 'agent':agent}[sys.argv[1]](config)

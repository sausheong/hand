"""Run the actual Hand binary against a local synthetic gateway tool turn."""
import hashlib
import json
import os
from pathlib import Path
import sys
from urllib.parse import urlparse
from evaluation_hand_run import run_hand


def check(binary, config_path, report_path):
    binary = Path(binary).resolve()
    report_path = Path(report_path).resolve()
    output = report_path.parent
    config = json.loads(Path(config_path).read_text())
    url = urlparse(config['url'])
    if url.scheme != 'http' or url.hostname != '127.0.0.1' or url.username or url.path:
        raise ValueError('explicit loopback gateway required')
    home = output/'agent-home'; home.mkdir(mode=0o700)
    workspace = output/'workspace'; workspace.mkdir()
    (workspace/'example.go').write_text('package main\n// HAND_GATEWAY_READ_PROBE\n')
    settings = home/'.hand'; settings.mkdir(mode=0o700)
    (settings/'config.json').write_text(json.dumps(dict(default_profile='probe', profiles=dict(probe=dict(
        provider='anthropic', model='claude-sonnet-4-5-20250929', endpoint=config['url'],
        credential_env='HAND_FIXTURE_TOKEN', context_limit=10000, max_output=4096, input_types=['text'])))))
    argv = [str(binary), '--profile=probe', '--jsonl', '--max-turns=3', '-p', 'Read example.go and report its marker.']
    report = dict(status='failed', argv=argv, binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  limitations=['Synthetic upstream only; explicit profile and three-turn cap are probe settings.',
                               'Fresh HOME and fixture credential; this is not container network isolation.'])
    try:
        result = run_hand(argv, workspace, output/'process',
                          dict(PATH=os.environ['PATH'], HOME=str(home), HAND_FIXTURE_TOKEN=config['token']),
                          timeout_seconds=25, max_output_bytes=16 << 20)
        stdout = Path(result['process']['logs']['stdout']['path']).read_bytes()
        report['exit_code'] = result['process']['exit_code']
        report['supervision'] = result
        report['observation'] = result['observation']
        if result['status'] != 'observed_unverified':
            raise ValueError('invalid Hand event transcript')
        events = [json.loads(line) for line in stdout.split(b'\n') if line]
        report['event_kinds'] = [e['kind'] for e in events]
        terminal = [e for e in events if e['kind'] == 'terminal']
        results = [e for e in events if 'result' in e['payload']]
        report['tool_results'] = results
        if report['exit_code'] or len(terminal) != 1 or terminal[0] != events[-1] or terminal[0]['payload']['status'] != 'completed':
            raise ValueError('incomplete Hand session')
        if len(results) != 1 or results[0]['payload']['result']['error'] or 'HAND_GATEWAY_READ_PROBE' not in results[0]['payload']['result']['output']:
            raise ValueError('successful actual read result required')
        if 'offline gateway fixture' not in ''.join(e['payload'].get('text', '') for e in events if e['kind'] == 'text'):
            raise ValueError('final fixture text missing')
        report['status'] = 'sdk_gateway_passed'
    except Exception as error:
        report['error_type'] = type(error).__name__
        raise
    finally:
        report_path.write_text(json.dumps(report, indent=2)+'\n')


if __name__ == '__main__':
    check(*sys.argv[1:])

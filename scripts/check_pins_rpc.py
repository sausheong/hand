#!/usr/bin/env python3
"""Verify built Hand pin persistence and replay across RPC process restart."""
import argparse
import hashlib
import json
from pathlib import Path
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
    report = {'status': 'failed', 'qualification': 'development',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest(),
              'model_runs_requested': 0}
    client = None
    pin = {'id': 'objective', 'kind': 'objective', 'text': 'Preserve this objective across restart'}
    constraint = {'id': 'constraint', 'kind': 'constraint', 'text': 'Keep user changes'}
    try:
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'first')
        client.call('hello', 'hello')
        session_id = client.call('new-pinned-session', 'session.new')['session_id']
        assert client.call('pin-objective', 'context.pin', pin) == {'updated': 'objective'}
        client.call('pin-constraint', 'context.pin', constraint)
        client.call('bad-pin', 'context.pin', {**pin, 'kind': 'invalid'}, expect_error='pin_rejected')
        assert client.call('list-before', 'context.pins') == [pin, constraint]
        estimate = client.call('inspect-pins', 'context.inspect')
        assert any(x['kind'] == 'pins' and x['count'] == 2 and x['estimated_tokens'] > 0 for x in estimate['contributions'])
        client.close()
        client = None
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'restarted')
        client.call('hello', 'hello')
        client.call('select-pinned', 'session.select', {'id': session_id})
        assert client.call('list-after', 'context.pins') == [pin, constraint]
        assert client.call('remove-objective', 'context.unpin', {'id': 'objective'}) == {'removed': 'objective'}
        assert client.call('pin-objective', 'context.pin', pin) == {'updated': 'objective'}
        assert client.call('list-replayed', 'context.pins') == [constraint], 'replayed request recreated removed pin'
        client.call('remove-constraint', 'context.unpin', {'id': 'constraint'})
        assert not client.call('empty', 'context.pins')
        client.close()
        client = None
        report.update(status='development_passed', session_id=session_id,
                      pins_survived_restart=True, replay_did_not_recreate=True,
                      clean_exits=2, inspection=estimate)
    finally:
        if client is not None:
            client.close()
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

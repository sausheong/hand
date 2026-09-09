#!/usr/bin/env python3
"""Verify named command evidence with real Hand RPC clients; no provider calls."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import time
from check_rpc_client import Client


def completed(client, request_id):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        record = client.call('get-' + request_id, 'request.get', {'id': request_id})
        if record['state'] == 'completed':
            response = record['result']
            assert not response.get('error'), response
            return response['result']
        time.sleep(.01)
    raise RuntimeError('verification completion timed out')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    out = args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    work = out / 'workspace'
    work.mkdir(mode=0o700)
    evidence = out / 'verification'
    evidence.mkdir(mode=0o700)
    (work / 'source').write_text('original source')
    config = out / 'verification.json'
    config.write_text(json.dumps({'directory': str(evidence), 'profiles': [
        {'name': 'pass', 'command': ['/bin/sh', '-c', 'cat source']},
        {'name': 'fail', 'command': ['/bin/sh', '-c', 'printf diagnostic >&2; exit 7']},
        {'name': 'hang', 'command': ['/bin/sh', '-c', 'printf %s "$$" > marker; exec sleep 30']},
    ]}))
    report = {'status': 'failed', 'qualification': 'development',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_runner_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest()}
    clients = []
    def connect(name):
        c = Client(binary, work, 'http://127.0.0.1:1/v1', out, name, out / 'checkpoints', ['--verification-config', str(config)])
        clients.append(c)
        c.call('hello', 'hello')
        return c
    try:
        c = connect('client-one')
        profiles = {p['name']: p for p in c.call('profiles', 'verification.profiles')}
        def params(name):
            return {'confirmed': True, 'profile': name, 'digest': profiles[name]['digest']}
        c.call('not-confirmed', 'verification.run', {'profile': 'pass', 'digest': profiles['pass']['digest']}, expect_error='verification_rejected')
        c.call('pass', 'verification.run', params('pass'))
        passed = completed(c, 'pass')
        result = passed['verification']
        assert passed['completed'] and result['assessment']['status'] == 'passed'
        assert result['record']['stdout'] == 'original source' and result['record']['exit_code'] == 0
        record_file = evidence / (result['id'] + '.json')
        assert record_file.is_file() and hashlib.sha256(record_file.read_bytes()).hexdigest() == result['id']
        state = c.call('context-after-pass', 'context.state')
        references = [item for item in state['items'] if item['reference'] == 'hand-verification:' + result['id']]
        assert len(references) == 1 and result['record']['before'] in references[0]['text']
        assert 'assessment_at_check' in references[0]['text'] and 'Recheck saved evidence' in references[0]['text']
        c.call('fail', 'verification.run', params('fail'))
        failed = completed(c, 'fail')['verification']
        assert failed['record']['exit_code'] == 7 and failed['assessment']['status'] == 'failed'
        (work / 'source').write_text('later user edit')
        checked = c.call('stale', 'verification.check', {'profile': 'pass', 'id': result['id']})
        assert checked['assessment']['status'] == 'stale'
        retained = c.call('context-after-edit', 'context.state')
        assert len(retained['items']) == 2
        reference = next(item for item in retained['items'] if item['reference'] == 'hand-verification:' + result['id'])
        assert '\"assessment_at_check\":\"stale\"' in reference['text']
        c.close(); clients.remove(c)
        c = connect('client-two')
        assert c.call('context-after-restart', 'context.state') == retained, 'structured evidence state changed on restart'

        c.call('pass', 'verification.run', params('pass'))
        assert completed(c, 'pass') == passed, 'verification reran after restart'
        assert c.call('context-after-replay', 'context.state') == retained, 'replay overwrote stale assessment'
        assert c.call('stale-again', 'verification.check', {'profile': 'pass', 'id': result['id']})['assessment']['status'] == 'stale'
        pids = []
        for mode in ['cancel', 'disconnect']:
            marker = work / 'marker'
            marker.unlink(missing_ok=True)
            c.call(mode, 'verification.run', params('hang'))
            deadline = time.monotonic() + 5
            while not marker.exists() and time.monotonic() < deadline:
                time.sleep(.01)
            pid = int(marker.read_text())
            pids.append(pid)
            if mode == 'cancel':
                c.call('cancel-command', 'cancel')
            else:
                c.close(); clients.remove(c)
                c = connect('client-three')
            cancelled = completed(c, mode)['verification']
            assert cancelled['assessment']['status'] != 'passed'
            try:
                os.kill(pid, 0)
            except ProcessLookupError:
                pass
            else:
                raise AssertionError('verification process survived completion')
        report.update(status='passed', passed_id=result['id'], failed_id=failed['id'], later_edit_stale=True,
                      replay_after_restart=True, structured_context_restart=True, replay_preserves_stale_context=True, cancel_joined=True, disconnect_joined=True, command_pids=pids, provider_calls=0)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        for client in clients:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

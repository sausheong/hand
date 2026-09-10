#!/usr/bin/env python3
"""Exercise reviewed summariser selection and restart with the actual Hand CLI."""
import argparse
import hashlib
import json
import subprocess
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
    config_dir = out / 'home' / '.hand'
    config_dir.mkdir(parents=True)
    config = {'profiles': {'summary': {'provider': 'local', 'model': 'summary-fixture',
                                     'endpoint': 'http://127.0.0.1:1/v1'}}}
    config_path = config_dir / 'config.json'
    config_path.write_text(json.dumps(config))
    saved_path = out / 'summary.json'
    report = {'status': 'failed', 'qualification': 'development',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest(),
              'model_runs_requested': 0}
    client = None
    try:
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'review')
        client.call('hello', 'hello')
        review = client.call('review', 'summarizer.review', {
            'profile': 'summary', 'max_output_tokens': 512, 'timeout_seconds': 20})
        assert review['destination'] == config['profiles']['summary']['endpoint']
        assert 'session history' in review['disclosure']
        payload = {'options': review['options'], 'digest': review['digest'], 'confirmed': True}
        result = client.call('select', 'summarizer.select', payload)
        assert client.call('select', 'summarizer.select', payload) == result
        assert client.call('status', 'summarizer.status')['independent']
        saved_path.write_text(json.dumps({'version': 1, 'options': review['options'], 'digest': review['digest']}))
        client.close()
        client = None
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'restart',
                        checkpoint_flags=['--summarizer-config', str(saved_path)])
        client.call('hello', 'hello')
        restored = client.call('restored', 'summarizer.status')
        assert restored['independent'] and restored['selection'] == review
        following = client.call('follow-review', 'summarizer.review', {'follow_main': True})
        client.call('follow-select', 'summarizer.select', {
            'options': following['options'], 'digest': following['digest'], 'confirmed': True})
        status = client.call('following', 'summarizer.status')
        assert not status['independent'] and status['selection']['options']['follow_main']
        assert status['selection']['model'] == 'fixture'
        client.close()
        client = None
        config['profiles']['summary']['endpoint'] = 'http://127.0.0.1:2/v1'
        config_path.write_text(json.dumps(config))
        command = [str(binary), '--rpc', '--model', 'local/fixture', '--base-url',
                   'http://127.0.0.1:1/v1', '--summarizer-config', str(saved_path)]
        failed = subprocess.run(command, cwd=work,
                                env={'HOME': str(out / 'home'), 'PATH': '/usr/bin:/bin'},
                                input=b'', capture_output=True, timeout=15)
        (out / 'stale.stdout').write_bytes(failed.stdout)
        (out / 'stale.stderr').write_bytes(failed.stderr)
        assert failed.returncode != 0, 'changed destination accepted at startup'
        assert b'apply reviewed summariser configuration' in failed.stderr
        assert not failed.stdout, 'failed startup emitted protocol output'
        report.update(status='development_passed', restored=restored,
                      following=status, stale_exit_code=failed.returncode, clean_rpc_exits=True)
    finally:
        if client is not None:
            client.close()
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

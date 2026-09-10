#!/usr/bin/env python3
"""Install the packaged tool viewer and verify typed presentation through RPC."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
from check_rpc_client import Client
from check_extension_rpc import poll


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', required=True, type=Path)
    parser.add_argument('--package-dir', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    package = args.package_dir.resolve(strict=True)
    root = args.out.resolve(); root.mkdir(parents=True, exist_ok=False)
    workspace = root / 'workspace'; workspace.mkdir()
    home = root / 'home'; home.mkdir(mode=0o700)
    store, snapshots = root / 'store', root / 'snapshots'
    records = []
    client = None
    report = dict(status='failed', qualification='development', provider_calls=0,
                  binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())

    def command(*words):
        result = subprocess.run([str(binary), 'packages', *map(str, words)], cwd=workspace,
                                env={'HOME': str(home), 'PATH': '/usr/bin:/bin'},
                                capture_output=True, text=True, timeout=30)
        records.append(dict(arguments=list(map(str, words)), exit_code=result.returncode,
                            stdout=result.stdout, stderr=result.stderr))
        (root / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
        assert result.returncode == 0, records[-1]
        return json.loads(result.stdout)

    try:
        inspected = command('inspect', '--source', package)
        pin = inspected['package_digest']
        install = command('review-install', '--source', package, '--pin', pin,
                          '--store', store, '--out', root / 'install.json')
        command('apply', '--review', install['review_file'], '--approve', install['approval_digest'])
        launch = command('review-extensions', '--store', store, '--names', 'tool-viewer',
                         '--workspace', workspace, '--snapshots', snapshots, '--out', root / 'startup.json')
        client = Client(binary, workspace, 'http://127.0.0.1:1/v1', root, 'viewer', checkpoint_flags=[
            '--extension-config', launch['review_file'], '--approve-extension-config', launch['approval_digest']])
        client.call('hello', 'hello')
        supplied = dict(tool='read_file', output='```\n[link](https://example.invalid)\n\u001b[31m', error='supplied error')
        client.call('view', 'extension.command', dict(name='tool-viewer', command='view-tool', arguments=json.dumps(supplied)))
        completed = poll(client, 'view'); assert not completed.get('error'), completed
        blocks = completed['result']['blocks']
        assert [b['kind'] for b in blocks] == ['text', 'list', 'code']
        assert json.loads(blocks[2]['text']) == supplied and '\u001b' not in blocks[2]['text']
        assert 'not an independently verified' in blocks[0]['text']
        client.call('bad', 'extension.command', dict(name='tool-viewer', command='view-tool', arguments='{"outcome":"complete"}'))
        assert poll(client, 'bad').get('error'), 'authority-shaped input accepted'
        client.close(); client = None
        assert not list(snapshots.iterdir())
        report.update(status='passed', package_digest=pin, typed_presentation=True,
                      literal_record_roundtrip=True, invalid_record_rejected=True, snapshots_removed=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if client:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (root / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

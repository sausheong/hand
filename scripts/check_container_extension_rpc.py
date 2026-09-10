#!/usr/bin/env python3
"""Exercise a reviewed native extension through built Hand RPC and container isolation."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
from check_rpc_client import Client
from check_extension_rpc import poll


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ['hand-binary', 'worker', 'extension', 'docker', 'socket', 'out']:
        p.add_argument('--' + name, required=True, type=Path)
    p.add_argument('--image', required=True)
    a = p.parse_args()
    binary, worker, extension, docker, socket = [x.resolve(strict=True) for x in
        [a.hand_binary, a.worker, a.extension, a.docker, a.socket]]
    out = a.out.resolve(); out.mkdir(parents=True, exist_ok=False)
    workspace = out / 'workspace'; workspace.mkdir()
    home = out / 'home'; home.mkdir(mode=0o700)
    configdir = home / '.hand'; configdir.mkdir(mode=0o700)
    env = {'HOME': str(home), 'PATH': '/usr/bin:/bin'}
    docker_config = out / 'docker-config'; docker_config.mkdir(mode=0o700)
    docker_args = [str(docker), '--config', str(docker_config), '--host', 'unix://' + str(socket)]
    records = []
    client = None
    report = dict(status='failed', qualification='development', image=a.image,
                  binary_sha256=sha(binary), worker_sha256=sha(worker), extension_sha256=sha(extension),
                  runner_sha256=sha(Path(__file__)),
                  client_sha256=sha(Path(__file__).with_name('check_rpc_client.py')),
                  poll_runner_sha256=sha(Path(__file__).with_name('check_extension_rpc.py')),
                  provider_calls=0)

    def command(argv):
        r = subprocess.run(list(map(str, argv)), cwd=workspace, env=env,
                           capture_output=True, text=True, timeout=30)
        records.append(dict(argv=list(map(str, argv)), exit_code=r.returncode,
                            stdout=r.stdout, stderr=r.stderr))
        (out / 'commands.json').write_text(json.dumps(records, indent=2) + '\n')
        assert r.returncode == 0, records[-1]
        return r.stdout.strip()

    def containers():
        return set(command(docker_args + ['ps', '-aq', '--filter', 'name=harness-exec-']).split())

    try:
        before = containers()
        boundary = dict(docker=str(docker), socket=str(socket), image=a.image, writable=False, network=False)
        (configdir / 'config.json').write_text(json.dumps(dict(execution=dict(
            boundary, backend='container', worker=str(worker), worker_sha256=sha(worker)))))
        source, review = out / 'input.json', out / 'review.json'
        source.write_text(json.dumps(dict(version=1, snapshot_root=str(out / 'snapshots'), container=boundary,
            extensions=[dict(identity='fixture/container-viewer', launch=dict(name='tool-viewer',
                executable=str(extension), workspace=str(workspace), capabilities=['commands', 'presentation']))])))
        digest = command([binary, '--review-extensions', source, '--extension-review-output', review])
        assert digest == sha(review)
        assert containers() == before, 'review launched a container'
        client = Client(binary, workspace, 'http://127.0.0.1:1/v1', out, 'container', checkpoint_flags=[
            '--extension-config', str(review), '--approve-extension-config', digest])
        client.call('hello', 'hello')
        supplied = dict(tool='fixture', output='container RPC content', error='fixture supplied error')
        client.call('view', 'extension.command', dict(name='tool-viewer', command='view-tool', arguments=json.dumps(supplied)))
        result = poll(client, 'view'); assert not result.get('error'), result
        assert json.loads(result['result']['blocks'][2]['text']) == supplied, result
        active = containers() - before
        assert len(active) == 1, ('expected one extension container', active)
        inspected = json.loads(command(docker_args + ['inspect', next(iter(active))]))[0]
        (out / 'container-inspect.json').write_text(json.dumps(inspected, indent=2) + '\n')
        host = inspected['HostConfig']
        assert host['NetworkMode'] == 'none' and host['ReadonlyRootfs']
        mounts = {m['Destination']: m for m in inspected['Mounts']}
        assert not mounts['/workspace']['RW'] and not mounts['/harness-resources']['RW']
        assert set(mounts) <= {'/workspace', '/harness-resources', '/tmp'}, mounts
        client.close(); client = None
        assert not (containers() - before), 'container survived shutdown'
        report.update(status='passed', typed_presentation=True, inspected_isolation=True, cleanup_joined=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if client:
            try:
                client.close()
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

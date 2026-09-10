"""Exercise a supplied Pi npm runtime with a local deterministic provider only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import time

from evaluation_pi_events import PiRunObserver


def check(runtime, output):
    runtime, output = Path(runtime).resolve(), Path(output).resolve()
    node = shutil.which('node')
    if not node:
        raise ValueError('Node runtime required')
    package = runtime/'node_modules/@earendil-works/pi-coding-agent'
    metadata = json.loads((package/'package.json').read_text())
    if metadata['name'] != '@earendil-works/pi-coding-agent' or metadata['version'] != '0.85.1':
        raise ValueError('expected pinned Pi 0.85.1 runtime')
    cli = package/'dist/bundle/cli.js'
    fixture = Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/pi_offline_provider.ts'
    # Resolve the fixture's SDK import from this isolated npm installation.
    output.mkdir(mode=0o700)
    installed_fixture = runtime/'hand-offline-fixture.ts'
    with installed_fixture.open('xb') as stream:
        stream.write(fixture.read_bytes())
    report = dict(status='running', version=metadata['version'], runtime=str(runtime),
                  cli_sha256=hashlib.sha256(cli.read_bytes()).hexdigest(),
                  fixture_sha256=hashlib.sha256(fixture.read_bytes()).hexdigest(), cases=[],
                  limitations=['Deterministic provider only; not model quality, real billing or spending enforcement.',
                               'No tool-process or platform qualification established by this probe.'])
    try:
        for name, prompt, outcome in [('success', 'FINISH', 'completed'),
                                      ('retry', 'RETRY_ONCE', 'completed'),
                                      ('abort', 'WAIT_FOR_ABORT', 'cancelled')]:
            case = output/name; case.mkdir()
            home, workspace = case/'home', case/'workspace'
            home.mkdir(); workspace.mkdir()
            argv = [node, str(cli), '--mode', 'rpc', '--no-extensions', '-e', str(installed_fixture),
                    '--no-skills', '--no-prompt-templates', '--no-themes', '--no-tools',
                    '--provider', 'hand-offline-fixture', '--model', 'offline-fixture',
                    '--session-dir', str(home/'sessions')]
            env = dict(HOME=str(home), PATH=str(Path(node).parent)+':/usr/bin:/bin',
                       PI_CODING_AGENT_DIR=str(home/'agent'))
            observer = PiRunObserver('prompt-1', 'stats-1', 'hand-offline-fixture', 'offline-fixture')
            started = time.monotonic()
            with (case/'stderr.log').open('wb') as stderr, (case/'stdout.jsonl').open('wb') as stdout:
                process = subprocess.Popen(argv, cwd=workspace, env=env, stdin=subprocess.PIPE,
                                           stdout=subprocess.PIPE, stderr=stderr, start_new_session=True)
                def send(command):
                    process.stdin.write((json.dumps(command)+'\n').encode()); process.stdin.flush()
                try:
                    send(dict(id='prompt-1', type='prompt', message=prompt))
                    stats_sent = aborted = False
                    pending = b''
                    with selectors.DefaultSelector() as selector:
                        selector.register(process.stdout, selectors.EVENT_READ)
                        while observer.stats is None:
                            if time.monotonic()-started > 20:
                                raise TimeoutError('offline Pi case did not settle')
                            if not selector.select(0.1): continue
                            chunk = os.read(process.stdout.fileno(), 65536)
                            if not chunk: raise RuntimeError('Pi closed stdout before final statistics')
                            stdout.write(chunk); stdout.flush(); observer.feed(chunk)
                            pending += chunk
                            while b'\n' in pending:
                                line, pending = pending.split(b'\n', 1)
                                event = json.loads(line)
                                if (name == 'abort' and event.get('type') == 'message_start'
                                        and event.get('message', {}).get('role') == 'assistant' and not aborted):
                                    send(dict(id='abort-1', type='abort')); aborted = True
                            if observer.errors: raise ValueError(observer.errors)
                            if observer.ready_for_stats and not stats_sent:
                                send(dict(id='stats-1', type='get_session_stats')); stats_sent = True
                    process.stdin.close()
                    exit_code = process.wait(timeout=5)
                    remainder = process.stdout.read()
                    if remainder: stdout.write(remainder); observer.feed(remainder)
                    result = observer.finish()
                    if exit_code != 0 or result['status'] != 'observed_unverified' or result['outcome'] != outcome:
                        raise AssertionError(dict(exit_code=exit_code, observation=result))
                    if name == 'retry' and (result['assistant_errors'] != 1 or result['assistant_messages'] != 2):
                        raise AssertionError('retry did not exercise both attempts')
                    report['cases'].append(dict(name=name, argv=argv, exit_code=exit_code,
                        seconds=time.monotonic()-started, observation=result))
                finally:
                    if process.poll() is None:
                        process.kill(); process.wait(timeout=5)
                    process.stdout.close()
                    if not process.stdin.closed: process.stdin.close()
        report['status'] = 'offline_runtime_passed'
    except Exception as error:
        report['status'] = 'failed'
        report['error'] = dict(type=type(error).__name__, message=str(error))
        raise
    finally:
        installed_fixture.unlink()
        report['raw_artifacts'] = [dict(path=str(path.relative_to(output)),
            sha256=hashlib.sha256(path.read_bytes()).hexdigest())
            for path in sorted(output.rglob('*')) if path.is_file() and path.name in ('stderr.log', 'stdout.jsonl')]
        (output/'report.json').write_text(json.dumps(report, indent=2)+'\n')
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(check(args.runtime, args.output)))

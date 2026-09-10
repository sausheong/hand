#!/usr/bin/env python3
"""Kill an actual CLI legacy importer and verify durable partial-import recovery."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    out = args.out.absolute()
    out.mkdir(parents=True, exist_ok=False)
    home, workspace = out/'home', out/'workspace'
    home.mkdir(mode=0o700)
    (workspace/'.hand').mkdir(parents=True)
    settings = json.dumps({'always_allow': ['fixture-tool-'+str(i).zfill(3) for i in range(256)]}).encode()
    (workspace/'.hand/settings.json').write_bytes(settings)
    env = {'HOME': str(home), 'PATH': '/usr/bin:/bin'}
    command = [str(binary), '--permissions', '--model=local/fixture']
    report = dict(status='failed', qualification='development',
                  binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())
    child = None

    def call(label, extra=()):
        result = subprocess.run(command+list(extra), cwd=workspace, env=env,
                                stdin=subprocess.DEVNULL, capture_output=True, timeout=30)
        (out/(label+'.stdout')).write_bytes(result.stdout)
        (out/(label+'.stderr')).write_bytes(result.stderr)
        if result.returncode:
            raise RuntimeError(label+' exited '+str(result.returncode))
        return json.loads(result.stdout)

    try:
        proposal = call('inspect')
        assert not proposal['grants'] and len(proposal['legacy_proposal']) == 256
        fingerprint = proposal['legacy_fingerprint']
        journal, = (home/'.hand/authority').glob('*/*/grants.jsonl')
        extra = ['--ack-legacy-permissions', fingerprint]
        with (out/'interrupted.stdout').open('wb') as stdout, (out/'interrupted.stderr').open('wb') as stderr:
            child = subprocess.Popen(command+extra, cwd=workspace, env=env, stdin=subprocess.DEVNULL,
                                     stdout=stdout, stderr=stderr)
            started = time.monotonic()
            deadline = started+15
            observed = 0
            while time.monotonic() < deadline:
                if child.poll() is not None:
                    raise RuntimeError('importer exited before interruption was delivered')
                observed = journal.read_bytes().count(b'\n')
                if observed:
                    child.send_signal(signal.SIGSTOP)
                    child.kill()
                    break
                time.sleep(.0002)
            else:
                raise RuntimeError('no grant observed before deadline')
            code = child.wait(timeout=5)
            report.update(interrupted_exit_code=code, observed_records_before_signal=observed,
                          interruption_seconds=time.monotonic()-started)
            assert code == -signal.SIGKILL, 'importer was not killed'
        partial = journal.read_bytes()
        (out/'interrupted-grants.jsonl').write_bytes(partial)
        assert partial.endswith(b'\n'), 'interruption left incomplete record; recovery must fail closed'
        records = [json.loads(line) for line in partial.splitlines()]
        assert 0 < len(records) < 256, 'did not interrupt a partial import'
        assert len({r['grant']['ID'] for r in records}) == len(records)
        inspected = call('partial-inspect')
        assert len(inspected['grants']) == len(records)
        assert journal.read_bytes() == partial, 'inspection modified interrupted bytes'
        completed = call('resume', extra)
        expected = {g['ID']:g for g in proposal['legacy_proposal']}
        assert {g['ID']:g for g in completed['grants']} == expected
        recovered = journal.read_bytes()
        assert recovered.startswith(partial), 'resume rewrote durable prefix'
        all_records = [json.loads(line) for line in recovered.splitlines()]
        assert len(all_records) == 256 and {r['grant']['ID'] for r in all_records} == set(expected)
        (out/'recovered-grants.jsonl').write_bytes(recovered)
        call('repeat', extra)
        assert journal.read_bytes() == recovered, 'repeat import appended records'
        assert (workspace/'.hand/settings.json').read_bytes() == settings
        report.update(status='passed', durable_partial_grants=len(records), recovered_grants=256,
                      prefix_preserved=True, repeated_resume_unchanged=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if child is not None and child.poll() is None:
            child.kill()
            child.wait(timeout=5)
        report['artifacts'] = {p.name:hashlib.sha256(p.read_bytes()).hexdigest()
                               for p in out.iterdir() if p.is_file()}
        (out/'run.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','durable_partial_grants','recovered_grants','error'] if k in report}))
    return 0 if report['status']=='passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

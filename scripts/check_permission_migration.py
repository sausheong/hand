#!/usr/bin/env python3
"""Exercise credential-free permission migration, revocation and quarantine recovery."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


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
    settings = workspace/'.hand/settings.json'
    original = b'{"always_allow":["bash","write_file","edit_file"]}\n'
    settings.write_bytes(original)
    report = dict(status='failed', qualification='development', commands=[],
                  binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())

    def call(label, flags=(), failed=False):
        command = [str(binary), '--permissions', '--model=local/fixture', *flags]
        result = subprocess.run(command, cwd=workspace, env={'HOME': str(home), 'PATH': '/usr/bin:/bin'},
                                stdin=subprocess.DEVNULL, capture_output=True, timeout=10)
        (out/(label+'.stdout')).write_bytes(result.stdout)
        (out/(label+'.stderr')).write_bytes(result.stderr)
        report['commands'].append(dict(label=label, command=command, exit_code=result.returncode))
        if failed:
            assert result.returncode != 0 and not result.stdout, 'failed authority exposed usable output'
            return result.stderr.decode()
        assert result.returncode == 0, result.stderr.decode()
        return json.loads(result.stdout)

    try:
        inspection = call('inspect')
        assert not inspection['grants'] and len(inspection['legacy_proposal']) == 3
        fingerprint = inspection['legacy_fingerprint']
        call('reject-wrong-fingerprint', ['--ack-legacy-permissions', 'wrong'], failed=True)
        assert not call('unchanged-after-rejection')['grants']
        imported = call('acknowledge', ['--ack-legacy-permissions', fingerprint])
        assert len(imported['grants']) == 3
        assert call('idempotent-acknowledge', ['--ack-legacy-permissions', fingerprint])['grants'] == imported['grants']
        journals = list((home/'.hand/authority').glob('*/*/grants.jsonl'))
        assert len(journals) == 1
        journal = journals[0]
        complete = journal.read_bytes()
        lines = complete.splitlines(keepends=True)
        assert len(lines) == 3 and all(line.endswith(b'\n') for line in lines)
        (out/'complete-import.jsonl').write_bytes(complete)
        expected = {g['ID']: g for g in imported['grants']}
        report['partial_imports'] = []
        for count in range(1, len(lines)):
            # Authentic records from the actual CLI, cut only at durable record
            # boundaries. This models a partial import; it is not a kill probe.
            prefix = b''.join(lines[:count])
            (out/('prefix-'+str(count)+'.jsonl')).write_bytes(prefix)
            journal.write_bytes(prefix)
            before = call('partial-inspect-'+str(count))
            assert len(before['grants']) == count
            assert before['legacy_fingerprint'] == fingerprint
            assert journal.read_bytes() == prefix, 'inspection rewrote partial import'
            after = call('partial-resume-'+str(count), ['--ack-legacy-permissions', fingerprint])
            assert {g['ID']: g for g in after['grants']} == expected
            recovered = journal.read_bytes()
            assert recovered.startswith(prefix), 'resume rewrote accepted records'
            records = [json.loads(line) for line in recovered.splitlines()]
            assert len(records) == 3
            assert {r['grant']['ID'] for r in records} == set(expected)
            (out/('recovered-'+str(count)+'.jsonl')).write_bytes(recovered)
            call('partial-repeat-'+str(count), ['--ack-legacy-permissions', fingerprint])
            assert journal.read_bytes() == recovered, 'repeated resume appended duplicates'
            report['partial_imports'].append(dict(initial_grants=count, final_grants=3,
                                                  prefix_preserved=True, repeated_resume_unchanged=True))
        for index, grant in enumerate(imported['grants']):
            remaining = call('revoke-'+str(index), ['--revoke-permission', grant['ID']])['grants']
            assert len(remaining) == 2-index
        assert not call('reopen-revoked')['grants']
        corrupt = b'{"version":1,"version":2}\n'
        journal.write_bytes(corrupt)
        diagnostic = call('corrupt', failed=True)
        assert str(journal.parent) in diagnostic and journal.read_bytes() == corrupt
        # All CLI children have exited. Preserve the entire failed authority
        # directory; recreating it must start with no persistent grants.
        quarantine = home/('authority-quarantine-'+journal.parent.name)
        journal.parent.rename(quarantine)
        assert not call('fresh-authority')['grants']
        assert (quarantine/'grants.jsonl').read_bytes() == corrupt
        assert settings.read_bytes() == original
        report.update(status='passed', no_credentials=True, no_model_calls=True,
                      legacy_settings_unchanged=True, quarantine_preserved=True,
                      fresh_authority_has_no_grants=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        report['artifacts'] = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                               for p in out.iterdir() if p.is_file()}
        (out/'run.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','error'] if k in report}))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

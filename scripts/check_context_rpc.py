#!/usr/bin/env python3
"""Check context inspection and explicit skill reload using the built CLI."""
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
    binary = args.hand_binary.resolve(strict=True)
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    work = out / 'workspace'
    work.mkdir()
    (work / 'HAND.md').write_text('Follow the project style guide.\n')
    report = {'status': 'failed', 'qualification': 'development',
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'client_sha256': hashlib.sha256(Path(__file__).with_name('check_rpc_client.py').read_bytes()).hexdigest(),
              'model_runs_requested': 0}
    client = None
    try:
        client = Client(binary, work, 'http://127.0.0.1:1/v1', out, 'context-client')
        client.call('hello', 'hello')
        client.call('invalid', 'context.inspect', {'unknown': True}, expect_error='invalid_params')
        initial = client.call('initial', 'context.inspect')
        assert 'not provider-reported' in initial['estimate_method']
        assert {x['kind'] for x in initial['contributions']} == {'static_prompt', 'messages', 'tool_results', 'tool_schemas'}
        assert initial['estimated_tokens'] > 0 and initial['limitations']
        skill = work / '.agents/skills/new-skill/SKILL.md'
        skill.parent.mkdir(parents=True)
        skill.write_text('---\ndescription: Newly discovered workflow ' + 'descriptive words ' * 40 + '\n---\nProcedure body\n')
        before_reload = client.call('before-reload', 'context.inspect')
        assert before_reload == initial, 'inspection silently refreshed the installed index'
        first_reload = client.call('reload', 'skills.reload')
        refreshed = client.call('refreshed', 'context.inspect')
        assert refreshed['estimated_tokens'] > initial['estimated_tokens']
        skill.unlink()
        replay = client.call('reload', 'skills.reload')
        assert replay == first_reload
        assert client.call('after-replay', 'context.inspect') == refreshed
        client.call('reload-fresh', 'skills.reload')
        removed = client.call('removed', 'context.inspect')
        assert removed == initial, 'removed skill remains in installed context'
        client.close()
        client = None
        report.update(status='development_passed', initial=initial,
                      refreshed=refreshed, removed=removed, clean_exit=True)
    finally:
        if client is not None:
            client.close()
        (out / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

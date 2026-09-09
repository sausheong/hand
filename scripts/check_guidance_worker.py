#!/usr/bin/env python3
"""Exercise a built native worker's instruction and skill protocol, offline."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    evidence = args.evidence.resolve()
    evidence.mkdir(parents=True, exist_ok=False)
    work = evidence / 'workspace'
    home = evidence / 'home'
    home.mkdir()
    files = {
        'HAND.md': 'ROOT_NOT_REPEATED',
        'nested/AGENTS.md': 'NESTED_GUIDANCE',
        'nested/deep/HAND.md': 'NEAREST_GUIDANCE',
        'nested/deep/file.txt': 'SOURCE_CONTENTS',
        'other/HAND.md': 'SIBLING_EXCLUDED',
        '.agents/skills/example/SKILL.md': '---\ndescription: conventional example\n---\nRead references/help.txt',
        '.agents/skills/example/references/help.txt': 'RESOURCE_CONTENTS',
    }
    for name, body in files.items():
        path = work / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)
    records = []

    def invoke(name, tool, params, *, expected_error=False):
        request = {'version': 1, 'tool': tool, 'input': params}
        (evidence / f'{name}.request.json').write_text(json.dumps(request))
        result = subprocess.run([str(binary)], input=json.dumps(request).encode(),
                                cwd=work, env={'HOME': str(home), 'PATH': os.defpath},
                                capture_output=True, timeout=10)
        (evidence / f'{name}.stdout.json').write_bytes(result.stdout)
        (evidence / f'{name}.stderr.txt').write_bytes(result.stderr)
        assert result.returncode == 0, (name, result.returncode, result.stderr)
        payload = json.loads(result.stdout)
        assert payload['version'] == 1, payload
        assert bool(payload['result'].get('error')) == expected_error, payload
        records.append({'name': name, 'exit_code': result.returncode,
                        'stdout_sha256': hashlib.sha256(result.stdout).hexdigest()})
        return payload['result']

    read = invoke('nested', 'read_file', {'path': 'nested/deep/file.txt'})
    text = read['output']
    assert all(x in text for x in ['NESTED_GUIDANCE', 'NEAREST_GUIDANCE', 'SOURCE_CONTENTS'])
    assert 'ROOT_NOT_REPEATED' not in text and 'SIBLING_EXCLUDED' not in text
    assert text.index('NESTED_GUIDANCE') < text.index('NEAREST_GUIDANCE')
    sources = read['metadata']['instruction_sources']
    assert len(sources) == 2
    assert sources[1]['Path'] == str(work / 'nested/deep/HAND.md')
    skill = invoke('skill', 'load_skill', {'name': 'example'})
    assert 'Resource base directory: ' + str(work / '.agents/skills/example') in skill['output']
    resource = invoke('resource', 'read_file', {'path': '.agents/skills/example/references/help.txt'})
    assert 'RESOURCE_CONTENTS' in resource['output']
    (work / 'nested/deep/HAND.md').write_text('UPDATED_GUIDANCE')
    updated = invoke('updated', 'read_file', {'path': 'nested/deep/file.txt'})
    assert 'UPDATED_GUIDANCE' in updated['output'] and 'NEAREST_GUIDANCE' not in updated['output']
    target = work / 'nested/deep/new/child.txt'
    params = {'path': 'nested/deep/new/child.txt', 'content': 'CREATED_CONTENT'}
    refused = invoke('write-before-guidance', 'write_file', params, expected_error=True)
    assert not target.exists() and not target.parent.exists()
    assert 'UPDATED_GUIDANCE' in refused['output']
    digest = refused['metadata']['instruction_digest']
    assert len(digest) == 64
    (work / 'nested/deep/HAND.md').write_text('NEW_MUTATION_GUIDANCE')
    params['instruction_digest'] = digest
    stale = invoke('write-stale-guidance', 'write_file', params, expected_error=True)
    assert not target.exists() and not target.parent.exists()
    assert 'NEW_MUTATION_GUIDANCE' in stale['output']
    assert stale['metadata']['instruction_digest'] != digest
    params['instruction_digest'] = stale['metadata']['instruction_digest']
    invoke('write-current-guidance', 'write_file', params)
    assert target.read_text() == 'CREATED_CONTENT'
    edit = {'path': params['path'], 'old_string': 'CREATED_CONTENT', 'new_string': 'EDITED_CONTENT'}
    edit_refused = invoke('edit-before-guidance', 'edit_file', edit, expected_error=True)
    assert target.read_text() == 'CREATED_CONTENT'
    edit['instruction_digest'] = edit_refused['metadata']['instruction_digest']
    invoke('edit-current-guidance', 'edit_file', edit)
    assert target.read_text() == 'EDITED_CONTENT'
    search = invoke('search-guidance', 'search', {'content': 'EDITED_CONTENT', 'name_glob': '*.txt'})
    assert search['metadata']['complete'] is True
    assert search['metadata']['instruction_guidance_complete'] is True
    assert 'NEW_MUTATION_GUIDANCE' in search['output'] and 'NESTED_GUIDANCE' in search['output']
    assert 'SIBLING_EXCLUDED' not in search['output']
    (work / 'nested/deep/HAND.md').write_text('LONG_GUIDANCE ' * 400)
    bounded = invoke('search-bounded-guidance', 'search', {'content': 'EDITED_CONTENT', 'name_glob': '*.txt', 'max_bytes': 2048})
    assert len(bounded['output'].encode()) <= 2048
    assert bounded['metadata']['instruction_guidance_complete'] is False
    assert 'guidance was omitted' in bounded['output']
    assert 'LONG_GUIDANCE' not in bounded['output']
    # A digest acknowledges guidance only; workspace validation still applies.
    outside = evidence / 'outside'
    outside.mkdir()
    (outside / 'HAND.md').write_text('OUTSIDE_SECRET')
    (work / 'escape').symlink_to(outside, target_is_directory=True)
    denied = invoke('outside-denied', 'write_file', {'path': 'escape/new/file', 'content': 'FORBIDDEN', 'instruction_digest': digest}, expected_error=True)
    assert not (outside / 'new').exists() and 'OUTSIDE_SECRET' not in denied.get('output', '')
    report = {'status': 'development_passed', 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
              'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'provider_calls': 0, 'boundary': 'native worker fixture, no container qualification', 'invocations': records}
    (evidence / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))


if __name__ == '__main__':
    main()

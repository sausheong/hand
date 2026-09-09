"""Verify evaluation source pins using explicitly mapped local Git repositories."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from evaluation_manifest import validate

LANGUAGES = {'.go': 'Go', '.py': 'Python', '.js': 'JavaScript', '.jsx': 'JavaScript',
             '.ts': 'TypeScript', '.tsx': 'TypeScript', '.rs': 'Rust', '.java': 'Java',
             '.c': 'C', '.cpp': 'C++', '.rb': 'Ruby', '.swift': 'Swift', '.sh': 'Shell'}


def inventory(repository, commit):
    if not isinstance(commit, str) or not re.fullmatch('[0-9a-f]{40}', commit):
        raise ValueError('source requires a full lowercase Git commit')
    repository = Path(repository).resolve(strict=True)
    environment = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                       GIT_NO_REPLACE_OBJECTS='1', GIT_TERMINAL_PROMPT='0', GIT_NO_LAZY_FETCH='1')
    # Git environment overrides must not redirect an explicitly mapped source.
    for key in ['GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_OBJECT_DIRECTORY',
                'GIT_ALTERNATE_OBJECT_DIRECTORIES', 'GIT_CONFIG_COUNT', 'GIT_CONFIG_PARAMETERS']:
        environment.pop(key, None)
    def git(*args):
        return subprocess.check_output(['git', '--no-pager', '--no-optional-locks', '-C', str(repository),
            '-c', 'core.fsmonitor=false', '-c', 'protocol.allow=never',
            '-c', 'protocol.file.allow=never', *args], env=environment, timeout=60, stderr=subprocess.PIPE)
    if git('cat-file', '-t', commit).strip() != b'commit':
        raise ValueError('source pin does not identify a commit')
    raw = git('ls-tree', '-r', '-l', '-z', '--full-tree', commit)
    entries, languages, special = [], {}, []
    total = 0
    for item in raw.split(b'\0'):
        if not item: continue
        metadata, name = item.split(b'\t', 1)
        mode, kind, object_id, size = metadata.decode().split()
        name = name.decode('utf-8')
        length = int(size) if size != '-' else None
        entry = dict(path=name, mode=mode, kind=kind, object_id=object_id, bytes=length)
        entries.append(entry)
        if mode not in ('100644', '100755') or kind != 'blob':
            special.append(entry)
            continue
        total += length
        language = LANGUAGES.get(Path(name).suffix.lower())
        if language:
            metric = languages.setdefault(language, dict(files=0, bytes=0))
            metric['files'] += 1; metric['bytes'] += length
    return dict(commit=commit, tree=git('rev-parse', commit+'^{tree}').decode().strip(),
                local_repository=str(repository), raw_inventory_sha256=hashlib.sha256(raw).hexdigest(),
                regular_files=len(entries)-len(special), regular_bytes=total,
                language_extensions=languages, special_entries=special, entries=entries)


def verify_sources(manifest, manifest_root, repositories):
    validation = validate(manifest, manifest_root)
    if not isinstance(repositories, dict) or any(not isinstance(k, str) or not isinstance(v, str) or not Path(v).is_absolute()
                                               for k, v in repositories.items()):
        raise ValueError('repository mappings require declared URLs and absolute local paths')
    sources, missing, assignments = {}, [], []
    for task in manifest['tasks']:
        url, commit = task['repository']['url'], task['repository']['commit']
        key = url+'@'+commit
        assignments.append(dict(task=task['id'], source=key, split=task['split'], category=task['category']))
        if key in sources or key in missing: continue
        if url not in repositories:
            missing.append(key)
            continue
        sources[key] = inventory(repositories[url], commit)
    return dict(schema_version=1, status='local_sources_verified' if not missing else 'incomplete',
                catalogue_status=validation['status'], tasks=assignments, sources=sources, missing_sources=missing,
                distinct_repositories=len({t['repository']['url'] for t in manifest['tasks']}),
                limitations=['No network fetch, checkout, hook, build, test or model run was performed.',
                    'Explicit local mappings prove availability of commit/tree objects, not remote origin ownership.',
                    'File extensions and byte counts describe the entire committed tree, including tests/vendor/generated files; they are not lines of authored code or proof of task diversity.',
                    'Symlinks and submodules are listed separately; their targets were not followed or fetched.',
                    'The working tree is ignored. This does not qualify candidate changes or verify task oracles.'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('repositories', type=Path)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    try:
        manifest_raw, mappings_raw = args.manifest.read_bytes(), args.repositories.read_bytes()
        result = verify_sources(json.loads(manifest_raw), args.manifest.resolve().parent, json.loads(mappings_raw))
        result['inputs'] = dict(manifest_sha256=hashlib.sha256(manifest_raw).hexdigest(),
                               mappings_sha256=hashlib.sha256(mappings_raw).hexdigest(),
                               runner_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())
        with args.out.open('x') as output: output.write(json.dumps(result, indent=2)+'\n')
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(json.dumps(dict(status='invalid', error=str(error))), file=sys.stderr)
        return 2
    print(json.dumps(dict(status=result['status'], sources=len(result['sources']), missing=len(result['missing_sources']))))
    return 1 if result['missing_sources'] else 0


if __name__ == '__main__': raise SystemExit(main())

"""Prepare an independent, exact-blob Git checkout without executing repository code."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

from evaluation_sources import inventory
from evaluation_workspace import parts

MAX_SOURCE_BYTES = 1 << 30
MAX_BLOB_BYTES = 64 << 20


def git(repository, *args, file_transport=False):
    # Do not inherit Git redirects, templates, helpers, config injection or user
    # settings. Only the explicit local fetch may use any transport.
    environment = dict(PATH=os.environ.get('PATH', os.defpath), LANG='C',
                       GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                       GIT_NO_REPLACE_OBJECTS='1', GIT_TERMINAL_PROMPT='0',
                       GIT_NO_LAZY_FETCH='1')
    return subprocess.check_output([
        'git', '--no-pager', '--no-optional-locks', '-C', str(repository),
        '-c', 'core.fsmonitor=false', '-c', 'core.hooksPath=/dev/null',
        '-c', 'protocol.allow=never', '-c', 'protocol.file.allow='+('always' if file_transport else 'never'),
        *args], env=environment, stderr=subprocess.PIPE, timeout=120)


def prepare_checkout(repository, commit, destination):
    """Destination must not exist; discard it after any failure.

    Symlinks and submodules currently fail preparation rather than being omitted
    or followed. The caller must exclusively own the destination's parent.
    """
    source = inventory(repository, commit)
    if source['special_entries']:
        raise ValueError('source contains symlinks or submodules requiring explicit preparation support')
    if source['regular_bytes'] > MAX_SOURCE_BYTES:
        raise ValueError('source exceeds checkout byte limit')
    for entry in source['entries']:
        parts(entry['path'])
        if entry['bytes'] > MAX_BLOB_BYTES:
            raise ValueError('source blob exceeds checkout byte limit')
    destination = Path(destination)
    if not destination.is_absolute() or destination != destination.resolve():
        raise ValueError('destination must be absolute without symlink components')
    destination.mkdir(mode=0o700)
    git(destination, 'init', '--template=', '--quiet')
    git(destination, 'fetch', '--quiet', '--no-tags', '--no-recurse-submodules',
        '--no-write-fetch-head', '--', source['local_repository'], commit, file_transport=True)
    if git(destination, 'cat-file', '-t', commit).strip() != b'commit':
        raise ValueError('fetched object is not the pinned commit')
    tree = git(destination, 'rev-parse', commit+'^{tree}').decode().strip()
    if tree != source['tree']:
        raise ValueError('fetched tree differs from source inventory')
    # Populate index/HEAD without checkout filters, hooks, autocrlf, export-ignore
    # or working-tree-encoding rewriting the task's committed input bytes.
    git(destination, 'read-tree', commit)
    git(destination, 'update-ref', '--no-deref', 'HEAD', commit)
    files = []
    for entry in source['entries']:
        payload = git(destination, 'cat-file', 'blob', entry['object_id'])
        if len(payload) != entry['bytes'] or len(payload) > MAX_BLOB_BYTES:
            raise ValueError('blob size differs from pinned inventory')
        # Verify Git's object identity independently of the transport.
        object_id = hashlib.sha1(b'blob '+str(len(payload)).encode()+b'\0'+payload).hexdigest()
        if object_id != entry['object_id']:
            raise ValueError('blob object identity mismatch')
        target = destination/entry['path']
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with target.open('xb') as stream:
            stream.write(payload)
        target.chmod(0o755 if entry['mode'] == '100755' else 0o644)
        files.append(dict(path=entry['path'], mode=entry['mode'], bytes=len(payload),
                          sha256=hashlib.sha256(payload).hexdigest()))
    if (destination/'.git/objects/info/alternates').exists():
        raise ValueError('checkout unexpectedly depends on alternate object storage')
    # read-tree populated object identities before files existed. Refresh the
    # stat cache so operations requiring an index/worktree match can use this
    # checkout immediately, without a preceding status command.
    git(destination, 'update-index', '--refresh')
    return dict(schema_version=1, status='source_checkout_prepared', commit=commit, tree=tree,
                destination=str(destination), local_repository=source['local_repository'],
                raw_inventory_sha256=source['raw_inventory_sha256'], files=files,
                limitations=['No build, test, agent or model execution occurred.',
                    'The local mapping does not establish ownership of a remote repository.',
                    'Caller must exclusively own the preparation directory and discard it after any error.',
                    'Committed attributes are retained but no worktree transformations are applied.'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', type=Path, required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--destination', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare_checkout(args.repository, args.commit, args.destination)))


if __name__ == '__main__': main()

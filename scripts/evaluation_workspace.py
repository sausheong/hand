"""Install pinned verification fixtures into an idle, independently owned checkout."""
import argparse
from contextlib import ExitStack
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat

MAX_FILE_BYTES = 16 << 20
MAX_TOTAL_BYTES = 64 << 20


def parts(name):
    if not isinstance(name, str) or not name or '\\' in name or '\x00' in name:
        raise ValueError('invalid fixture path')
    path = PurePosixPath(name)
    if (path.is_absolute() or path.as_posix() != name or name == '.'
            or '..' in path.parts or any(p.lower() == '.git' for p in path.parts)):
        raise ValueError('fixture path must be canonical and outside Git metadata')
    return path.parts


def directory(stack, name, parent=None):
    fd = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=parent)
    stack.callback(os.close, fd)
    return fd


def root_directory(stack, root):
    root = Path(root)
    if not root.is_absolute():
        raise ValueError('root must be absolute')
    # Open every component, including ancestors of the supplied root, without
    # following links. Resolve OS aliases such as /tmp explicitly at the caller.
    fd = directory(stack, '/')
    if '..' in root.parts:
        raise ValueError('root must not contain traversal')
    for component in root.parts[1:]:
        fd = directory(stack, component, fd)
    return fd


def parent_directory(stack, root, components, create=False):
    fd = root
    for component in components[:-1]:
        if create:
            try:
                os.mkdir(component, 0o700, dir_fd=fd)
            except FileExistsError:
                pass
        fd = directory(stack, component, fd)
    return fd


def install_fixtures(source_root, workspace, fixtures):
    """No shell, model calls or overwrite. Discard the fresh checkout on failure.

    Caller must own an idle checkout: no agent or other writer may rename its
    directories during installation. Descriptor-relative operations reject
    symlink substitution, but are not a sandbox against a concurrent host owner.
    An I/O failure can leave partial fixtures; it never returns a success record.
    """
    if not isinstance(fixtures, list) or len(fixtures) > 128:
        raise ValueError('expected at most 128 fixtures')
    prepared, destinations, total = [], set(), 0
    with ExitStack() as roots:
        source_fd = root_directory(roots, source_root)
        workspace_fd = root_directory(roots, workspace)
        for fixture in fixtures:
            if not isinstance(fixture, dict) or set(fixture) != {'source', 'destination', 'sha256'}:
                raise ValueError('invalid fixture record')
            source, target = parts(fixture['source']), parts(fixture['destination'])
            if any(target[:len(old)] == old or old[:len(target)] == target for old in destinations):
                raise ValueError('duplicate or overlapping fixture destinations')
            destinations.add(target)
            digest = fixture['sha256']
            if not isinstance(digest, str) or not re.fullmatch('[0-9a-f]{64}', digest):
                raise ValueError('invalid fixture digest')
            with ExitStack() as opened:
                parent = parent_directory(opened, source_fd, source)
                fd = os.open(source[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
                opened.callback(os.close, fd)
                info = os.fstat(fd)
                if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_FILE_BYTES:
                    raise ValueError('fixture must be a bounded regular file')
                with os.fdopen(os.dup(fd), 'rb') as stream:
                    payload = stream.read(MAX_FILE_BYTES + 1)
                total += len(payload)
                if len(payload) > MAX_FILE_BYTES or total > MAX_TOTAL_BYTES:
                    raise ValueError('fixture byte limit exceeded')
                if hashlib.sha256(payload).hexdigest() != digest:
                    raise ValueError('fixture hash mismatch')
            prepared.append((fixture, target, payload))
        # Preflight all existing paths before creating even the first file.
        for _, target, _ in prepared:
            with ExitStack() as opened:
                try:
                    parent = parent_directory(opened, workspace_fd, target)
                except FileNotFoundError:
                    continue
                try:
                    os.stat(target[-1], dir_fd=parent, follow_symlinks=False)
                except FileNotFoundError:
                    continue
                raise FileExistsError('fixture destination already exists: ' + '/'.join(target))
        installed = []
        for fixture, target, payload in prepared:
            with ExitStack() as opened:
                parent = parent_directory(opened, workspace_fd, target, create=True)
                fd = os.open(target[-1], os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                             0o600, dir_fd=parent)
                with os.fdopen(fd, 'wb') as stream:
                    stream.write(payload)
                    stream.flush()
                    os.fsync(stream.fileno())
                os.fsync(parent)
            installed.append(dict(fixture, bytes=len(payload)))
    return dict(status='fixtures_installed', fixtures=installed, total_bytes=total)


def verify_fixtures(workspace, fixtures):
    """Observe pinned bytes after a joined command, without following links.

    This detects persistent mutation only. It cannot detect a write restored
    before observation or isolate hostile code running as the same host user.
    """
    errors, observed = [], []
    for fixture in fixtures:
        target = parts(fixture['destination'])
        try:
            with ExitStack() as opened:
                root = root_directory(opened, workspace)
                parent = parent_directory(opened, root, target)
                fd = os.open(target[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
                opened.callback(os.close, fd)
                info = os.fstat(fd)
                if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_FILE_BYTES:
                    raise ValueError('fixture is no longer a bounded regular file')
                with os.fdopen(os.dup(fd), 'rb') as stream:
                    data = stream.read(MAX_FILE_BYTES + 1)
                digest = hashlib.sha256(data).hexdigest()
                observed.append(dict(destination=fixture['destination'], sha256=digest, bytes=len(data)))
                if len(data) > MAX_FILE_BYTES or digest != fixture['sha256']:
                    raise ValueError('fixture bytes changed')
        except (OSError, ValueError) as error:
            errors.append(dict(destination=fixture['destination'], error=str(error), type=type(error).__name__))
    return dict(status='failed' if errors else 'passed', observed=observed, errors=errors,
                limitation='Post-command observation cannot detect transient restored changes or replace isolation and review.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('--task', required=True)
    parser.add_argument('--workspace', type=Path, required=True)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_bytes())
    if type(manifest.get('schema_version')) is not int or manifest['schema_version'] != 1:
        parser.error('unsupported manifest version')
    tasks = [task for task in manifest['tasks'] if task['id'] == args.task]
    if len(tasks) != 1:
        parser.error('task must identify exactly one manifest entry')
    report = install_fixtures(args.manifest.absolute().parent, args.workspace, tasks[0]['fixtures'])
    report['task'] = args.task
    print(json.dumps(report))


if __name__ == '__main__':
    main()

"""Source fingerprints and numeric coverage derived from raw Go profiles."""
import hashlib
import json
from pathlib import Path
import subprocess


def source_snapshot(root):
    names = subprocess.check_output(['git', '-C', str(root), 'ls-files', '-z', '--cached',
                                     '--others', '--exclude-standard']).split(b'\0')
    entries = []
    for name in sorted(set(n for n in names if n)):
        relative = name.decode('utf-8')
        path = root / relative
        if path.is_symlink():
            import os
            payload = os.readlink(path).encode()
            mode = 'symlink'
        elif path.is_file():
            payload = path.read_bytes()
            mode = oct(path.stat().st_mode & 0o777)
        else:
            payload, mode = b'', 'deleted'
        entries.append(dict(path=relative, mode=mode, sha256=hashlib.sha256(payload).hexdigest()))
    encoded = json.dumps(entries, sort_keys=True, separators=(',', ':')).encode()
    return dict(sha256=hashlib.sha256(encoded).hexdigest(), entries=entries)


def coverage_metrics(text):
    lines = text.splitlines()
    if not lines or lines[0] != 'mode: atomic':
        raise ValueError('expected atomic Go coverage')
    blocks = {}
    for line in lines[1:]:
        location, statements, count = line.rsplit(' ', 2)
        statements, count = int(statements), int(count)
        if statements < 0 or count < 0:
            raise ValueError('negative coverage counts')
        # Multiple test binaries can cover the same production block.
        if location in blocks and blocks[location][0] != statements:
            raise ValueError('conflicting coverage block size')
        blocks[location] = (statements, max(count, blocks.get(location, (0, 0))[1]))
    packages = {}
    for location, (statements, count) in blocks.items():
        filename = location.rsplit(':', 1)[0]
        package = filename.rsplit('/', 1)[0]
        entry = packages.setdefault(package, dict(statements=0, covered=0))
        entry['statements'] += statements
        entry['covered'] += statements if count > 0 else 0
    def summarise(total, covered):
        return dict(statements=total, covered=covered, percentage=100 * covered / total if total else None)
    return dict(total=summarise(sum(p['statements'] for p in packages.values()),
                               sum(p['covered'] for p in packages.values())),
                packages={k: summarise(v['statements'], v['covered']) for k, v in packages.items()})

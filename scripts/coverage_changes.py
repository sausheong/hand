"""Conservative changed-statement coverage from a Go profile and actual Git hunks.

A whole instrumented block counts when it intersects a changed source line.
Missing profile files remain gaps unless Go's instrumenter proves zero counters.
Zero-counter files add neither covered nor total statements.
"""
import re
import hashlib
import subprocess
import tempfile
from pathlib import Path


def classify_changed_sources(root, changes, scope):
    """Exclude only enumerated, fingerprinted test drivers; retain all other gaps.

    This is deliberately not a glob-based coverage ignore list. Platform code,
    examples and ordinary production paths cannot be excluded by this policy.
    """
    if scope.get('schema_version') != 1 or set(scope) != {'schema_version', 'test_fixtures'}:
        raise ValueError('invalid coverage source scope')
    if not isinstance(scope['test_fixtures'], list):
        raise ValueError('test fixtures must be an enumerated list')
    root = Path(root).resolve()
    excluded = {}
    for entry in scope['test_fixtures']:
        if not isinstance(entry, dict) or set(entry) != {'path', 'sha256', 'reason'}:
            raise ValueError('invalid test fixture classification')
        name, digest, reason = entry['path'], entry['sha256'], entry['reason']
        if not isinstance(name, str) or not name.endswith('.go') or '\\' in name:
            raise ValueError('fixture requires an exact Go source path')
        path = Path(name)
        if path.is_absolute() or '..' in path.parts or str(path) != name or name in excluded:
            raise ValueError('unsafe or duplicate fixture path')
        if 'testdata' not in path.parts and not name.startswith('docs/acceptance/development-evidence/'):
            raise ValueError('ordinary source cannot be classified as a test fixture')
        if not isinstance(reason, str) or not reason.strip() or not isinstance(digest, str) or not re.fullmatch('[0-9a-f]{64}', digest):
            raise ValueError('fixture requires a reason and lowercase SHA-256')
        source = root / path
        if source.resolve() != source or not source.is_file():
            raise ValueError('fixture source must exist without symlink components')
        if hashlib.sha256(source.read_bytes()).hexdigest() != digest:
            raise ValueError('test fixture changed since classification: ' + name)
        excluded[name] = entry
    return dict(production_changes={k: v for k, v in changes.items() if k not in excluded},
                excluded_tests=[dict(excluded[k], changed_lines=changes[k])
                                for k in sorted(set(changes) & excluded.keys())])


def changed_lines(root, baseline):
    def git(*args):
        return subprocess.check_output(['git', '-C', str(root), *args])
    # Verify the baseline rather than silently treating a typo as an empty diff.
    git('rev-parse', '--verify', baseline + '^{commit}')
    tracked = git('diff', '--no-renames', '--name-only', '-z', baseline, '--', '*.go')
    untracked = git('ls-files', '--others', '--exclude-standard', '-z', '--', '*.go')
    new = {n.decode() for n in untracked.split(b'\0') if n}
    result = {}
    for name in sorted({n.decode() for n in tracked.split(b'\0') if n} | new):
        path = Path(root) / name
        if name.endswith('_test.go') or not path.is_file():
            continue
        if name in new:
            intervals = [(1, len(path.read_text().splitlines()))]
        else:
            diff = git('diff', '--no-ext-diff', '--no-renames', '--unified=0', baseline,
                       '--', name).decode()
            intervals = []
            for start, length in re.findall(r'^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@', diff, re.M):
                start, length = int(start), int(length) if length else 1
                if length:
                    intervals.append((start, start + length - 1))
        if intervals:
            result[name] = intervals
    return result


def declaration_only_source(root, name):
    """Require Go's own instrumenter to prove a missing file has no counters.

    This is measurement of zero executable coverage blocks, not a scope waiver.
    Any parse/tool/path failure leaves the source as missing evidence.
    """
    root = Path(root).resolve()
    source = root / name
    if Path(name).is_absolute() or '..' in Path(name).parts or source.resolve() != source:
        return None
    try:
        before = source.read_bytes()
        with tempfile.TemporaryDirectory(prefix='hand-coverage-') as directory:
            output = Path(directory) / 'instrumented.go'
            command = ['go', 'tool', 'cover', '-mode=atomic', '-var=HandChangedCoverage',
                       '-o', str(output), str(source)]
            result = subprocess.run(command, capture_output=True, timeout=30)
            if result.returncode:
                return None
            instrumented = output.read_text()
        if source.read_bytes() != before:
            return None
        # The instrumenter appends this declaration after the source. Take the
        # final occurrence so a string/comment in input cannot impersonate it.
        generated = instrumented.rsplit('\nvar HandChangedCoverage = struct {', 1)
        if len(generated) != 2 or not re.match(
                r'\s*Count\s+\[0\]uint32\s+Pos\s+\[3 \* 0\]uint32\s+NumStmt\s+\[0\]uint16\s*}', generated[1]):
            return None
        return dict(path=name, sha256=hashlib.sha256(before).hexdigest(),
                    instrumented_sha256=hashlib.sha256(instrumented.encode()).hexdigest(),
                    counters=0, method='go tool cover -mode=atomic')
    except (OSError, UnicodeError, subprocess.TimeoutExpired):
        return None


def changed_coverage(profile, module, changes, root=None):
    lines = profile.splitlines()
    if not lines or lines[0] != 'mode: atomic':
        raise ValueError('expected atomic coverage profile')
    blocks, seen_files = {}, set()
    for line in lines[1:]:
        location, statements, count = line.rsplit(' ', 2)
        statements, count = int(statements), int(count)
        if min(statements, count) < 0:
            raise ValueError('negative coverage count')
        filename, coordinates = location.rsplit(':', 1)
        if not filename.startswith(module + '/'):
            raise ValueError('profile contains a different module: ' + filename)
        filename = filename[len(module) + 1:]
        match = re.fullmatch(r'(\d+)\.\d+,(\d+)\.\d+', coordinates)
        if not match:
            raise ValueError('invalid block coordinates')
        first, last = map(int, match.groups())
        if first < 1 or last < first:
            raise ValueError('invalid block interval')
        seen_files.add(filename)
        if not any(first <= end and last >= start for start, end in changes.get(filename, [])):
            continue
        if location in blocks and blocks[location]['statements'] != statements:
            raise ValueError('conflicting block statement count')
        old_count = blocks.get(location, {}).get('count', 0)
        blocks[location] = dict(statements=statements, count=max(count, old_count))
    total = sum(b['statements'] for b in blocks.values())
    covered = sum(b['statements'] for b in blocks.values() if b['count'])
    missing = sorted(set(changes) - seen_files)
    zero_counter_sources = []
    if root is not None:
        for name in missing[:]:
            proof = declaration_only_source(root, name)
            if proof is not None:
                zero_counter_sources.append(proof)
                missing.remove(name)
    return dict(status='missing_profile_files' if missing else ('measured' if total else 'no_applicable_statements'),
                statements=total, covered=covered, percentage=100 * covered / total if total else None,
                missing_profile_files=missing, changed_lines=changes, blocks=blocks,
                zero_counter_sources=zero_counter_sources,
                method='entire instrumented block intersecting any added or changed line')

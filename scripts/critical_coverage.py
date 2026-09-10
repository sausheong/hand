"""Measure mapped critical code without turning absent evidence into a pass."""
import fnmatch
import re
from coverage_changes import declaration_only_source

GROUPS = {'lifecycle', 'permissions', 'persistence', 'execution', 'checkpoints',
          'budgets', 'protocol', 'extensions', 'accounting'}


def critical_coverage(profiles, mapping, roots=None):
    if mapping.get('schema_version') != 1 or set(mapping.get('groups', {})) != GROUPS:
        raise ValueError('mapping must contain exactly the nine required critical groups')
    modules = {}
    for module, text in profiles.items():
        lines = text.splitlines()
        if not lines or lines[0] != 'mode: atomic':
            raise ValueError('expected atomic coverage profile')
        blocks = {}
        for line in lines[1:]:
            location, statements, count = line.rsplit(' ', 2)
            statements, count = int(statements), int(count)
            filename, coordinates = location.rsplit(':', 1)
            coordinate_match = re.fullmatch(r'(\d+)\.\d+,(\d+)\.\d+', coordinates)
            if not coordinate_match or int(coordinate_match[1]) < 1 or int(coordinate_match[2]) < int(coordinate_match[1]):
                raise ValueError('invalid coverage coordinates')
            if not filename.startswith(module + '/') or min(statements, count) < 0:
                raise ValueError('invalid module or coverage counts')
            if location in blocks and blocks[location]['statements'] != statements:
                raise ValueError('conflicting coverage block sizes')
            blocks[location] = dict(file=filename[len(module)+1:], statements=statements,
                                    count=max(count, blocks.get(location, {}).get('count', 0)))
        modules[module] = blocks
    result = {}
    zero_cache = {}
    for name, group in mapping['groups'].items():
        selected, missing_profiles, unmatched = {}, [], []
        zero_sources = {}
        sources = group.get('sources', {})
        pending = group.get('pending_capabilities', [])
        if not isinstance(pending, list) or any(not isinstance(p, str) or not p for p in pending):
            raise ValueError('pending capabilities must be explicit descriptions')
        for module, patterns in sources.items():
            if not isinstance(patterns, list) or not patterns:
                raise ValueError('source mapping requires nonempty path patterns')
            if module not in modules:
                missing_profiles.append(module)
                continue
            for pattern in patterns:
                if not isinstance(pattern, str) or not pattern or pattern.startswith('/') or '..' in pattern.split('/'):
                    raise ValueError('source patterns must be module-relative')
                matched = {loc: b for loc, b in modules[module].items()
                           if fnmatch.fnmatchcase(b['file'], pattern)}
                if not matched:
                    key = module + '/' + pattern
                    # Only an exact file can be proven empty. A glob may hide
                    # missing executable files, even if some matches are empty.
                    proof = None
                    if roots and module in roots and not any(c in pattern for c in '*?['):
                        if key not in zero_cache:
                            zero_cache[key] = declaration_only_source(roots[module], pattern)
                        proof = zero_cache[key]
                    if proof:
                        zero_sources[key] = dict(proof, module=module)
                    else:
                        unmatched.append(key)
                selected.update(matched)
        total = sum(b['statements'] for b in selected.values())
        covered = sum(b['statements'] for b in selected.values() if b['count'])
        observed = 100 * covered / total if total else None
        complete = bool(sources) and not (missing_profiles or unmatched or pending) and total > 0
        result[name] = dict(status='measured' if complete else 'incomplete',
                            percentage=observed if complete else None,
                            observed_percentage=observed, statements=total, covered=covered,
                            missing_profiles=sorted(missing_profiles), unmatched_patterns=sorted(unmatched),
                            pending_capabilities=pending, blocks=selected,
                            zero_counter_sources=list(zero_sources.values()))
    return result


def main():
    import argparse
    import hashlib
    import json
    from pathlib import Path
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mapping', type=Path, required=True)
    parser.add_argument('--profile', action='append', required=True, help='module=atomic-profile-path; repeat per module')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--source-root', action='append', default=[],
                        help='module=source-root for exact zero-counter proofs; repeat per module')
    args = parser.parse_args()
    profiles, artifacts = {}, []
    for item in args.profile:
        module, separator, name = item.partition('=')
        if not separator or not module or module in profiles:
            parser.error('each profile needs a unique module=path')
        path = Path(name).resolve()
        payload = path.read_bytes()
        profiles[module] = payload.decode()
        artifacts.append(dict(module=module, path=str(path), sha256=hashlib.sha256(payload).hexdigest()))
    roots = {}
    for item in args.source_root:
        module, separator, name = item.partition('=')
        if not separator or not name or module not in profiles or module in roots:
            parser.error('each source root needs a unique profiled module=path')
        roots[module] = Path(name).resolve()
    raw_mapping = args.mapping.read_bytes()
    report = dict(groups=critical_coverage(profiles, json.loads(raw_mapping), roots=roots),
                  profiles=artifacts, mapping_sha256=hashlib.sha256(raw_mapping).hexdigest(),
                  limitation='Hashes identify inputs; candidate identity and test provenance require separate verification.')
    args.output.write_text(json.dumps(report, indent=2)+'\n')


if __name__ == '__main__':
    main()

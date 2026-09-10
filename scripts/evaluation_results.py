"""Summarise pinned evaluation records; never execute, authorise or certify runs."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import random
import statistics
import sys
from evaluation_schedule import schedule


def number(value, name, nullable=False, integer=False):
    if value is None and nullable:
        return
    if isinstance(value, bool) or not isinstance(value, int if integer else (int, float)) or not math.isfinite(value) or value < 0:
        raise ValueError('invalid nonnegative measurement: ' + name)


def verify_record(record, evidence_root):
    fields = {'run_id', 'outcome', 'tests', 'review', 'regressions', 'cost_usd',
              'elapsed_seconds', 'interventions', 'recovery_attempted', 'recovery_succeeded', 'evidence'}
    if not isinstance(record, dict) or set(record) != fields:
        raise ValueError('run record fields do not match schema')
    if record['outcome'] not in {'completed', 'failed', 'cancelled', 'timed_out'}:
        raise ValueError('invalid run outcome')
    for key in ['tests', 'review']:
        if record[key] not in {'passed', 'failed', 'skipped', 'missing'}:
            raise ValueError('invalid verification outcome')
    for key in ['regressions', 'interventions']:
        number(record[key], key, integer=True)
    number(record['cost_usd'], 'cost_usd', nullable=True)
    number(record['elapsed_seconds'], 'elapsed_seconds')
    if type(record['recovery_attempted']) is not bool or record['recovery_succeeded'] not in (None, True, False):
        raise ValueError('invalid recovery measurement')
    if record['recovery_succeeded'] is not None and type(record['recovery_succeeded']) is not bool:
        raise ValueError('recovery success must be boolean or null')
    if not record['recovery_attempted'] and record['recovery_succeeded'] is not None:
        raise ValueError('recovery success without a recovery attempt')
    artifacts = record['evidence']
    if not isinstance(artifacts, list) or not 1 <= len(artifacts) <= 64:
        raise ValueError('each record needs 1-64 evidence artifacts')
    seen = set()
    root = Path(evidence_root).resolve()
    for artifact in artifacts:
        if not isinstance(artifact, dict) or set(artifact) != {'path', 'sha256'}:
            raise ValueError('invalid artifact record')
        name = artifact['path']
        if not isinstance(name, str) or not name or '\\' in name:
            raise ValueError('invalid artifact path')
        relative = Path(name)
        if relative.is_absolute() or '..' in relative.parts or str(relative) != name or name in seen:
            raise ValueError('unsafe or duplicate artifact path')
        seen.add(name)
        path = root / relative
        if path.resolve() != path or not path.is_file():
            raise ValueError('artifact must be a regular file without symlink components')
        hasher = hashlib.sha256()
        with path.open('rb') as source:
            for block in iter(lambda: source.read(1 << 20), b''):
                hasher.update(block)
        if hasher.hexdigest() != artifact['sha256']:
            raise ValueError('evidence digest mismatch: ' + name)


def success(record):
    return (record['outcome'] == 'completed' and record['tests'] == 'passed'
            and record['review'] == 'passed' and record['regressions'] == 0)


def summarise(planned, records):
    available = [records[r['id']] for r in planned if r['id'] in records]
    complete = len(available) == len(planned)
    verified = sum(success(r) for r in available)
    known_cost = sum(r['cost_usd'] for r in available if r['cost_usd'] is not None)
    unknown_cost = sum(r['cost_usd'] is None for r in available)
    attempts = [r for r in available if r['recovery_attempted']]
    unknown_recovery = sum(r['recovery_succeeded'] is None for r in attempts)
    return dict(expected_runs=len(planned), recorded_runs=len(available), missing_runs=len(planned)-len(available),
        verified_successes=verified, verified_success_rate=verified/len(planned) if complete and planned else None,
        recorded_outcomes={s: sum(r['outcome'] == s for r in available)
                           for s in ['completed', 'failed', 'cancelled', 'timed_out']},
        incomplete_verifications=sum(r['tests'] in {'missing', 'skipped'} or r['review'] in {'missing', 'skipped'} for r in available),
        recorded_regressions=sum(r['regressions'] for r in available),
        known_cost_usd=known_cost, unknown_cost_runs=unknown_cost,
        total_cost_usd=known_cost if complete and not unknown_cost else None,
        cost_per_verified_success_usd=known_cost/verified if complete and not unknown_cost and verified else None,
        median_elapsed_seconds=statistics.median(r['elapsed_seconds'] for r in available) if complete and available else None,
        recorded_interventions=sum(r['interventions'] for r in available),
        recovery_attempts=len(attempts), unknown_recovery_outcomes=unknown_recovery,
        recovery_success_rate=sum(r['recovery_succeeded'] is True for r in attempts)/len(attempts)
            if complete and attempts and not unknown_recovery else None)


def paired_uncertainty(planned, records, seed):
    if any(r['id'] not in records for r in planned):
        return dict(status='incomplete', difference=None, interval_95=None)
    tasks = {}
    for run in planned:
        groups = tasks.setdefault(run['task']['id'], {'hand': [], 'pi': []})
        groups[run['agent']['id']].append(int(success(records[run['id']])))
    differences = [statistics.mean(groups['hand'])-statistics.mean(groups['pi'])
                   for _, groups in sorted(tasks.items())]
    if not differences:
        return dict(status='incomplete', difference=None, interval_95=None)
    rng = random.Random(seed)
    bootstrap = sorted(statistics.mean(rng.choices(differences, k=len(differences))) for _ in range(2000))
    # Nearest-rank empirical percentile endpoints. Resample whole task clusters,
    # retaining repetitions and agent pairing within each sampled task.
    return dict(status='descriptive_unreviewed', difference=statistics.mean(differences),
                interval_95=[bootstrap[math.ceil(.025*len(bootstrap))-1], bootstrap[math.ceil(.975*len(bootstrap))-1]],
                task_clusters=len(differences), bootstrap_replicates=2000, seed=seed,
                direction='Hand minus Pi verified success rate', method='paired task-cluster percentile bootstrap')


def analyse(manifest, manifest_root, results, evidence_root):
    plan = schedule(manifest, manifest_root)
    if not isinstance(results, dict) or set(results) != {'schema_version', 'manifest_sha256', 'runs'} or type(results['schema_version']) is not int or results['schema_version'] != 1:
        raise ValueError('invalid result envelope')
    if results['manifest_sha256'] != plan['manifest_sha256'] or not isinstance(results['runs'], list):
        raise ValueError('results are not bound to the frozen manifest')
    expected = {r['id']: r for r in plan['runs']}
    if len(results['runs']) > len(expected):
        raise ValueError('more records than planned runs; retries require separate explicit accounting')
    records, violations = {}, []
    for record in results['runs']:
        verify_record(record, evidence_root)
        run_id = record['run_id']
        if not isinstance(run_id, str) or run_id not in expected or run_id in records:
            raise ValueError('unknown or duplicate run ID; retries require separate explicit accounting')
        records[run_id] = record
        cap = expected[run_id]['budget']['max_cost_usd']
        if record['cost_usd'] is not None and record['cost_usd'] > cap:
            violations.append(dict(run_id=run_id, maximum_cost_usd=cap, observed_cost_usd=record['cost_usd']))
    groups, comparisons = {}, {}
    for mode in sorted({r['mode'] for r in plan['runs']}):
        for phase in ['all', 'calibration', 'held_out']:
            group = [r for r in plan['runs'] if r['mode'] == mode and (phase == 'all' or r['phase'] == phase)]
            for agent in ['hand', 'pi']:
                groups[f'{mode}/{phase}/{agent}'] = summarise([r for r in group if r['agent']['id'] == agent], records)
            comparisons[f'{mode}/{phase}'] = paired_uncertainty(group, records, plan['seed'])
    return dict(schema_version=1, status='records_complete_unreviewed' if len(records) == len(expected) else 'incomplete',
                manifest_sha256=plan['manifest_sha256'], expected_runs=len(expected), recorded_runs=len(records),
                missing_run_ids=sorted(set(expected)-set(records)), cost_cap_violations=violations,
                groups=groups, paired_comparisons=comparisons,
                limitations=['No run was executed or authorised by this report.',
                    'Hashes establish artifact identity, not truth of verification, source/model provenance or spending authorisation; independent reconciliation is required.',
                    'Missing runs yield null rates and paired intervals, never an easier denominator. Unknown cost is not zero.',
                    'Intervals resample the selected task clusters and preserve repeated paired observations; they do not establish general superiority or cover unrepresented tasks.',
                    'Defaults and configured modes are separate; calibration and held-out summaries remain identifiable.'])


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('manifest', type=Path)
    p.add_argument('results', type=Path)
    p.add_argument('--out', type=Path, required=True)
    args = p.parse_args()
    try:
        manifest_raw, results_raw = args.manifest.read_bytes(), args.results.read_bytes()
        result = analyse(json.loads(manifest_raw), args.manifest.resolve().parent,
                         json.loads(results_raw), args.results.resolve().parent)
        result['inputs'] = {str(path.resolve()): hashlib.sha256(raw).hexdigest() for path, raw in
                           [(args.manifest, manifest_raw), (args.results, results_raw), (Path(__file__), Path(__file__).read_bytes())]}
        with args.out.open('x') as output:
            output.write(json.dumps(result, indent=2, allow_nan=False)+'\n')
    except (ValueError, OSError) as error:
        print(json.dumps(dict(status='invalid', error=str(error))), file=sys.stderr)
        return 2
    print(json.dumps(dict(status=result['status'], recorded_runs=result['recorded_runs'], expected_runs=result['expected_runs'])))
    return 1 if result['status'] == 'incomplete' else 0


if __name__ == '__main__':
    raise SystemExit(main())

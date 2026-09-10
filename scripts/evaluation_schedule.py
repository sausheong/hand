"""Create a reproducible paired run plan. This neither executes nor authorises runs."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import random
from evaluation_manifest import validate


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False).encode()


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def schedule(manifest, root):
    validation = validate(manifest, root)
    if validation['status'] != 'qualification_inputs_valid':
        raise ValueError('freeze qualification inputs before scheduling: ' + '; '.join(validation['qualification_gaps']))
    frozen = copy.deepcopy(manifest)
    rng = random.Random(frozen['seed'])
    agents = {a['id']: a for a in frozen['agents']}
    modes = {m['id']: m for m in frozen['modes']}
    runs, pairs = [], []
    manifest_digest = digest(frozen)
    for phase in ['calibration', 'held_out']:
        blocks = []
        for mode in sorted(modes):
            group = [(task, repeat) for task in frozen['tasks'] if task['split'] == phase
                     for repeat in range(1, frozen['repetitions'] + 1)]
            rng.shuffle(group)
            # Counterbalance first agent independently within each phase/mode.
            offset = rng.randrange(2)
            for index, (task, repeat) in enumerate(group):
                first = ['hand', 'pi'][(index + offset) % 2]
                order = [first, 'pi' if first == 'hand' else 'hand']
                blocks.append((mode, task, repeat, order))
        rng.shuffle(blocks)
        for mode, task, repeat, order in blocks:
            identity = dict(manifest_sha256=manifest_digest, task=task['id'], mode=mode, repetition=repeat)
            pair_id = digest(identity)
            pair = dict(id=pair_id, phase=phase, task=task['id'], mode=mode, repetition=repeat,
                        agent_order=order, run_ids=[])
            for position, agent_id in enumerate(order):
                run_id = digest(dict(pair_id=pair_id, agent=agent_id))
                pair['run_ids'].append(run_id)
                runs.append(dict(id=run_id, position=len(runs), pair_id=pair_id, pair_position=position,
                    phase=phase, task=copy.deepcopy(task), mode=mode, repetition=repeat,
                    agent=copy.deepcopy(agents[agent_id]), configuration=copy.deepcopy(modes[mode]['agents'][agent_id]),
                    budget=copy.deepcopy(frozen['per_run_budget']), task_sha256=digest(task)))
            pairs.append(pair)
    assert len(runs) == validation['planned_runs']
    return dict(schema_version=1, status='planned_not_authorised', manifest_sha256=manifest_digest,
                seed=frozen['seed'], order='counterbalanced', phases=['calibration', 'held_out'],
                run_count=len(runs), pair_count=len(pairs), planned_total_usd=frozen['planned_total_usd'],
                pairs=pairs, runs=runs,
                execution_conditions=['Resolve and verify all source, binary, model and fixture pins.',
                    'Obtain separate budget authorisation; this plan grants none.',
                    'Use fresh independent workspaces and sessions for every run.',
                    'Freeze configuration after calibration before executing the held-out phase.',
                    'If calibration changes configuration, create a new input digest and schedule before held-out execution.',
                    'Record failures, cancellations and actual order; do not replace failed runs with unrecorded retries.'])


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('manifest', type=Path)
    p.add_argument('--out', type=Path, required=True)
    a = p.parse_args()
    raw = a.manifest.read_bytes()
    result = schedule(json.loads(raw), a.manifest.resolve().parent)
    result['manifest_file_sha256'] = hashlib.sha256(raw).hexdigest()
    result['runner_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    payload = json.dumps(result, indent=2, allow_nan=False) + '\n'
    with a.out.open('x') as output:
        output.write(payload)
    print(json.dumps(dict(status=result['status'], run_count=result['run_count'],
                         schedule_sha256=hashlib.sha256(payload.encode()).hexdigest())))


if __name__ == '__main__':
    main()

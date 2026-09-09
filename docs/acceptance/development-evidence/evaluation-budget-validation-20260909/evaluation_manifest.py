"""Validate pinned evaluation inputs. This never authorises or executes model calls."""
import argparse
import hashlib
import json
import math
from fractions import Fraction
from pathlib import Path, PurePosixPath
import re


def validate(manifest, root):
    def require(condition, message):
        if not condition: raise ValueError(message)
    def string(value): return isinstance(value, str) and bool(value.strip())
    def integer(value): return type(value) is int and value > 0
    def money(value):
        if type(value) not in (float, int): return False
        try: return math.isfinite(value) and value >= 0
        except OverflowError: return False
    def relative(value):
        return string(value) and not PurePosixPath(value).is_absolute() and '..' not in PurePosixPath(value).parts and '\\' not in value
    require(isinstance(manifest,dict), 'manifest must be an object')
    require(type(manifest.get('schema_version')) is int and manifest['schema_version'] == 1, 'unsupported manifest version')
    require(string(manifest.get('id')), 'manifest id required')
    require(manifest.get('purpose') in ['calibration', 'qualification'], 'purpose required')
    require(type(manifest.get('seed')) is int, 'integer seed required')
    require(manifest.get('order') == 'counterbalanced', 'counterbalanced order required')
    repetitions = manifest.get('repetitions')
    require(integer(repetitions), 'positive repetitions required')
    budget = manifest.get('per_run_budget', {})
    require(all(integer(budget.get(k)) for k in ['timeout_seconds','max_requests','max_input_tokens','max_output_tokens']), 'finite positive time/request/token limits required')
    require(money(budget.get('max_cost_usd')), 'finite nonnegative per-run cost required')
    require(money(manifest.get('planned_total_usd')), 'finite nonnegative total cost required')
    agents, modes, tasks = manifest.get('agents'), manifest.get('modes'), manifest.get('tasks')
    require(isinstance(agents,list) and isinstance(modes,list) and isinstance(tasks,list) and tasks, 'agent/mode/task arrays required; tasks cannot be empty')
    seen_agents, seen_modes, seen_tasks = set(), set(), set()
    for agent in agents:
        require(isinstance(agent,dict), 'agent must be an object')
        require(string(agent.get('id')) and agent['id'] not in seen_agents, 'unique agent id required')
        seen_agents.add(agent['id'])
        require(bool(re.fullmatch('[0-9a-f]{40}', agent.get('commit',''))), 'agent needs full commit')
        require(string(agent.get('repository')), 'agent repository required')
        model=agent.get('model',{})
        require(all(string(model.get(k)) for k in ['provider','id','reasoning']), 'explicit provider/model/reasoning required')
    for mode in modes:
        require(isinstance(mode,dict), 'mode must be an object')
        require(mode.get('id') in ['defaults','configured'] and mode['id'] not in seen_modes, 'unique defaults/configured mode required')
        seen_modes.add(mode['id'])
        configs=mode.get('agents',{})
        require(set(configs)==seen_agents, 'each mode must configure exactly the declared agents')
        for config in configs.values():
            require(isinstance(config.get('tools'),list) and all(string(t) for t in config['tools']), 'explicit tool list required')
            require(isinstance(config.get('settings'),dict), 'explicit settings object required')
            require(isinstance(config.get('packages'),list), 'explicit package list required')
            for package in config['packages']:
                require(string(package.get('id')) and re.fullmatch('[0-9a-f]{64}',package.get('sha256','')), 'packages need identity and digest')
    fixture_records=[]
    for task in tasks:
        require(isinstance(task,dict), 'task must be an object')
        require(string(task.get('id')) and task['id'] not in seen_tasks, 'unique task id required')
        seen_tasks.add(task['id'])
        require(task.get('split') in ['calibration','held_out'], 'explicit task split required')
        require(task.get('category') in ['bug_fix','feature','refactor','tests','discovery','continuation'], 'task category required')
        require(string(task.get('prompt')) and string(task.get('review_criterion')), 'prompt and independent review criterion required')
        repo=task.get('repository',{})
        require(string(repo.get('url')) and bool(re.fullmatch('[0-9a-f]{40}',repo.get('commit',''))), 'task repository and full commit required')
        require(relative(task.get('working_directory')), 'safe working directory required')
        fixtures=task.get('fixtures',[])
        require(isinstance(fixtures,list), 'fixtures must be an array')
        destinations=set()
        for fixture in fixtures:
            require(relative(fixture.get('source')) and relative(fixture.get('destination')), 'safe fixture paths required')
            require(fixture['destination'] not in destinations, 'duplicate fixture destination')
            destinations.add(fixture['destination'])
            path=(root/fixture['source']).resolve()
            require(path.is_relative_to(root.resolve()) and path.is_file(), 'fixture must exist inside manifest directory')
            digest=hashlib.sha256(path.read_bytes()).hexdigest()
            require(fixture.get('sha256')==digest, 'fixture hash mismatch')
            fixture_records.append(dict(task=task['id'],source=fixture['source'],sha256=digest))
        checks=task.get('verification')
        require(isinstance(checks,list) and checks, 'nonempty verification commands required')
        for check in checks:
            argv=check.get('argv')
            require(isinstance(argv,list) and argv and all(string(a) for a in argv), 'verification must use explicit argv')
            require(type(check.get('expected_exit')) is int and check['expected_exit']==0, 'successful verification exit must be zero')
            require(integer(check.get('timeout_seconds')), 'verification timeout required')
    planned_runs=len(tasks)*len(agents)*len(modes)*repetitions
    gaps=[]
    if len(tasks)<30:gaps.append('fewer than 30 tasks')
    if sum(t['split']=='held_out' for t in tasks)<10:gaps.append('fewer than 10 held-out tasks')
    if repetitions<3:gaps.append('fewer than three repetitions')
    if seen_agents!={'hand','pi'}:gaps.append('Hand and Pi revisions/model settings are not both pinned')
    if seen_modes!={'defaults','configured'}:gaps.append('both comparison modes are not configured')
    if {t['category'] for t in tasks}!={'bug_fix','feature','refactor','tests','discovery','continuation'}:gaps.append('task categories are incomplete')
    # Compare decimal input values exactly: binary float multiplication can
    # incorrectly reject a total such as 0.30 for three runs capped at 0.10.
    if Fraction(str(manifest['planned_total_usd'])) < planned_runs*Fraction(str(budget['max_cost_usd'])):gaps.append('planned total does not cover all per-run caps')
    return dict(status='qualification_inputs_valid' if manifest['purpose']=='qualification' and not gaps else 'draft',
                planned_runs=planned_runs, qualification_gaps=gaps, fixtures=fixture_records,
                authorisation='Not granted by this manifest or validator. Paid execution requires separate user authorisation.')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest',type=Path)
    parser.add_argument('--qualification',action='store_true')
    args=parser.parse_args()
    try:
        payload=args.manifest.read_bytes()
        result=validate(json.loads(payload),args.manifest.resolve().parent)
        result['manifest_sha256']=hashlib.sha256(payload).hexdigest()
        print(json.dumps(result,indent=2))
        return 1 if args.qualification and result['status']!='qualification_inputs_valid' else 0
    except (ValueError,TypeError,KeyError,AttributeError,OSError) as error:
        print(json.dumps({'status':'invalid','error':str(error)}))
        return 2


if __name__=='__main__':raise SystemExit(main())

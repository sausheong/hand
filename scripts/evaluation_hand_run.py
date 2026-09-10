"""Supervise and observe Hand; callers own approval, configuration and scoring."""
import json
from pathlib import Path
from evaluation_hand_events import HandRunObserver
from evaluation_process import run_command


def run_hand(argv, workspace, evidence, environment, timeout_seconds, max_output_bytes, cancelled=None):
    process = run_command(argv, workspace, evidence, timeout_seconds, max_output_bytes, environment, cancelled)
    observer = HandRunObserver(max_bytes=max_output_bytes)
    with Path(process['logs']['stdout']['path']).open('rb') as stream:
        while data := stream.read(65536):
            observer.feed(data)
    observation = observer.finish()
    expected_exit = {'completed': 0, 'verification_failed': 3, 'budget_exhausted': 4,
                     'cancelled': 130, 'infrastructure_error': 5}.get(observation['outcome'])
    consistent = (observation['status'] == 'observed_unverified'
                  and process['status'] in ('completed', 'failed')
                  and process['process_and_pipes_joined']
                  and expected_exit == process['exit_code'])
    report = dict(status='observed_unverified' if consistent else 'incomplete',
                  process=process, observation=observation,
                  limitation='No task correctness, model identity, deployment isolation or billing verification.')
    (Path(evidence)/'hand-run.json').write_text(json.dumps(report, indent=2)+'\n')
    return report

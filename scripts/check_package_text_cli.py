#!/usr/bin/env python3
"""Exercise pinned package skills and prompts through a built one-shot client."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import platform
from pathlib import Path
import shutil
import subprocess
import threading


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--hand-binary', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--execution-config', type=Path, help='Explicit container fixture configuration')
    args = parser.parse_args()
    binary = args.hand_binary.resolve(strict=True)
    root = args.out.resolve(); root.mkdir(parents=True, exist_ok=False)
    home = root / 'home'; home.mkdir(mode=0o700)
    workspace = root / 'workspace'; workspace.mkdir()
    source = root / 'source'; source.mkdir()
    store = root / 'store'
    env = {'HOME': str(home), 'PATH': '/usr/bin:/bin'}
    execution = None
    container_commands = []
    before_containers = set()
    def containers():
        config = root / 'docker-config'; config.mkdir(mode=0o700, exist_ok=True)
        argv = [execution['docker'], '--config', str(config), '--host', 'unix://' + execution['socket'],
                'ps', '-aq', '--filter', 'name=harness-exec-']
        result = subprocess.run(argv, env=env, capture_output=True, text=True, timeout=10)
        container_commands.append(dict(argv=argv, exit_code=result.returncode, stdout=result.stdout, stderr=result.stderr))
        (root / 'container-commands.json').write_text(json.dumps(container_commands, indent=2)+'\n')
        assert result.returncode == 0, container_commands[-1]
        return set(result.stdout.split())
    prompt = 'Package prompt marker. Keep @/missing/private.txt and $(touch sentinel) literal.'
    skill = 'Package skill marker: read refs/checklist.md relative to this skill before changing implementation.'
    checklist = 'Installed relative resource marker: preserve unrelated changes.'
    manifest = dict(schema=1, name='text-journey', version='1.0.0',
                    compatibility=dict(minimum_hand='0.1.0', extension_protocol=1), files=[])
    for filename, kind, content in [('SKILL.md', 'skill', skill), ('review.md', 'prompt', prompt),
                                     ('refs/checklist.md', 'asset', checklist)]:
        path = source / filename; path.parent.mkdir(parents=True, exist_ok=True); path.write_text(content)
        manifest['files'].append(dict(path=filename, kind=kind, sha256=sha(path), size=path.stat().st_size))
    (source / 'hand-package.json').write_text(json.dumps(manifest))
    requests, commands = [], []
    report = dict(status='failed', qualification='development', binary_sha256=sha(binary),
                  runner_sha256=sha(Path(__file__)), paid_provider_calls=0, fixture_tool_approval='--yes in isolated fixture; provider requests only load_skill for the selected body and its declared text resource')

    def run(words, success=True):
        result = subprocess.run([str(binary), *map(str, words)], cwd=workspace, env=env,
                                capture_output=True, text=True, timeout=30)
        commands.append(dict(arguments=list(map(str, words)), exit_code=result.returncode,
                             stdout=result.stdout, stderr=result.stderr))
        (root / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        assert (result.returncode == 0) == success, commands[-1]
        return result

    class Provider(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            requests.append(body)
            outputs = [m for m in body['messages'] if m['role'] == 'tool']
            if not outputs:
                call_id, tool_name, arguments = 'load-package-skill', 'load_skill', dict(name='package-review')
            elif len(outputs) == 1:
                # Verify actual tool provenance, then ask the bounded loader
                # for the relative reference. No unrestricted host read is used.
                base_line = next(line for line in outputs[0]['content'].splitlines()
                                 if line.startswith('Resource base directory: '))
                base = Path(json.loads(base_line.removeprefix('Resource base directory: ')))
                assert base == store / 'objects' / pin
                call_id, tool_name, arguments = 'read-skill-resource', 'load_skill', dict(name='package-review', resource='refs/checklist.md')
            elif execution and len(outputs) == 2:
                call_id, tool_name, arguments = 'confirm-container-process', 'bash', dict(command='uname -s')
            if len(outputs) < (3 if execution else 2):
                message = dict(role='assistant', content=None, tool_calls=[dict(id=call_id, type='function',
                    function=dict(name=tool_name, arguments=json.dumps(arguments)))])
                finish = 'tool_calls'
            else:
                message = dict(role='assistant', content='Package text journey complete.')
                finish = 'stop'
            self.send_response(200)
            if body.get('stream'):
                self.send_header('Content-Type', 'text/event-stream'); self.end_headers()
                delta = dict(message); delta.pop('role')
                if 'tool_calls' in delta:
                    delta['tool_calls'][0]['index'] = 0
                for value in [dict(choices=[dict(index=0, delta=delta, finish_reason=None)]),
                              dict(choices=[dict(index=0, delta={}, finish_reason=finish)])]:
                    self.wfile.write(('data: ' + json.dumps(value) + '\n\n').encode())
                self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
            else:
                raw = json.dumps(dict(id='fixture', model='fixture', choices=[dict(index=0, message=message,
                                 finish_reason=finish)], usage=dict(prompt_tokens=30, completion_tokens=10, total_tokens=40))).encode()
                self.send_header('Content-Type', 'application/json'); self.send_header('Content-Length', str(len(raw)))
                self.end_headers(); self.wfile.write(raw)

    server = None
    try:
        if args.execution_config:
            execution = json.loads(args.execution_config.read_text())
            assert isinstance(execution, dict)
            assert execution['backend'] == 'container' and not execution['network'] and not execution['writable']
            configdir = home / '.hand'; configdir.mkdir(mode=0o700)
            (configdir/'config.json').write_text(json.dumps(dict(execution=execution)))
            before_containers = containers()
            report['fixture_tool_approval'] += '; container fixture also approves uname -s through bash'
            report['execution'] = execution
            report['host_platform'] = platform.system()
        inspected = json.loads(run(['packages', 'inspect', '--source', source]).stdout)
        pin = inspected['package_digest']
        change = json.loads(run(['packages', 'review-install', '--source', source, '--pin', pin,
                                 '--store', store, '--out', root / 'install.json']).stdout)
        run(['packages', 'apply', '--review', change['review_file'], '--approve', change['approval_digest']])
        shutil.rmtree(source)
        skills_file, prompt_file = root / 'skills.json', root / 'prompt.json'
        skills_file.write_text(json.dumps(dict(version=1, store=str(store), skills=[dict(
            package='text-journey', digest=pin, path='SKILL.md', name='package-review')])))
        selected_prompt = dict(version=1, store=str(store), package='text-journey', digest=pin, path='review.md')
        prompt_file.write_text(json.dumps(selected_prompt))
        server = ThreadingHTTPServer(('127.0.0.1', 0), Provider)
        thread = threading.Thread(target=server.serve_forever); thread.start()
        words = ['--model', 'local/fixture', '--base-url', 'http://127.0.0.1:' + str(server.server_port) + '/v1',
                 '--yes', '--package-skills', skills_file, '--package-prompt', prompt_file, '-p', 'Use package-review and explain the change.']
        result = run(words)
        assert 'Package text journey complete.' in result.stdout
        expected_requests = 4 if execution else 3
        assert len(requests) == expected_requests, requests
        first = requests[0]['messages']; second = requests[1]['messages']
        assert any(m['role'] == 'user' and prompt in str(m['content']) for m in first)
        assert any(m['role'] == 'system' and 'package-review' in str(m['content']) and pin in str(m['content']) for m in first)
        assert any(m['role'] == 'tool' and skill in str(m['content']) for m in second)
        assert any(m['role'] == 'tool' and checklist in str(m['content']) for m in requests[2]['messages'])
        if execution:
            process_results = [m['content'] for m in requests[3]['messages']
                               if m['role'] == 'tool' and m.get('tool_call_id') == 'confirm-container-process']
            assert len(process_results) == 1 and 'Linux' in str(process_results[0]) and 'Darwin' not in str(process_results[0])
            assert not (containers() - before_containers), 'container survived client completion'
            report.update(container_process_reported_linux=True, container_cleanup=True)
        assert not (workspace / 'sentinel').exists()
        selected_prompt['digest'] = '0' * 64; prompt_file.write_text(json.dumps(selected_prompt))
        run(words, success=False)
        assert len(requests) == expected_requests, 'wrong prompt pin reached provider'
        report.update(status='passed', local_provider_requests=len(requests), package_digest=pin,
                      prompt_reached_user_message=True, skill_loaded_by_tool=True, relative_resource_read_by_tool=True,
                      source_removed=True, literal_prompt=True, wrong_pin_preprovider_rejection=True)
    except Exception as exc:
        report['error'] = repr(exc)
    finally:
        if server:
            server.shutdown(); server.server_close(); thread.join(timeout=5)
        if execution and container_commands:
            try:
                assert not (containers() - before_containers), 'container survived fixture cleanup'
            except Exception as exc:
                report.update(status='failed', cleanup_error=repr(exc))
        (root / 'provider-requests.json').write_text(json.dumps(requests, indent=2) + '\n')
        (root / 'run.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

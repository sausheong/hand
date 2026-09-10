#!/usr/bin/env python3
"""Run the full Hand Linux race suite in a pinned Go image with real fixtures."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import uuid
from evidence import source_snapshot


def summarise_go_log(text):
    tests, packages, open_tests, open_packages = {}, {}, set(), set()
    unparsed = 0
    for line in text.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            unparsed += 1
            continue
        if not isinstance(event, dict):
            unparsed += 1
            continue
        package, test, action = event.get('Package'), event.get('Test'), event.get('Action')
        if not package:
            unparsed += 1
            continue
        if action == 'start':
            open_packages.add(package)
        if test and action == 'run':
            open_tests.add((package, test))
        if action in ['pass', 'fail', 'skip']:
            counts = tests if test else packages
            counts[action] = counts.get(action, 0)+1
            if test:
                open_tests.discard((package, test))
            else:
                open_packages.discard(package)
    return dict(test_records=tests, package_records=packages,
                unfinished_tests=[dict(package=p, test=t) for p,t in sorted(open_tests)],
                unfinished_packages=sorted(open_packages), unparsed_lines=unparsed)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True)
    parser.add_argument('--docker', default='docker')
    parser.add_argument('--docker-cli', type=Path, required=True, help='Linux Docker executable mounted in test container')
    parser.add_argument('--socket', type=Path, required=True)
    parser.add_argument('--module-cache', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--socket-group', type=int, required=True, help='Observed Linux socket group granted to the test runner')
    args = parser.parse_args()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image):
        parser.error('immutable image ID required')
    root = Path(__file__).resolve().parent.parent
    out = args.out.resolve()
    if out == root or root in out.parents:
        parser.error('output must be outside checkout')
    out.mkdir(parents=True, exist_ok=False)
    for name in ['tmp', 'cache']:
        (out/name).mkdir()
    module = json.loads(subprocess.check_output(['go','list','-m','-json','github.com/sausheong/harness'],cwd=root))
    harness = Path(module['Dir']).resolve()
    if module.get('Replace') or module.get('Version') != 'v0.4.0' or not module.get('Sum'):
        raise RuntimeError('published Harness v0.4.0 with checksum required')
    def snapshot_inputs():
        # Published module caches have no Git repository. Hash all shipped files.
        entries = []
        for path in sorted(harness.rglob('*')):
            if path.is_file():
                entries.append(dict(path=str(path.relative_to(harness)), sha256=hashlib.sha256(path.read_bytes()).hexdigest()))
        digest = hashlib.sha256(json.dumps(entries,sort_keys=True).encode()).hexdigest()
        return {str(root): source_snapshot(root), str(harness): dict(sha256=digest, entries=entries)}
    def git(*argv):return subprocess.check_output(['git',*argv],cwd=root,text=True).strip()
    report = dict(commit_before=git('rev-parse','HEAD'),clean_before=not git('status','--porcelain'),status='failed',qualification='published_dependency_linux_runtime',commands=[],harness_module=module,
                  sources_before=snapshot_inputs())
    env = dict(os.environ,DOCKER_HOST='unix://'+str(args.socket))
    settings = dict(GOCACHE=str(out/'cache'),GOMODCACHE='/gomod',GOPROXY='off',GOSUMDB='off',
                    GOTOOLCHAIN='local',GOWORK='off',GIT_CONFIG_COUNT='1',
                    GIT_CONFIG_KEY_0='safe.directory',GIT_CONFIG_VALUE_0=str(root),TMPDIR=str(out/'tmp'),CGO_ENABLED='1',
                    HARNESS_TEST_CONTAINER_SOCKET=str(args.socket),HAND_TEST_CONTAINER_ARCH='arm64',
                    HARNESS_TEST_CONTAINER_IMAGE='sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818',
                    HARNESS_TEST_BASH_IMAGE='sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818',
                    HAND_TEST_PYTHON_IMAGE='sha256:6771159cd4fa5d9bba1258caf0b82e6b73458c694d178ad97c5e925c2d0e1a91',
                    HAND_TEST_WORKER_BINARY=str(out/'worker'),HAND_TEST_EXTENSION_PEER_BINARY=str(out/'peer.test'),
                    HAND_TEST_ENOSPC_ROOT='/case',HAND_TEST_ENOSPC_BINARY=str(out/'hand'),
                    HAND_TEST_PUBLIC_FETCH_URL='https://example.com/')
    settings['HOME']=str(out/'home')
    (out/'home').mkdir()
    report['filesystem_note']='Linux runtime in Docker on macOS; temporary workspaces are host-shared, while ENOSPC uses an explicit Linux tmpfs.'
    report['environment']=settings

    def run(label, command, timeout):
        name='hand-linux-full-'+uuid.uuid4().hex
        argv=[args.docker,'run','--rm','--name',name,'--read-only','--tmpfs','/tmp:rw,exec,size=256m,mode=1777',
              '--tmpfs','/case:rw,exec,size=2m,mode=1777','--workdir',str(root),'--user',str(os.getuid())+':'+str(os.getgid()),
              '--group-add',str(args.socket_group)]
        for source,target,readonly in [(root,root,True),(harness,harness,True),(args.module_cache,'/gomod',True),
                                      (out,out,False),(args.socket,args.socket,False),(args.docker_cli,'/usr/local/bin/docker',True)]:
            argv+=['--mount',f'type=bind,source={source},target={target}'+(',readonly' if readonly else '')]
        for key,value in settings.items():argv+=['-e',key+'='+value]
        argv += [args.image]+command
        start=time.monotonic()
        try:
            with (out/(label+'.log')).open('wb') as stream:
                result=subprocess.run(argv,env=env,stdout=stream,stderr=subprocess.STDOUT,timeout=timeout)
            report['commands'].append(dict(argv=argv,exit_code=result.returncode,seconds=time.monotonic()-start,log=label+'.log'))
            if result.returncode:raise RuntimeError(label+' failed')
        finally:
            subprocess.run([args.docker,'rm','-f',name],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=15)
        print(label+' passed',flush=True)

    try:
        run('environment',['go','env','-json','GOVERSION','GOOS','GOARCH','CGO_ENABLED'],30)
        observed=json.loads((out/'environment.log').read_text())
        report['platform']=observed
        if observed['GOOS']!='linux' or observed['GOARCH']!='arm64' or observed['CGO_ENABLED']!='1':
            raise RuntimeError('unexpected runtime architecture or race capability')
        run('build-hand',['go','build','-o',str(out/'hand'),'./cmd/hand'],300)
        run('build-worker',['go','build','-o',str(out/'worker'),'./cmd/hand-tool-worker'],300)
        run('build-peer',['go','test','-c','-o',str(out/'peer.test'),'./internal/app'],300)
        run('check-peer-entry',[str(out/'peer.test'),'-test.list=^TestFileWritePeerProcess$'],30)
        if 'TestFileWritePeerProcess' not in (out/'check-peer-entry.log').read_text().splitlines():
            raise RuntimeError('extension peer binary is missing TestFileWritePeerProcess')
        report['binaries']={p:hashlib.sha256((out/p).read_bytes()).hexdigest() for p in ['hand','worker','peer.test']}
        run('vet',['go','vet','./...'],300)
        run('packages',['go','list','./...'],60)
        run('inventory',['go','test','-json','-list','^(Test|Example|Fuzz)','./...'],300)
        run('suite',['go','test','-p=1','-race','-count=1','-timeout=15m','-coverpkg=./...',
                     '-coverprofile='+str(out/'coverage.out'),'-json','./...'],1800)
        summary=summarise_go_log((out/'suite.log').read_text())
        report.update(summary)
        counts=summary['test_records']
        if not counts.get('pass') or counts.get('fail') or counts.get('skip') or summary['unfinished_tests'] or summary['unfinished_packages'] or summary['package_records'].get('fail'):
            raise RuntimeError('required test records failed, skipped, unfinished or absent')
        report['sources_after']=snapshot_inputs()
        if report['sources_before']!=report['sources_after']:raise RuntimeError('source changed during suite')
        report['status']='development_passed'
    except Exception as exc:
        report['error']=repr(exc)
    finally:
        if (out/'suite.log').exists():
            report.update(summarise_go_log((out/'suite.log').read_text()))
        try:
            report['sources_after']=snapshot_inputs()
        except Exception as exc:
            report['status']='failed'
            report['source_snapshot_error']=repr(exc)
        report['commit_after']=git('rev-parse','HEAD')
        report['clean_after']=not git('status','--porcelain')
        report['artifacts']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in out.iterdir() if p.is_file()}
        (out/'run.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','test_records','error'] if k in report}))
    return 0 if report['status']=='development_passed' else 1


if __name__=='__main__':
    raise SystemExit(main())

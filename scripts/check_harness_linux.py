#!/usr/bin/env python3
"""Run the full Harness Linux race suite in a pinned Go image with real fixtures."""
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
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--socket-group', type=int, required=True, help='Socket group observed inside the Linux fixture')
    parser.add_argument('--docker', default='docker')
    parser.add_argument('--docker-cli', type=Path, required=True, help='Linux Docker executable mounted in test container')
    parser.add_argument('--socket', type=Path, required=True)
    parser.add_argument('--module-cache', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image):
        parser.error('immutable image ID required')
    if args.socket_group < 0:
        parser.error('nonnegative socket group required')
    root = args.source.resolve()
    out = args.out.resolve()
    if out == root or root in out.parents:
        parser.error('output must be outside checkout')
    out.mkdir(parents=True, exist_ok=False)
    for name in ['tmp', 'cache']:
        (out/name).mkdir()
    roots = [root]
    report = dict(status='failed',qualification='development',commands=[],
                  sources_before={str(p):source_snapshot(p) for p in roots})
    env = dict(os.environ,DOCKER_HOST='unix://'+str(args.socket))
    settings = dict(GOCACHE=str(out/'cache'),GOMODCACHE='/gomod',GOPROXY='off',GOSUMDB='off',
                    GOTOOLCHAIN='local',GOWORK='off',TMPDIR=str(out/'tmp'),CGO_ENABLED='1',
                    HARNESS_TEST_CONTAINER_SOCKET=str(args.socket),
                    HARNESS_TEST_CONTAINER_IMAGE='sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce',
                    HARNESS_TEST_BASH_IMAGE='sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818',
                    HARNESS_TEST_VOLUME_IMAGE='sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94')
    settings['HOME']=str(out/'home')
    (out/'home').mkdir()
    report['environment']=settings

    def run(label, command, timeout):
        name='hand-linux-full-'+uuid.uuid4().hex
        argv=[args.docker,'run','--rm','--name',name,'--read-only','--tmpfs','/tmp:rw,exec,size=256m,mode=1777',
              '--tmpfs','/case:rw,exec,size=2m,mode=1777','--workdir',str(root),'--user',str(os.getuid())+':'+str(os.getgid()),'--group-add',str(args.socket_group)]
        for source,target,readonly in [(root,root,True),(args.module_cache,'/gomod',True),
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
        report['temporary_storage']='dedicated host-shared output/tmp; outside Docker daemon root'
        run('environment',['go','env','-json','GOVERSION','GOOS','GOARCH','CGO_ENABLED'],30)
        observed=json.loads((out/'environment.log').read_text())
        if observed['GOOS']!='linux' or observed['GOARCH']!='arm64' or observed['CGO_ENABLED']!='1':
            raise RuntimeError('unexpected runtime architecture or race capability')
        run('vet',['go','vet','./...'],300)
        run('boundary-smoke',['go','test','-count=1','-race','-json','./execution','-run','^TestNativeContainerBoundary$'],180)
        run('suite',['go','test','-p=1','-race','-count=1','-timeout=15m','-coverpkg=./...',
                     '-coverprofile='+str(out/'coverage.out'),'-json','./...'],1800)
        summary=summarise_go_log((out/'suite.log').read_text())
        report.update(summary)
        counts=summary['test_records']
        if not counts.get('pass') or counts.get('fail') or counts.get('skip') or summary['unfinished_tests'] or summary['unfinished_packages'] or summary['package_records'].get('fail'):
            raise RuntimeError('required test records failed, skipped, unfinished or absent')
        report['sources_after']={str(p):source_snapshot(p) for p in roots}
        if report['sources_before']!=report['sources_after']:raise RuntimeError('source changed during suite')
        report['status']='development_passed'
    except Exception as exc:
        report['error']=repr(exc)
    finally:
        if (out/'suite.log').exists():
            report.update(summarise_go_log((out/'suite.log').read_text()))
        try:
            report['sources_after']={str(p):source_snapshot(p) for p in roots}
        except Exception as exc:
            report['status']='failed'
            report['source_snapshot_error']=repr(exc)
        report['artifacts']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in out.iterdir() if p.is_file()}
        (out/'run.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','test_records','error'] if k in report}))
    return 0 if report['status']=='development_passed' else 1


if __name__=='__main__':
    raise SystemExit(main())

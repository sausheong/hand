#!/usr/bin/env python3
"""Collect uncached native macOS qualification for the retained Hand scope."""
import argparse
import json
import os
from pathlib import Path
import subprocess
from evidence import source_snapshot, coverage_metrics


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out',type=Path,required=True)
    args=parser.parse_args();root=Path(__file__).resolve().parent.parent;out=args.out.resolve()
    if out.is_relative_to(root):parser.error('evidence must be outside candidate')
    out.mkdir(parents=True)
    fixtures=json.loads((root/'docs/acceptance/current-native-suite.json').read_text())['environment']
    env=dict(PATH=os.environ['PATH'],HOME=os.environ['HOME'],TMPDIR=os.environ.get('TMPDIR','/tmp'),**fixtures)
    env.update(GOWORK='off',GOPROXY='off',GOCACHE='/private/tmp/hand-review-gocache',PYTHONDONTWRITEBYTECODE='1')
    env['HAND_TEST_WORKER_BINARY']=str(out/'worker');env['HAND_TEST_EXTENSION_PEER_BINARY']=str(out/'peer.test')
    def git(*argv):return subprocess.check_output(['git',*argv],cwd=root,text=True).strip()
    report=dict(commit_before=git('rev-parse','HEAD'),clean_before=not git('status','--porcelain'),
                source_before=source_snapshot(root),environment=env,commands=[],status='failed')
    def run(label,argv,settings=env):
        with (out/(label+'.log')).open('w') as log:
            result=subprocess.run(argv,cwd=root,env=settings,stdout=log,stderr=subprocess.STDOUT,timeout=1800)
        report['commands'].append(dict(label=label,argv=argv,exit_code=result.returncode,log=label+'.log'))
        if result.returncode:raise RuntimeError(label+' failed')
        print(label+' passed',flush=True)
        return (out/(label+'.log')).read_text()
    try:
        report['platform']=json.loads(run('platform',['go','env','-json','GOOS','GOARCH','CGO_ENABLED']))
        if report['platform']['GOOS']!='darwin' or report['platform']['CGO_ENABLED']!='1':raise RuntimeError('native macOS race runtime required')
        report['module']=json.loads(run('module',['go','list','-m','-json','github.com/sausheong/harness']))
        if report['module'].get('Version')!='v0.4.0' or report['module'].get('Replace'):raise RuntimeError('published Harness required')
        run('module-verify',['go','mod','verify'])
        cross=dict(env,GOOS='linux',GOARCH='arm64',CGO_ENABLED='0')
        run('worker-build',['go','build','-o',env['HAND_TEST_WORKER_BINARY'],'./cmd/hand-tool-worker'],cross)
        run('peer-build',['go','test','-c','-o',env['HAND_TEST_EXTENSION_PEER_BINARY'],'./internal/app'],cross)
        run('vet',['go','vet','./...'])
        run('packages',['go','list','./...'])
        run('inventory',['go','test','-json','-list','^(Test|Example|Fuzz)','./...'])
        run('tests',['go','test','-p=1','-count=1','-race','-timeout=15m','-coverpkg=./...',
                     '-coverprofile='+str(out/'coverage.out'),'-json','./...'])
        report['coverage']=coverage_metrics((out/'coverage.out').read_text())
        report['status']='tests_passed'
    except Exception as error:report['error']=str(error)
    finally:
        report['commit_after']=git('rev-parse','HEAD');report['clean_after']=not git('status','--porcelain')
        report['source_after']=source_snapshot(root);report['source_unchanged']=report['source_before']==report['source_after']
        if not report['source_unchanged']:report['status']='failed'
        (out/'run.json').write_text(json.dumps(report,indent=2)+'\n')
    return 0 if report['status']=='tests_passed' else 1


if __name__=='__main__':raise SystemExit(main())

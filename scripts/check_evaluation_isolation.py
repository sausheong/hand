#!/usr/bin/env python3
"""Qualify a network-disabled actual Hand agent via a guarded Unix gateway.

Uses only explicit loaded images and synthetic credentials/responses. No provider
calls, spending approval, daemon startup or shared Docker cleanup is performed.
"""
import argparse
import hashlib
import json
import ipaddress
import os
from pathlib import Path
import re
import secrets
import subprocess
import time
import uuid


def check(args):
    output = args.output.resolve(); output.mkdir(mode=0o700)
    binary = args.binary.resolve(); fixture = Path(__file__).with_name('evaluation_isolation_fixture.py').resolve()
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', args.image): raise ValueError('loaded immutable image required')
    module_metadata = subprocess.check_output(['go','version','-m',str(binary)], text=True)
    if 'github.com/sausheong/harness\tv0.4.0\t' not in module_metadata:
        raise ValueError('actual Hand binary linked to published Harness v0.4.0 required')
    prefix = [str(args.docker), '--host', 'unix://'+str(args.socket)]
    report = dict(status='failed', synthetic_upstream=True, binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
                  fixture_sha256=hashlib.sha256(fixture.read_bytes()).hexdigest(), image=args.image,
                  limitations=['Synthetic upstream verifies boundary/admission, not live billing or coding quality.',
                               'The tool container inside Hand is not nested here: the entire evaluation agent is isolated.'])
    report['module_metadata'] = module_metadata
    report['gateway_source_hashes'] = {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((args.source_root/'scripts').glob('evaluation_*.py'))}
    report['runner_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    identifier = 'hand-gateway-probe-'+uuid.uuid4().hex
    volume = identifier+'-socket'; names = []; created_volume = False; gateway_name = None
    uid = str(os.getuid()); gid = str(os.getgid())
    def docker(*argv, timeout=30):
        result = subprocess.run(prefix+list(argv), capture_output=True, timeout=timeout)
        if result.returncode: raise RuntimeError(result.stderr.decode(errors='replace'))
        return result.stdout.decode().strip()
    def inspect(name): return json.loads(docker('inspect', name))[0]
    def create(name, network, mounts, command, environment=()):
        argv = ['create', '--name', name, '--pull=never', '--network', network, '--read-only', '--cap-drop=ALL',
                '--security-opt=no-new-privileges:true', '--pids-limit=128', '--memory=512m', '--cpus=1',
                '--user',uid+':'+gid, '--tmpfs','/tmp:rw,nosuid,nodev,size=128m,mode=1777']
        for mount in mounts: argv += ['--mount',mount]
        for value in environment: argv += ['-e',value]
        docker(*(argv+[args.image]+command)); names.append(name)
        state=inspect(name)
        assert state['HostConfig']['NetworkMode']==network and state['HostConfig']['ReadonlyRootfs'] and not state['HostConfig']['Privileged']
        assert state['HostConfig']['CapDrop']==['ALL']
        assert 'no-new-privileges:true' in state['HostConfig']['SecurityOpt']
        return state
    def mount(source, target, readonly=True):
        return 'type=bind,source='+str(source)+',target='+target+(',readonly' if readonly else '')
    workspace=output/'workspace'; workspace.mkdir()
    (workspace/'example.go').write_text('package main\n// ISOLATED_HAND_MARKER\n')
    audit=output/'audit'; audit.mkdir(mode=0o700)
    private=output/'private'; private.mkdir(mode=0o700)
    token=secrets.token_hex(24); provider='synthetic-provider-'+secrets.token_hex(24)
    sentinel=private/'host-sentinel'; sentinel.write_text(provider); sentinel.chmod(0o600)
    gateway_config=private/'gateway.json'; gateway_config.write_text(json.dumps(dict(token=token,provider_key=provider)));gateway_config.chmod(0o600)
    try:
        docker('image','inspect',args.image)
        docker('volume','create',volume);created_volume=True
        docker('run','--rm','--pull=never','--network=none','--mount','type=volume,source='+volume+',target=/socket',args.image,
               'python3','-c','import os;os.chown("/socket",'+uid+','+gid+');os.chmod("/socket",0o700)')
        outside=identifier+'-outside'
        create(outside,'bridge',[],['python3','-m','http.server','8765']);docker('start',outside)
        outside_ip=inspect(outside)['NetworkSettings']['Networks']['bridge']['IPAddress']
        ipaddress.ip_address(outside_ip)
        # Positive control proves the target is listening before the denial test.
        docker('run','--rm','--pull=never','--network','bridge',args.image,'python3','-c',
               'import socket,time\nfor i in range(50):\n try:\n  s=socket.create_connection((' + repr(outside_ip) + ',8765),timeout=1);s.close();break\n except OSError:time.sleep(.1)\nelse:raise RuntimeError("sentinel not listening")')
        report['outside_sentinel_live']=True
        gateway_name=identifier+'-gateway'
        report['gateway_container']=create(gateway_name,'none',[
            mount(args.source_root/'scripts','/runner'),mount(fixture,'/fixture.py'),mount(gateway_config,'/private-config/config.json'),
            mount(audit,'/audit',False),'type=volume,source='+volume+',target=/socket'],
            ['python3','/fixture.py','gateway','/private-config/config.json'],['PYTHONPATH=/runner','PYTHONDONTWRITEBYTECODE=1'])
        docker('start',gateway_name)
        deadline=time.monotonic()+15
        while not (audit/'ready').exists():
            if time.monotonic()>deadline or not inspect(gateway_name)['State']['Running']:
                raise RuntimeError('gateway not ready: '+docker('logs',gateway_name))
            time.sleep(.1)
        agent_config=output/'agent.json';agent_config.write_text(json.dumps(dict(token=token,host_sentinel=str(sentinel),
            provider_sha256=hashlib.sha256(provider.encode()).hexdigest(),outside_ip=outside_ip)));agent_config.chmod(0o600)
        agent_name=identifier+'-agent'
        report['agent_container']=create(agent_name,'none',[
            mount(fixture,'/fixture.py'),mount(binary,'/opt/hand'),mount(agent_config,'/agent.json'),
            mount(workspace,'/workspace',False),'type=volume,source='+volume+',target=/socket,readonly'],
            ['python3','/fixture.py','agent','/agent.json'],['PYTHONDONTWRITEBYTECODE=1'])
        result=subprocess.run(prefix+['start','--attach',agent_name],capture_output=True,timeout=60)
        (output/'agent.stdout').write_bytes(result.stdout);(output/'agent.stderr').write_bytes(result.stderr)
        state=inspect(agent_name);report['agent_exit_code']=state['State']['ExitCode']
        if result.returncode or report['agent_exit_code']: raise RuntimeError('agent probe failed; see agent.stderr')
        docker('stop','--time','10',gateway_name)
        report['agent']=json.loads((workspace/'agent-report.json').read_text())
        report['gateway']=json.loads((audit/'gateway-report.json').read_text())
        assert report['agent']['passed'] and report['agent']['proxy_joined']
        assert report['gateway']['joined'] and not report['gateway']['failures'] and report['gateway']['requests_charged']==2
        assert len(report['gateway']['requests'])==2 and all(r['provider_credential_substituted'] for r in report['gateway']['requests'])
        assert report['gateway']['requests'][1]['tool_marker_seen']
        report['provider_secret_absent_from_agent_files']=all(provider.encode() not in p.read_bytes() for p in workspace.rglob('*') if p.is_file())
        assert report['provider_secret_absent_from_agent_files']
        report['status']='passed'
    except Exception as error:
        report['error']=str(error)
    finally:
        report['cleanup']=[]
        if gateway_name in names:
            try:
                if inspect(gateway_name)['State']['Running']: docker('stop','--time','10',gateway_name)
                if (audit/'gateway-report.json').exists():
                    report['gateway']=json.loads((audit/'gateway-report.json').read_text())
            except Exception as error:
                report['gateway_cleanup_error']=str(error);report['status']='failed'
        for name in reversed(names):
            result=subprocess.run(prefix+['rm','--force','--volumes',name],capture_output=True,timeout=20)
            absent=subprocess.run(prefix+['inspect',name],capture_output=True,timeout=10).returncode!=0
            report['cleanup'].append(dict(container=name,removed=result.returncode==0,absent=absent))
            if result.returncode or not absent:report['status']='failed'
        if created_volume:
            result=subprocess.run(prefix+['volume','rm',volume],capture_output=True,timeout=20)
            report['volume_removed']=result.returncode==0
            if result.returncode:report['status']='failed'
        report['source_unchanged'] = (
            hashlib.sha256(binary.read_bytes()).hexdigest() == report['binary_sha256']
            and hashlib.sha256(fixture.read_bytes()).hexdigest() == report['fixture_sha256']
            and hashlib.sha256(Path(__file__).read_bytes()).hexdigest() == report['runner_sha256']
            and {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((args.source_root/'scripts').glob('evaluation_*.py'))} == report['gateway_source_hashes'])
        if not report['source_unchanged']: report['status']='failed'
        # Only non-secret metadata and hashes are included in the report.
        (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ['status','error'] if k in report}))
    return 0 if report['status']=='passed' else 1


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['binary','docker','socket','output','source-root']:parser.add_argument('--'+name,type=Path,required=True)
    parser.add_argument('--image',required=True)
    raise SystemExit(check(parser.parse_args()))

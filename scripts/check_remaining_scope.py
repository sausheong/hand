#!/usr/bin/env python3
"""Audit the user-amended isolation/released-Harness scope from raw evidence.

The original full checker and requirement manifest remain unchanged. This audit
never turns omitted original requirements into passes or claims Pi superiority.
"""
import argparse
from collections import Counter
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
from evidence import source_snapshot, coverage_metrics
from coverage_changes import changed_lines, changed_coverage, classify_changed_sources
from critical_coverage import critical_coverage
from validate import check_events, inventory_names

ROOT=Path(__file__).resolve().parent.parent
MODULE='github.com/sausheong/hand'
HARNESS='github.com/sausheong/harness'
RELEASE_COMMIT='5d4632ffe3fdfeb171bb217a45ae75bef056811e'
RELEASE_SUM='h1:HoGrAbqYvmgsuR3RU14f6hHrOogrDhmSjqzQWsbEhMg='
PENDING_AUDIT='Integrated implementation requires final scenario audit, platform evidence and released Harness qualification; coverage mapping alone does not establish completion'
HAND_REQUIRED={
 'TestWorkspacePathMappingPreservesContentAndRejectsEscape','TestNativeProxyWorkerRoundtrip',
 'TestMissingBackendIsNotHostFallback','TestExecutionConfigurationFailsClosed',
 'TestUnsupportedToolRejectsInstallationAtomically','TestAllWorkerToolsReceiveContainerProxy',
 'TestContainerExtensionActivationAndReviewedReload','TestContainerExtensionFileCallbackBoundary',
 'TestContainerReviewSeparatesBoundaryApproval','TestNativeScopedSDKContainerJourney',
 'TestNativeSDKRejectsChangedWorker','TestNativeBackgroundContainerInputOutputAndShutdown',
 'TestPreparedTransportCloseDrainsStartupBacklogAfterNotifiedEOF',
 'TestUnsupportedPlatformOperationsPreserveWorkspace'}
HARNESS_REQUIRED={'TestNativeContainerBoundary','TestNativeCancellationStopsChildWrites',
 'TestMissingContainerNeverExecutesHostCommand','TestNativeImageVolumeRejection',
 'TestContainerProtocolStreamRoundTrip','TestContainerProtocolPinnedResources'}


def require(value,message):
    if not value:raise ValueError(message)


def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def artifact(base,records,name):
    entry=records.get(name);require(isinstance(entry,dict),'missing artifact: '+name)
    path=(base/name).resolve();require(path.is_relative_to(base.resolve()) and path.is_file(),'unsafe/missing artifact: '+name)
    require(digest(path)==entry.get('sha256'),'artifact changed: '+name)
    return path


def merge_profiles(texts):
    blocks={}
    for text in texts:
        lines=text.splitlines();require(lines and lines[0]=='mode: atomic','atomic coverage required')
        for line in lines[1:]:
            key,count,hit=line.rsplit(' ',2);count=int(count);hit=int(hit)
            require(min(count,hit)>=0,'negative coverage')
            if key in blocks:require(blocks[key][0]==count,'inconsistent coverage blocks')
            blocks[key]=(count,max(hit,blocks.get(key,(0,0))[1]))
    return 'mode: atomic\n'+''.join(f'{key} {size} {hit}\n' for key,(size,hit) in sorted(blocks.items()))


def event_audit(raw,inventory,packages,strict):
    checked=strict(raw);require(checked['status']=='passed','strict test stream failed: '+str(checked['errors']))
    events=[json.loads(line) for line in raw.splitlines()]
    expected=set()
    for line in inventory.splitlines():
        event=json.loads(line)
        for name in inventory_names(event.get('Output','')):expected.add((event['Package'],name))
    require(expected,'empty executable test inventory')
    result=check_events(events,expected)
    require(not any(result[k] for k in ['skipped','failed','missing','unfinished','missing_packages']),'test inventory not satisfied')
    require(set(packages.splitlines())==set(result['packages']),'package inventory not satisfied')
    return {event['Test'] for event in events if event.get('Test') and event.get('Action')=='pass'},checked


def audit(evidence):
    base=evidence.resolve().parent;require(not base.is_relative_to(ROOT),'evidence must be outside candidate')
    data=json.loads(evidence.read_text())
    require(data.get('schema_version')==1 and data.get('baseline')=='e6dd26351c1b1436616e1c2b8da0086a3580d882','wrong evidence schema/baseline')
    records={e['path']:e for e in data['artifacts']}
    require(len(records)==len(data['artifacts']),'duplicate artifact paths')
    for name in records:artifact(base,records,name)
    def read(name):return artifact(base,records,name).read_text()
    def obj(name):return json.loads(read(name))
    def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT,text=True,timeout=30).strip()
    commit=git('rev-parse','HEAD');require(commit==data['source_commit'],'wrong candidate commit')
    require(not git('status','--porcelain','--untracked-files=all'),'candidate is not clean')
    require(data['scope']=='2026-09-09-user-amendment','scope amendment identity missing')
    require(data['scope_amendment_sha256']==digest(ROOT/'docs/acceptance/scope-amendment-2026-09-09.md'),'scope amendment changed')
    require(not (ROOT/'go.work').exists(),'workspace override remains')
    env=dict(os.environ,GOWORK='off',GOPROXY='off',GOSUMDB='off',GOTOOLCHAIN='local')
    module=json.loads(subprocess.check_output(['go','list','-m','-json',HARNESS],cwd=ROOT,env=env,text=True,timeout=30))
    require(module.get('Version')=='v0.4.0' and module.get('Sum')==RELEASE_SUM and not module.get('Replace'),'unreleased/replaced Harness')
    subprocess.check_output(['go','mod','verify'],cwd=ROOT,env=env,timeout=60)
    refs=git('ls-remote','https://github.com/sausheong/harness.git','refs/tags/v0.4.0^{}')
    require(refs.split()[0]==RELEASE_COMMIT,'published Harness tag changed')
    harness_root=Path(module['Dir'])
    spec=importlib.util.spec_from_file_location('harness_test_gate',harness_root/'scripts/check_test_events.py')
    gate=importlib.util.module_from_spec(spec);spec.loader.exec_module(gate)
    snapshot=source_snapshot(ROOT);platform_results={};profiles=[];harness_profiles=[]
    for name,goos,log in [('mac','darwin','tests.log'),('linux','linux','suite.log')]:
        report=obj(name+'/run.json')
        require(report['commit_before']==commit==report['commit_after'] and report['clean_before'] and report['clean_after'],name+' candidate was not frozen')
        require(report['platform']['GOOS']==goos and report['platform']['CGO_ENABLED']=='1',name+' wrong runtime')
        if name=='mac':
            require(report['source_before']==snapshot==report['source_after'],name+' source changed')
            effective=report['module']
        else:
            require(report['sources_before'][str(ROOT)]==snapshot==report['sources_after'][str(ROOT)],name+' source changed')
            require(report['sources_before']==report['sources_after'],'dependency source changed')
            effective=report['harness_module']
        require(effective.get('Version')=='v0.4.0' and effective.get('Sum')==RELEASE_SUM and not effective.get('Replace'),name+' wrong dependency')
        require(report['commands'] and all(c['exit_code']==0 for c in report['commands']),name+' failed command')
        inventory_commands=[c for c in report['commands'] if c.get('log')=='inventory.log']
        require(len(inventory_commands)==1 and all(flag in inventory_commands[0]['argv'] for flag in ['-json','-list','^(Test|Example|Fuzz)','./...']),name+' executable inventory command missing')
        test_commands=[c for c in report['commands'] if c.get('log')==log]
        require(len(test_commands)==1 and all(flag in test_commands[0]['argv'] for flag in ['-race','-count=1','-coverpkg=./...','./...']),name+' full uncached race command missing')
        names,stream=event_audit(read(name+'/'+log),read(name+'/inventory.log'),read(name+'/packages.log'),gate.check)
        require(HAND_REQUIRED<=names,name+' retained isolation regression missing: '+str(HAND_REQUIRED-names))
        platform_results[name]=stream
        profiles.append(read(name+'/coverage.out'))
        h='harness/'+name+'/'
        require(read(h+'commit.txt').strip()==RELEASE_COMMIT and not read(h+'status.txt').strip(),'Harness qualification candidate mismatch')
        require(obj(h+'platform.json')['GOOS']==goos,'Harness platform mismatch')
        events=read(h+'tests.jsonl');checked=gate.check(events)
        require(checked['status']=='passed' and checked['passed_test_records']==1114 and checked['packages']==32,'Harness v0.4.0 suite missing/failed/skipped')
        names={e['Test'] for e in map(json.loads,events.splitlines()) if e.get('Test') and e.get('Action')=='pass'}
        require(HARNESS_REQUIRED<=names,'Harness isolation regressions missing')
        repeated=read(h+'critical-repeat.jsonl')
        require(gate.check(repeated)['status']=='passed','Harness lifecycle repetitions failed')
        counts=Counter(e.get('Test') for e in map(json.loads,repeated.splitlines()) if e.get('Test') and e.get('Action')=='pass')
        require(all(counts[n]==20 for n in ['TestCancellationStopsShellDescendant','TestNormalExitStopsShellDescendant','TestArtifactCrossProcessReservation','TestRun_StreamingAbortMidKickoffPairsAllEntries']),'Harness lifecycle repetitions incomplete')
        harness_profiles.append(read(h+'coverage.out'))
    hand_profile=merge_profiles(profiles);harness_profile=merge_profiles(harness_profiles)
    overall=coverage_metrics(hand_profile)['total'];require(overall['percentage']>=80,'overall coverage below 80')
    scope=classify_changed_sources(ROOT,changed_lines(ROOT,data['baseline']),json.loads((ROOT/'docs/acceptance/coverage-scope.json').read_text()))
    changed=changed_coverage(hand_profile,MODULE,scope['production_changes'],ROOT)
    require(not changed['missing_profile_files'] and changed['percentage']>=80,'changed-source coverage incomplete/below 80')
    groups=critical_coverage({MODULE:hand_profile,HARNESS:harness_profile},json.loads((ROOT/'docs/acceptance/critical-coverage-map.json').read_text()),{MODULE:ROOT,HARNESS:harness_root})
    for name,group in groups.items():
        require(not group['missing_profiles'] and not group['unmatched_patterns'] and group['observed_percentage']>=80,name+' coverage incomplete/below 80')
        require(all(p==PENDING_AUDIT for p in group['pending_capabilities']),name+' unresolved capability beyond scoped audit')
    gateway=obj('gateway/report.json');linux=obj('linux/run.json')
    require(gateway['status']=='passed' and gateway['source_unchanged'],'gateway probe failed/source changed')
    require(gateway['binary_sha256']==linux['binaries']['hand'],'gateway used another Hand binary')
    require('vcs.revision='+commit in gateway['module_metadata'] and 'vcs.modified=false' in gateway['module_metadata'],'gateway binary is not stamped with clean candidate identity')
    require(gateway['runner_sha256']==digest(ROOT/'scripts/check_evaluation_isolation.py') and gateway['fixture_sha256']==digest(ROOT/'scripts/evaluation_isolation_fixture.py'),'gateway runner/fixture changed')
    require(gateway['gateway_source_hashes']=={p.name:digest(p) for p in sorted((ROOT/'scripts').glob('evaluation_*.py'))},'gateway source mismatch')
    for who in ['agent_container','gateway_container']:
        config=gateway[who]['HostConfig']
        require(config['NetworkMode']=='none' and config['ReadonlyRootfs'] and not config['Privileged'] and config['CapDrop']==['ALL'] and 'no-new-privileges:true' in config['SecurityOpt'],'container boundary missing')
    agent=gateway['agent'];require(agent['passed'] and agent['proxy_joined'] and agent['hand_completed'] and agent['actual_tool_marker_read'],'actual Hand turn incomplete')
    require(all(agent['gateway_rejections'].values()) and len(agent['gateway_rejections'])==4,'gateway bypass probe missing')
    require(all(agent['external_connections_denied'].values()) and len(agent['external_connections_denied'])==2 and gateway['outside_sentinel_live'],'network denial/control missing')
    require(agent['no_routable_external_interface'] and agent['provider_secret_absent'] and all(agent['inaccessible_host_and_gateway_paths'].values()) and gateway['provider_secret_absent_from_agent_files'],'credential/filesystem isolation missing')
    upstream=gateway['gateway'];require(upstream['joined'] and not upstream['failures'] and upstream['requests_charged']==2 and len(upstream['requests'])==2 and all(r['provider_credential_substituted'] for r in upstream['requests']) and upstream['requests'][1]['tool_marker_seen'],'gateway admission/accounting mismatch')
    require(len(gateway['cleanup'])==3 and all(c['removed'] and c['absent'] for c in gateway['cleanup']) and gateway['volume_removed'],'owned-resource cleanup incomplete')
    review=obj('review.json');require(review['source_commit']==commit and review['open_defects']==[] and review['status']=='passed','final review incomplete')
    manifest=json.loads((ROOT/'docs/acceptance/requirements.json').read_text())
    scenarios=next(r['scenarios'] for r in manifest['requirements'] if r['id']=='M5.2')
    require(set(review['isolation_scenarios'])==set(scenarios) and all(v=='passed' for v in review['isolation_scenarios'].values()),'retained scenario audit incomplete')
    require(review['migration_rollback_reviewed'] and review['host_mcp_extension_boundaries_reviewed'],'migration/boundary review missing')
    original=obj('original-checker.json');require(original['source_commit']==commit and original['status']=='not_complete','original full-checker output missing/unexpected')
    return dict(status='complete',scope=data['scope'],source_commit=commit,harness_version='v0.4.0',
        platform_results=platform_results,overall_coverage=overall,changed_coverage={k:changed[k] for k in ['statements','covered','percentage','missing_profile_files']},
        critical_coverage={k:v['observed_percentage'] for k,v in groups.items()},retained_isolation_scenarios=len(scenarios),
        original_full_checker_status=original['status'],original_full_plan_complete=False,paid_provider_calls=0,pi_superiority_claim=False)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--evidence',type=Path,required=True)
    args=parser.parse_args()
    try:result=audit(args.evidence)
    except (OSError,ValueError,KeyError,TypeError,subprocess.SubprocessError) as error:result=dict(status='not_complete',error=str(error))
    print(json.dumps(result,indent=2));raise SystemExit(result['status']!='complete')

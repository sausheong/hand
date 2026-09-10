"""Probe installed Pi AI via loopback HTTP gateway and socket-pair upstream."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import threading

from evaluation_budget import EvaluationBudget
from evaluation_gateway import GatewayController
from evaluation_http_server import GatewayHTTPServer
from evaluation_anthropic_usage import verify_observed_usage


def check(runtime,output,client="pi"):
    if client not in ("pi","harness","pi-agent","hand-agent"):raise ValueError("known probe client required")
    runtime=Path(runtime).resolve();output=Path(output).resolve();output.mkdir(mode=0o700)
    evidence=output/'requests';evidence.mkdir(mode=0o700)
    path=output/'budget.sqlite';manifest='a'*64
    ledger=EvaluationBudget(path,manifest,100000000)
    try:ledger.add_run('probe',5,100000,25000,100000000,60)
    finally:ledger.close()
    token='offline_probe_'+'x'*32
    model='claude-sonnet-4-5-20250929'
    contract=dict(schema_version=1,provider='anthropic',model=model,source_sha256='b'*64,billing='text_tokens_with_partitioned_cache',context_tokens=10000,max_output_tokens=4096,max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
    runs=[dict(id='probe',token=token,model=model,contract=contract,allowed_beta_sets=[[],['interleaved-thinking-2025-05-14']],timeout_seconds=5)]
    threads=[];incoming=[];upstream=[];errors=[]
    def factory(timeout):
        client_socket,server=socket.socketpair();client_socket.settimeout(timeout);server.settimeout(5)
        def serve():
            try:
                with server.makefile('rb') as stream:
                    first=stream.readline();headers={}
                    while True:
                        line=stream.readline()
                        if line in (b'\r\n',b''):break
                        key,value=line.decode().split(':',1);headers[key.lower()]=value.strip()
                    raw=stream.read(int(headers['content-length']));request=json.loads(raw)
                    upstream.append(dict(method_target=first.decode().strip(),body_sha256=hashlib.sha256(raw).hexdigest(),model=request['model']))
                    events=[dict(type='message_start',message=dict(id='msg_fixture',type='message',role='assistant',model=request['model'],content=[],stop_reason=None,stop_sequence=None,usage=dict(input_tokens=5,output_tokens=0,cache_creation_input_tokens=0,cache_read_input_tokens=0))),
                        dict(type='content_block_start',index=0,content_block=dict(type='text',text='')),
                        dict(type='content_block_delta',index=0,delta=dict(type='text_delta',text='offline gateway fixture')),
                        dict(type='content_block_stop',index=0),dict(type='message_delta',delta=dict(stop_reason='end_turn',stop_sequence=None),usage=dict(output_tokens=2)),dict(type='message_stop')]
                    if client in ('pi-agent','hand-agent'):
                        if len(upstream)==1:
                            events[1]['content_block']=dict(type='tool_use',id='fixture-read',name='read' if client=='pi-agent' else 'read_file',input={})
                            events[2]['delta']=dict(type='input_json_delta',partial_json=json.dumps({'path':'example.go'}))
                            events[4]['delta']['stop_reason']='tool_use'
                        else:
                            marker='HAND_GATEWAY_READ_PROBE' in json.dumps(request['messages'])
                            upstream[-1]['tool_result_marker_seen']=marker
                            if not marker:raise ValueError('tool result missing from second provider request')
                    response=''.join('event: '+e['type']+'\ndata: '+json.dumps(e)+'\n\n' for e in events).encode()
                    server.sendall(b'HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: '+str(len(response)).encode()+b'\r\n\r\n'+response)
            except Exception as error:errors.append(type(error).__name__)
            finally:server.close()
        thread=threading.Thread(target=serve);thread.start();threads.append(thread)
        connection=http.client.HTTPConnection('api.anthropic.com',timeout=timeout);connection.connect=lambda:setattr(connection,'sock',client_socket);return connection
    class CaptureController(GatewayController):
        def preflight(self,pairs,body_length):
            incoming.append(dict(headers=[(k,v) for k,v in pairs if k.lower() not in ('x-api-key','authorization')],body_length=body_length))
            return super().preflight(pairs,body_length)
    server=GatewayHTTPServer(('127.0.0.1',0),None,intake_timeout=2,io_timeout=2)
    authority='127.0.0.1:'+str(server.server_address[1])
    controller=CaptureController(path,manifest,evidence,runs,'offline-provider-key',authority,connection_factory=factory)
    server.controller=controller
    serving=threading.Thread(target=lambda:server.serve_forever(poll_interval=0.01));serving.start()
    config=output/'config.json';config.write_text(json.dumps(dict(url='http://'+authority,token=token)))
    home=output/'home';home.mkdir()
    node=shutil.which('node');probe=Path(__file__).with_suffix('.mjs').resolve()
    report=dict(status='running',client=client,limitations=['Real provider SDK and loopback gateway only; synthetic upstream responses/prices and no provider/network-isolation/live-comparison claim.'])
    try:
        if client=='pi':
            argv=[node,str(probe),str(runtime),str(config),str(output/'sdk-report.json')]
            env={'PATH':str(Path(node).parent)+':/usr/bin:/bin','HOME':str(home)}
        elif client in ('pi-agent','hand-agent'):
            probe=Path(__file__).with_name('check_pi_agent_gateway.py' if client=='pi-agent' else 'check_hand_agent_gateway.py').resolve()
            argv=[sys.executable,str(probe),str(runtime),str(config),str(output/'sdk-report.json')]
            env={'PATH':os.environ['PATH'],'HOME':str(home),'PYTHONDONTWRITEBYTECODE':'1'}
        else:
            go=shutil.which('go')
            module_cache=subprocess.check_output([go,'env','GOMODCACHE'],text=True).strip()
            helper=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/harness_gateway.go'
            argv=[go,'run',str(helper),str(config),str(output/'sdk-report.json')]
            env={'PATH':os.environ['PATH'],'HOME':str(home),'GOMODCACHE':module_cache,
                 'GOCACHE':'/private/tmp/hand-review-gocache','GOPROXY':'off','GOTOOLCHAIN':'local'}
        report['argv']=argv
        result=subprocess.run(argv,env=env,capture_output=True,timeout=45)
        (output/'stdout.log').write_bytes(result.stdout);(output/'stderr.log').write_bytes(result.stderr)
        report['exit_code']=result.returncode;report['status']='sdk_gateway_passed' if result.returncode==0 else 'failed'
    finally:
        controller.close();server.shutdown();server.server_close();serving.join(2)
        for thread in threads:thread.join(2)
        report.update(server_joined=not serving.is_alive(),upstream_threads_joined=all(not t.is_alive() for t in threads),incoming=incoming,upstream=upstream,errors=errors)
        ledger=EvaluationBudget(path,manifest)
        try:report['budget_totals']=ledger.totals()
        finally:ledger.close()
        report['usage_observations']=[]
        for item in evidence.glob('*/report.json'):
            item=json.loads(item.read_text())
            if 'observation' in item:
                report['usage_observations'].append(verify_observed_usage(item['observation'],Path(item['response_file']).read_bytes(),model))
        expected = 2 if client in ('pi-agent', 'hand-agent') else 3
        if (not report['server_joined'] or not report['upstream_threads_joined'] or errors
                or len(upstream) != expected or report['budget_totals']['requests'] != expected
                or report['budget_totals']['uncertain'] != expected
                or len(report['usage_observations']) != expected
                or any(o['status'] != 'usage_observed_unverified' for o in report['usage_observations'])):
            report['status'] = 'failed'
        (output/'report.json').write_text(json.dumps(report,indent=2)+'\n')
    return report


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--runtime',required=True);parser.add_argument('--output',required=True);parser.add_argument('--client',choices=['pi','harness','pi-agent','hand-agent'],default='pi');args=parser.parse_args()
    result=check(args.runtime,args.output,args.client);print(json.dumps({k:result[k] for k in ('status','exit_code','budget_totals')}))
    raise SystemExit(0 if result['status']=='sdk_gateway_passed' else 1)

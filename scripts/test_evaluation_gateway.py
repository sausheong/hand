import hashlib
import http.client
import json
from pathlib import Path
import socket
import tempfile
import threading
import time
import unittest

from evaluation_budget import EvaluationBudget
from evaluation_gateway import GatewayController


class GatewayTests(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        self.root=Path(temp.name).resolve();self.path=self.root/'budget.sqlite';self.evidence=self.root/'evidence';self.evidence.mkdir(mode=0o700)
        ledger=EvaluationBudget(self.path,'a'*64,30000)
        try:ledger.add_run('run',5,100,20,30000,60)
        finally:ledger.close()
        self.token='fixture_'+'x'*32
        self.contract=dict(schema_version=1,provider='anthropic',model='fixture',source_sha256='b'*64,billing='text_tokens_with_partitioned_cache',context_tokens=10,max_output_tokens=4,max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
        self.runs=[dict(id='run',token=self.token,model='fixture',contract=self.contract,allowed_beta_sets=[[]],timeout_seconds=1)]
        self.body=b'{"model":"fixture","max_tokens":4,"messages":[{"role":"user","content":"code"}]}'
        self.headers=[('Host','gateway'),('Content-Type','application/json'),('Content-Length',str(len(self.body))),('Anthropic-Version','2023-06-01'),('X-Api-Key',self.token)]
        self.received=threading.Event();self.release=threading.Event();self.servers=[];self.requests=[]
        self.addCleanup(self.join_servers)

    def join_servers(self):
        self.release.set()
        for thread in self.servers:thread.join(3);self.assertFalse(thread.is_alive())

    def factory(self,stall=False):
        def create(timeout):
            client,server=socket.socketpair();client.settimeout(timeout);server.settimeout(2)
            def serve():
                try:
                    with server.makefile('rb') as stream:
                        first=stream.readline();headers={}
                        while True:
                            line=stream.readline()
                            if line in (b'\r\n',b''):break
                            k,v=line.decode().split(':',1);headers[k.lower()]=v.strip()
                        body=stream.read(int(headers['content-length']));self.requests.append((first,headers,body));self.received.set()
                        if stall:self.release.wait(2)
                        server.sendall(b'HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\ndone')
                except (BrokenPipeError,ConnectionResetError):pass
                finally:server.close()
            thread=threading.Thread(target=serve);thread.start();self.servers.append(thread)
            connection=http.client.HTTPConnection('api.anthropic.com',timeout=timeout);connection.connect=lambda:setattr(connection,'sock',client)
            return connection
        return create

    def controller(self,stall=False):
        controller=GatewayController(self.path,'a'*64,self.evidence,self.runs,'provider-fixture-key','gateway',connection_factory=self.factory(stall))
        self.addCleanup(controller.close);return controller

    def totals(self):
        ledger=EvaluationBudget(self.path,'a'*64)
        try:return ledger.totals()
        finally:ledger.close()

    def test_request_uses_frozen_settings_and_private_evidence(self):
        controller=self.controller();self.runs[0]['model']='caller-change';self.contract['price_sets'][0]['input']='999'
        chunks=[];report=controller.handle('POST','/v1/messages',self.headers,self.body,chunks.append)
        self.assertEqual(report['status'],'response_received');self.assertEqual(b''.join(chunks),b'done')
        raw=Path(report['response_file']).read_bytes();self.assertEqual(raw,b'done')
        self.assertEqual(report['response_file_sha256'],hashlib.sha256(raw).hexdigest())
        self.assertEqual(json.loads((Path(report['response_file']).parent/'report.json').read_text()),report)
        self.assertEqual(self.requests[0][2],self.body)
        for secret in [self.token,'provider-fixture-key']:self.assertNotIn(secret,json.dumps(report))
        self.assertEqual(self.totals()['cost_nanos'],18000)

    def test_unknown_credential_and_body_model_never_connect(self):
        controller=self.controller()
        headers=[(k,'unknown' if k=='X-Api-Key' else v) for k,v in self.headers]
        with self.assertRaises(ValueError):controller.handle('POST','/v1/messages',headers,self.body)
        bad=self.body.replace(b'fixture',b'changed')
        with self.assertRaises(ValueError):controller.handle('POST','/v1/messages',self.headers,bad)
        self.assertEqual(self.requests,[]);self.assertEqual(self.totals()['requests'],0)

    def test_revocation_interrupts_active_socket_and_prevents_new_work(self):
        controller=self.controller(stall=True);reports=[]
        worker=threading.Thread(target=lambda:reports.append(controller.handle('POST','/v1/messages',self.headers,self.body)))
        worker.start();self.assertTrue(self.received.wait(2))
        started=time.monotonic();controller.revoke('run');worker.join(2)
        self.assertFalse(worker.is_alive());self.assertLess(time.monotonic()-started,1)
        self.assertEqual(reports[0]['status'],'incomplete');self.assertEqual(controller.active,{})
        with self.assertRaises(ValueError):controller.handle('POST','/v1/messages',self.headers,self.body)
        self.assertEqual(self.totals()['cost_nanos'],18000);self.assertEqual(len(self.requests),1)

    def test_concurrent_requests_share_one_remaining_budget(self):
        controller=self.controller();barrier=threading.Barrier(2);results=[]
        def request():
            barrier.wait(2)
            try:results.append(controller.handle('POST','/v1/messages',self.headers,self.body)['status'])
            except ValueError:results.append('denied')
        workers=[threading.Thread(target=request) for _ in range(2)]
        for worker in workers:worker.start()
        for worker in workers:worker.join(3);self.assertFalse(worker.is_alive())
        self.assertCountEqual(results,['response_received','denied']);self.assertEqual(len(self.requests),1)
        self.assertEqual(self.totals()['requests'],1)

    def test_close_cancels_active_transport_and_rejects_requests(self):
        controller=self.controller(stall=True);reports=[]
        worker=threading.Thread(target=lambda:reports.append(controller.handle('POST','/v1/messages',self.headers,self.body)))
        worker.start();self.assertTrue(self.received.wait(2));controller.close();worker.join(2)
        self.assertFalse(worker.is_alive());self.assertEqual(reports[0]['status'],'incomplete')
        with self.assertRaises(ValueError):controller.handle('POST','/v1/messages',self.headers,self.body)
        self.assertEqual(controller.active,{})


if __name__ == '__main__':unittest.main()

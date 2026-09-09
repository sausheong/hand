import http.client
from pathlib import Path
import socket
import tempfile
import threading
import types
import unittest

from evaluation_budget import EvaluationBudget
from evaluation_gateway import GatewayController
from evaluation_http_server import GatewayHandler


class HTTPServerTests(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        root=Path(temp.name).resolve();self.path=root/'budget.sqlite';evidence=root/'evidence';evidence.mkdir(mode=0o700)
        ledger=EvaluationBudget(self.path,'a'*64,100000)
        try:ledger.add_run('run',5,100,100,100000,60)
        finally:ledger.close()
        self.body=b'{"model":"fixture","max_tokens":4,"messages":[{"role":"user","content":"code"}]}'
        self.token='fixture_'+'x'*32;self.threads=[];self.errors=[];self.calls=0;self.release=threading.Event()
        self.response=b'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 4\r\n\r\ndone'
        self.streaming=False
        contract=dict(schema_version=1,provider='anthropic',model='fixture',source_sha256='b'*64,billing='text_tokens_with_partitioned_cache',context_tokens=10,max_output_tokens=4,max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
        runs=[dict(id='run',token=self.token,model='fixture',contract=contract,allowed_beta_sets=[[]],timeout_seconds=2)]
        self.controller=GatewayController(self.path,'a'*64,evidence,runs,'provider-fixture-key','gateway',connection_factory=self.factory)
        self.server=types.SimpleNamespace(controller=self.controller,intake_timeout=0.2,io_timeout=1)
        self.addCleanup(self.cleanup)

    def cleanup(self):
        self.release.set();self.controller.close()
        for thread in self.threads:thread.join(3);self.assertFalse(thread.is_alive())
        self.assertEqual(self.errors,[])

    def factory(self,timeout):
        self.calls+=1;client,server=socket.socketpair();client.settimeout(timeout);server.settimeout(2)
        def upstream():
            try:
                with server.makefile('rb') as stream:
                    stream.readline();headers={}
                    while True:
                        line=stream.readline()
                        if line in (b'\r\n',b''):break
                        key,value=line.decode().split(':',1);headers[key.lower()]=value.strip()
                    stream.read(int(headers['content-length']))
                    if self.streaming:
                        server.sendall(b'HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n')
                        if not self.release.wait(2):raise TimeoutError('test did not release stream')
                        server.sendall(b'0\r\n\r\n')
                    else:server.sendall(self.response)
            except Exception as error:self.errors.append(type(error).__name__)
            finally:server.close()
        thread=threading.Thread(target=upstream);thread.start();self.threads.append(thread)
        connection=http.client.HTTPConnection('api.anthropic.com',timeout=timeout);connection.connect=lambda:setattr(connection,'sock',client)
        return connection

    def incoming(self,raw):
        client,server=socket.socketpair();client.settimeout(2)
        def serve():
            try:GatewayHandler(server,('local',0),self.server)
            except OSError:pass
            except Exception as error:self.errors.append(type(error).__name__)
            finally:server.close()
        thread=threading.Thread(target=serve);thread.start();self.threads.append(thread)
        self.addCleanup(client.close);client.sendall(raw)
        return client,thread

    def request(self,extra=b'',body=None):
        body=self.body if body is None else body
        return b'POST /v1/messages HTTP/1.1\r\nHost: gateway\r\nContent-Type: application/json\r\nAnthropic-Version: 2023-06-01\r\nX-Api-Key: '+self.token.encode()+b'\r\nContent-Length: '+str(len(body)).encode()+b'\r\n'+extra+b'\r\n'+body

    def test_complete_http_request_streams_response_and_keeps_accounting(self):
        client,thread=self.incoming(self.request());response=http.client.HTTPResponse(client);response.begin()
        self.addCleanup(response.close);self.assertEqual(response.status,200);self.assertEqual(response.read(),b'done')
        thread.join(2);self.assertFalse(thread.is_alive());self.assertEqual(self.calls,1)
        ledger=EvaluationBudget(self.path,'a'*64)
        try:self.assertEqual(ledger.totals()['cost_nanos'],18000);self.assertEqual(ledger.totals()['uncertain'],1)
        finally:ledger.close()

    def test_first_chunk_arrives_before_upstream_finishes(self):
        self.streaming=True
        client,_=self.incoming(self.request());response=http.client.HTTPResponse(client);response.begin();self.addCleanup(response.close)
        self.assertEqual(response.read(5),b'hello');self.assertFalse(self.release.is_set())
        self.release.set();self.assertEqual(response.read(),b'')

    def test_truncated_upstream_has_no_successful_terminal_chunk(self):
        self.response=b'HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nshort'
        client,_=self.incoming(self.request());response=http.client.HTTPResponse(client);response.begin();self.addCleanup(response.close)
        with self.assertRaises(http.client.IncompleteRead):response.read()

    def test_duplicate_length_rejected_before_upstream(self):
        client,_=self.incoming(self.request(extra=b'content-length: 10\r\n'));response=http.client.HTTPResponse(client);response.begin();self.addCleanup(response.close)
        self.assertEqual(response.status,400);response.read();self.assertEqual(self.calls,0)

    def test_slow_header_intake_has_absolute_deadline(self):
        client,thread=self.incoming(b'POST /v1/messages HTTP/1.1\r\nHost:')
        thread.join(1);self.assertFalse(thread.is_alive());self.assertEqual(self.calls,0)
        self.assertEqual(client.recv(1),b'')


if __name__ == '__main__':unittest.main()

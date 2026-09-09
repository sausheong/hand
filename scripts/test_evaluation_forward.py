import http.client
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

from evaluation_budget import EvaluationBudget
from evaluation_forward import forward, https_connection
from evaluation_http_policy import admit_http_request


class ForwardTests(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        self.path=Path(temp.name).resolve()/'budget.sqlite'
        self.ledger=EvaluationBudget(self.path,'a'*64,100000,clock=lambda:100);self.addCleanup(self.ledger.close)
        self.ledger.add_run('run',5,100,100,100000,60)
        self.body=b'{"model":"fixture","max_tokens":4,"messages":[{"role":"user","content":"code"}]}'
        token='run_'+'x'*32
        headers=[('Host','gateway'),('Content-Length',str(len(self.body))),('Content-Type','application/json'),('Anthropic-Version','2023-06-01'),('X-Api-Key',token)]
        contract=dict(schema_version=1,provider='anthropic',model='fixture',source_sha256='b'*64,billing='text_tokens_with_partitioned_cache',context_tokens=10,max_output_tokens=4,max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
        admit_http_request(self.ledger,'run','request','POST','/v1/messages',headers,self.body,token,'gateway',[[]],contract,'fixture')

    def connection(self, response, stall=False):
        captured=[];received=threading.Event();release=threading.Event();threads=[]
        def factory(timeout):
            self.assertEqual(self.ledger.db.execute('SELECT dispatched FROM transport_admissions').fetchone()[0],1)
            client,server=socket.socketpair();client.settimeout(timeout);server.settimeout(2)
            def serve():
                try:
                    with server.makefile('rb') as stream:
                        first=stream.readline();headers={}
                        while True:
                            line=stream.readline()
                            if line in (b'\r\n',b''):break
                            key,value=line.decode().split(':',1);headers[key.lower()]=value.strip()
                        body=stream.read(int(headers['content-length']))
                        captured.append((first,headers,body));received.set()
                        if stall:release.wait(2)
                        if response:server.sendall(response)
                except (BrokenPipeError,ConnectionResetError):pass
                finally:server.close()
            thread=threading.Thread(target=serve);thread.start();threads.append(thread)
            conn=http.client.HTTPConnection('api.anthropic.com',timeout=timeout)
            conn.connect=lambda:setattr(conn,'sock',client)
            return conn
        def cleanup():
            release.set()
            for thread in threads:thread.join(3);self.assertFalse(thread.is_alive())
        self.addCleanup(cleanup)
        return factory,captured,received

    def send(self,factory,**kwargs):
        chunks=[]
        result=forward(self.ledger,'run','request',self.body,'provider-fixture-key',chunks.append,
            timeout_seconds=kwargs.pop('timeout_seconds',1),connection_factory=factory,**kwargs)
        return result,b''.join(chunks)

    def test_socket_http_preserves_body_and_replaces_only_gateway_key(self):
        factory,captured,_=self.connection(b'HTTP/1.1 200 OK\r\nContent-Length: 4\r\nContent-Type: application/json\r\n\r\ndone')
        result,body=self.send(factory)
        self.assertEqual(result['status'],'response_received');self.assertEqual(body,b'done')
        self.assertEqual(captured[0][2],self.body)
        self.assertEqual(captured[0][1]['host'],'api.anthropic.com')
        self.assertEqual(captured[0][1]['x-api-key'],'provider-fixture-key')
        self.assertTrue(result['watcher_joined']);self.assertNotIn('provider-fixture-key',json.dumps(result))
        self.assertEqual(self.ledger.totals()['cost_nanos'],18000)
        self.assertEqual(self.ledger.totals()['uncertain'],1)
        with self.assertRaises(ValueError):self.send(factory)
        self.assertEqual(len(captured),1)

    def test_redirect_is_observed_without_following_location(self):
        factory,captured,_=self.connection(b'HTTP/1.1 307 Temporary Redirect\r\nLocation: https://other.invalid\r\nContent-Length: 0\r\n\r\n')
        result,_=self.send(factory)
        self.assertEqual(result['http_status'],307);self.assertEqual(len(captured),1)
        self.assertNotIn('location',result['response_headers'])

    def test_socket_observation_binds_usage_without_releasing_budget(self):
        from evaluation_anthropic_usage import verify_observed_usage
        message=dict(type='message',id='msg_fixture',role='assistant',model='fixture',
            content=[dict(type='text',text='done')],stop_reason='end_turn',
            usage=dict(input_tokens=3,output_tokens=2,cache_creation_input_tokens=0,cache_read_input_tokens=0))
        raw=json.dumps(message).encode()
        wire=b'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw
        factory,_,_=self.connection(wire)
        observation,body=self.send(factory)
        usage=verify_observed_usage(observation,body,'fixture')
        self.assertEqual(usage['status'],'usage_observed_unverified')
        self.assertEqual((usage['input_tokens'],usage['output_tokens']),(3,2))
        self.assertEqual(self.ledger.totals()['cost_nanos'],18000)
        self.assertEqual(self.ledger.totals()['uncertain'],1)

    def test_truncated_response_retains_uncertain_reservation(self):
        factory,_,_=self.connection(b'HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nshort')
        result,_=self.send(factory)
        self.assertEqual(result['status'],'incomplete');self.assertEqual(result['error_type'],'ValueError')
        self.assertEqual(self.ledger.totals()['cost_nanos'],18000)

    def test_response_cap_and_header_deadline_stop_transport(self):
        factory,_,_=self.connection(b'HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nlong')
        result,_=self.send(factory,max_response_bytes=3)
        self.assertEqual(result['status'],'incomplete');self.assertEqual(result['error_type'],'ValueError')

    def test_stalled_headers_cancel_and_join(self):
        factory,_,received=self.connection(b'',stall=True);cancel=threading.Event()
        def stop():
            received.wait(1);cancel.set()
        worker=threading.Thread(target=stop);worker.start()
        started=time.monotonic();result,_=self.send(factory,cancel=cancel)
        worker.join(2);self.assertFalse(worker.is_alive())
        self.assertLess(time.monotonic()-started,1);self.assertEqual(result['status'],'incomplete');self.assertTrue(result['watcher_joined'])

    def test_stalled_headers_obey_absolute_deadline(self):
        factory,_,_=self.connection(b'',stall=True)
        started=time.monotonic();result,_=self.send(factory,timeout_seconds=0.05)
        self.assertLess(time.monotonic()-started,0.5);self.assertEqual(result['status'],'incomplete')

    def test_bad_body_and_revocation_prevent_connection(self):
        calls=[]
        def factory(timeout):calls.append(timeout);raise AssertionError('must not connect')
        with self.assertRaises(ValueError):
            forward(self.ledger,'run','request',b'other','provider-key',lambda chunk:None,timeout_seconds=1,connection_factory=factory)
        self.ledger.revoke('run')
        with self.assertRaises(ValueError):self.send(factory)
        self.assertEqual(calls,[])

    def test_crash_after_claim_cannot_dispatch_on_restart(self):
        code="import os,sys; from evaluation_budget import EvaluationBudget; ledger=EvaluationBudget(sys.argv[1],'a'*64,clock=lambda:100); ledger.claim_dispatch('run','request'); os._exit(77)"
        child=subprocess.run([sys.executable,'-c',code,str(self.path)],cwd=Path(__file__).parent,
            env={'PATH':os.environ['PATH'],'PYTHONDONTWRITEBYTECODE':'1'},timeout=10)
        self.assertEqual(child.returncode,77)
        reopened=EvaluationBudget(self.path,'a'*64,clock=lambda:100);self.addCleanup(reopened.close)
        with self.assertRaises(ValueError):reopened.claim_dispatch('run','request')
        self.assertEqual(reopened.totals()['cost_nanos'],18000)

    def test_default_connection_has_fixed_host_and_verified_tls(self):
        import ssl
        with patch('evaluation_forward.http.client.HTTPSConnection') as create:
            https_connection(2)
        args,kwargs=create.call_args
        self.assertEqual(args,('api.anthropic.com',));self.assertEqual(kwargs['timeout'],2)
        self.assertEqual(kwargs['context'].verify_mode,ssl.CERT_REQUIRED)
        self.assertTrue(kwargs['context'].check_hostname)


if __name__ == '__main__':unittest.main()

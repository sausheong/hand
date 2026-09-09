"""Real loopback listener tests; upstream is a local socket-pair fixture."""
import http.client
import socket
import threading
import unittest

import test_evaluation_gateway as gateway_fixtures
from evaluation_gateway import GatewayController
from evaluation_http_server import GatewayHTTPServer


class ListenerTests(unittest.TestCase):
    def setUp(self):
        # Reuse the existing upstream fixture without inheriting/duplicating its
        # test cases. All fixture cleanups are explicitly registered here.
        self.fixture=gateway_fixtures.GatewayTests()
        self.fixture.setUp();self.addCleanup(lambda:self.assertTrue(self.fixture.doCleanups(),'upstream fixture cleanup failed'))
        self.server=GatewayHTTPServer(('127.0.0.1',0),None,max_connections=1,intake_timeout=0.3,io_timeout=0.3)
        authority='127.0.0.1:'+str(self.server.server_address[1])
        self.controller=GatewayController(self.fixture.path,'a'*64,self.fixture.evidence,
            self.fixture.runs,'provider-fixture-key',authority,connection_factory=self.fixture.factory())
        self.server.controller=self.controller
        self.entered=threading.Event();self.worker_starts=[]
        original=self.server.process_request_thread
        def observed(*args):
            self.worker_starts.append(threading.get_ident());self.entered.set();original(*args)
        self.server.process_request_thread=observed
        self.serving=threading.Thread(target=lambda:self.server.serve_forever(poll_interval=0.01))
        self.serving.start();self.addCleanup(self.stop)

    def stop(self):
        self.controller.close();self.server.shutdown();self.server.server_close();self.serving.join(2)
        self.assertFalse(self.serving.is_alive())
        # Every worker slot must have returned, including failed/cancelled intake.
        self.assertTrue(self.server.slots.acquire(blocking=False));self.server.slots.release()

    def test_real_tcp_request_and_response(self):
        connection=http.client.HTTPConnection(*self.server.server_address,timeout=2)
        self.addCleanup(connection.close)
        headers={k:v for k,v in self.fixture.headers if k not in ('Host','Content-Length')}
        connection.request('POST','/v1/messages',body=self.fixture.body,headers=headers)
        response=connection.getresponse();self.assertEqual(response.status,200);self.assertEqual(response.read(),b'done')
        self.assertEqual(len(self.fixture.requests),1)
        self.assertEqual(self.fixture.requests[0][2],self.fixture.body)
        self.assertEqual(self.fixture.totals()['cost_nanos'],18000)

    def test_connection_limit_refuses_extra_socket_without_dispatch(self):
        first=socket.create_connection(self.server.server_address,timeout=2);self.addCleanup(first.close)
        first.sendall(b'POST /v1/messages HTTP/1.1\r\n')
        self.assertTrue(self.entered.wait(1))
        second=socket.create_connection(self.server.server_address,timeout=2);self.addCleanup(second.close)
        try:result=second.recv(1)
        except ConnectionResetError:result=b''
        self.assertEqual(result,b'');self.assertEqual(self.fixture.requests,[])
        self.assertEqual(len(self.worker_starts),1,'extra socket must not start another handler')
        first.close()

    def test_shutdown_joins_stalled_intake(self):
        client=socket.create_connection(self.server.server_address,timeout=2);self.addCleanup(client.close)
        client.sendall(b'POST /v1/messages HTTP/1.1\r\n')
        self.assertTrue(self.entered.wait(1))
        self.controller.close();self.server.shutdown();self.server.server_close();self.serving.join(2)
        self.assertFalse(self.serving.is_alive());self.assertEqual(self.fixture.requests,[])


if __name__ == '__main__':unittest.main()

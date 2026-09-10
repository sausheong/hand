"""Bounded HTTP/1 gateway listener and streamed response bridge.

Use only within the separately qualified agent/gateway network sandbox. No CLI,
spending approval or credential discovery is provided. The owner must close the
controller, stop serving, close the server and join its serving thread.
"""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import socket
import threading


class GatewayHandler(BaseHTTPRequestHandler):
    protocol_version='HTTP/1.1'
    server_version='HandEvaluationGateway'
    sys_version=''

    def setup(self):
        self.request.settimeout(self.server.io_timeout)
        super().setup()
        self.intake_done=threading.Event()
        def expire():
            if not self.intake_done.wait(self.server.intake_timeout):
                try:self.connection.shutdown(socket.SHUT_RDWR)
                except OSError:pass
        self.intake_watch=threading.Thread(target=expire,daemon=True);self.intake_watch.start()

    def stop_intake_watch(self):
        self.intake_done.set();self.intake_watch.join(timeout=1)

    def finish(self):
        self.stop_intake_watch()
        super().finish()

    def handle(self):
        # One request per socket: no pipelined ambiguity or unbounded keepalive.
        self.close_connection=True
        self.handle_one_request()

    def log_message(self,*args):
        pass  # Request targets/headers must never enter default access logs.

    def send_error(self,code,message=None,explain=None):
        self.close_connection=True
        raw=json.dumps({'type':'error','error':{'type':'gateway_error','message':'Request rejected by evaluation gateway'}}).encode()
        try:
            self.send_response_only(code)
            self.send_header('Content-Type','application/json')
            self.send_header('Content-Length',str(len(raw)))
            self.send_header('Connection','close');self.end_headers()
            self.wfile.write(raw)
        except OSError:pass

    def handle_expect_100(self):
        self.send_error(417)
        return False

    def do_POST(self):
        self.close_connection=True
        started=False
        try:
            if self.request_version!='HTTP/1.1':raise ValueError('HTTP/1.1 required')
            if self.path not in ('/v1/messages','/v1/messages?beta=true'):raise ValueError('unsupported route')
            pairs=list(self.headers.raw_items())
            lengths=[value for key,value in pairs if key.lower()=='content-length']
            if len(lengths)!=1 or not lengths[0].isascii() or not lengths[0].isdecimal() or len(lengths[0])>8:
                raise ValueError('unambiguous request length required')
            length=int(lengths[0])
            self.server.controller.preflight(pairs,length)
            body=self.rfile.read(length)
            if len(body)!=length:raise ValueError('truncated request body')
            self.stop_intake_watch()
            def begin(status,headers):
                nonlocal started
                for key,value in headers.items():
                    if key not in ('content-type','content-encoding','request-id') or any(ord(c)<32 or ord(c)>126 for c in value):
                        raise ValueError('invalid upstream response headers')
                self.send_response_only(status)
                for key,value in headers.items():self.send_header(key,value)
                self.send_header('Transfer-Encoding','chunked')
                self.send_header('Connection','close');self.end_headers();started=True
            def chunk(raw):
                if not started:raise ValueError('upstream body before response headers')
                self.wfile.write(format(len(raw),'x').encode()+b'\r\n'+raw+b'\r\n')
            report=self.server.controller.handle('POST',self.path,pairs,body,sink=chunk,on_response=begin)
            if report['status']=='response_received' and started:
                self.wfile.write(b'0\r\n\r\n')
            elif not started:self.send_error(502)
            # Incomplete upstream after headers: deliberately omit terminal chunk
            # and close, so the client cannot mistake partial content for success.
        except (ValueError,OSError):
            if not started:self.send_error(400)
        finally:
            self.stop_intake_watch()


class GatewayHTTPServer(ThreadingHTTPServer):
    daemon_threads=False
    block_on_close=True
    allow_reuse_address=False

    def __init__(self,address,controller,*,max_connections=16,intake_timeout=5,io_timeout=1):
        if type(max_connections) is not int or not 1<=max_connections<=128:raise ValueError('bounded connection count required')
        for timeout in (intake_timeout,io_timeout):
            if type(timeout) not in (int,float) or not 0<timeout<=30:raise ValueError('bounded HTTP timeout required')
        self.controller=controller;self.intake_timeout=intake_timeout;self.io_timeout=io_timeout
        self.slots=threading.BoundedSemaphore(max_connections)
        super().__init__(address,GatewayHandler)

    def process_request(self,request,client_address):
        if not self.slots.acquire(blocking=False):
            self.shutdown_request(request);return
        try:super().process_request(request,client_address)
        except Exception:self.slots.release();raise

    def process_request_thread(self,request,client_address):
        try:super().process_request_thread(request,client_address)
        finally:self.slots.release()

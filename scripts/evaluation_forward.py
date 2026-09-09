"""Single-dispatch bounded HTTPS forwarding for an already admitted request.

No approval, credential discovery, automatic retry or usage settlement. Supply a
reviewed provider key from trusted gateway memory and a private evidence sink.
Network isolation and trustworthy DNS/TLS environment remain deployment gates.
"""
import hashlib
import http.client
import math
import socket
import ssl
import threading
import time


def https_connection(timeout):
    return http.client.HTTPSConnection('api.anthropic.com', timeout=timeout,
                                      context=ssl.create_default_context())


def forward(ledger, run_id, request_id, body, provider_key, sink, *, timeout_seconds,
            cancel=None, max_response_bytes=16 << 20, connection_factory=https_connection, on_response=None):
    if type(body) is not bytes or not callable(sink): raise ValueError('immutable body and evidence sink required')
    if on_response is not None and not callable(on_response):raise ValueError('callable response observer required')
    if type(provider_key) is not str or not 1 <= len(provider_key) <= 4096 or any(ord(c)<33 or ord(c)>126 for c in provider_key):
        raise ValueError('explicit provider credential required')
    if type(timeout_seconds) not in (int,float) or not math.isfinite(timeout_seconds) or not 0 < timeout_seconds <= 3600:
        raise ValueError('bounded forwarding timeout required')
    if type(max_response_bytes) is not int or not 1 <= max_response_bytes <= 64 << 20:
        raise ValueError('bounded response limit required')
    transport = ledger.transport(request_id)
    if transport['body_length'] != len(body) or transport['body_sha256'] != hashlib.sha256(body).hexdigest():
        raise ValueError('outbound bytes differ from admitted request')
    if cancel is not None and cancel.is_set(): raise ValueError('dispatch cancelled before claim')
    remaining = ledger.claim_dispatch(run_id,request_id)
    timeout = min(timeout_seconds,remaining)
    deadline = time.monotonic()+timeout
    connection = response = watcher = None
    done = threading.Event(); interrupted = threading.Event(); connected_socket = []
    result = dict(status='incomplete',request_id=request_id,bytes_received=0,
        limitation='Raw upstream observation only; reservation remains uncertain until authentic usage reconciliation.')
    digest = hashlib.sha256()
    def watch():
        while not done.wait(0.01):
            if time.monotonic() >= deadline or (cancel is not None and cancel.is_set()):
                interrupted.set()
                if connected_socket:
                    try: connected_socket[0].shutdown(socket.SHUT_RDWR)
                    except OSError: pass
                return
    def check():
        if interrupted.is_set() or time.monotonic() >= deadline or (cancel is not None and cancel.is_set()):
            raise TimeoutError('forwarding deadline or cancellation')
    try:
        # Claim has committed before factory/connect can create network effects.
        connection = connection_factory(timeout)
        watcher = threading.Thread(target=watch,daemon=True);watcher.start()
        connection.connect()
        connected_socket.append(connection.sock)
        check()
        headers = dict(transport['headers']);headers['x-api-key']=provider_key
        connection.request(transport['method'],transport['target'],body=body,headers=headers)
        response = connection.getresponse()
        result['http_status']=response.status
        # Never follow redirects or forward Location, Set-Cookie or other
        # upstream-controlled headers into a new routing decision.
        result['response_headers']={key:response.getheader(key) for key in ('content-type','content-encoding','request-id') if response.getheader(key) is not None}
        if on_response is not None:on_response(response.status,dict(result['response_headers']))
        while True:
            check()
            chunk=response.read1(min(65536,max_response_bytes-result['bytes_received']+1))
            if not chunk: break
            digest.update(chunk);result['bytes_received']+=len(chunk)
            if result['bytes_received']>max_response_bytes: raise ValueError('response cap exceeded')
            sink(chunk)
        check()
        # HTTPResponse.read1 does not itself reject premature Content-Length EOF.
        if response.length not in (None,0): raise ValueError('truncated upstream body')
        result['status']='response_received'
    except Exception as error:
        # Exception text may contain request headers or secrets. Keep only type.
        result['error_type']=type(error).__name__
    finally:
        done.set()
        if response is not None: response.close()
        if connection is not None: connection.close()
        if watcher is not None: watcher.join(timeout=1)
        result['watcher_joined']=watcher is None or not watcher.is_alive()
        result['body_sha256']=digest.hexdigest()
        # Even a complete 200 or error response does not prove billable usage.
        ledger.mark_uncertain(request_id)
    return result

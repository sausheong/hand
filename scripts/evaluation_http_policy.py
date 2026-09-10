"""Validate an evaluation gateway's HTTP/1 request envelope before admission.

No socket, forwarding, credential loading or authorisation is implemented here.
The server must supply raw header pairs before merging duplicate field names,
fully read the bounded body, and hold provider credentials outside agent access.
"""
import hashlib
import hmac
import json
import re


TELEMETRY = frozenset(('user-agent', 'x-stainless-arch', 'x-stainless-lang',
    'x-stainless-os', 'x-stainless-package-version', 'x-stainless-retry-count',
    'x-stainless-runtime', 'x-stainless-runtime-version', 'x-stainless-timeout'))


def validate_headers(pairs, body_length, run_token, gateway_authority, allowed_beta_sets):
    """Return reviewed upstream headers, omitting gateway credentials/framing.

    allowed_beta_sets must come from the frozen, reviewed run policy. Empty beta
    selection must be explicitly included when omission is permitted. No beta
    semantics are inferred from a token's spelling.
    """
    if type(body_length) is not int or not 1 <= body_length <= 16 << 20:
        raise ValueError('bounded positive body length required')
    if type(run_token) is not str or not re.fullmatch('[A-Za-z0-9_-]{32,256}', run_token):
        raise ValueError('opaque gateway run token required')
    if type(gateway_authority) is not str or not re.fullmatch(r'[A-Za-z0-9.\[\]:-]{1,255}', gateway_authority):
        raise ValueError('explicit gateway authority required')
    if type(allowed_beta_sets) not in (list, tuple) or not 1 <= len(allowed_beta_sets) <= 32:
        raise ValueError('explicit bounded beta policy required')
    reviewed = []
    for selection in allowed_beta_sets:
        if type(selection) not in (list, tuple) or len(selection) > 16:
            raise ValueError('beta selection list required')
        if any(type(beta) is not str or not re.fullmatch('[a-z0-9-]{1,128}', beta) for beta in selection):
            raise ValueError('invalid beta policy token')
        if len(set(selection)) != len(selection): raise ValueError('duplicate beta policy token')
        reviewed.append(frozenset(selection))
    if type(pairs) not in (list, tuple) or not 1 <= len(pairs) <= 64:
        raise ValueError('bounded raw header pairs required')
    headers, total = {}, 0
    for pair in pairs:
        if type(pair) not in (list, tuple) or len(pair) != 2:
            raise ValueError('raw header pair required')
        name, value = pair
        if type(name) is not str or not re.fullmatch("[!#$%&'*+.^_`|~0-9A-Za-z-]+", name):
            raise ValueError('invalid header name')
        if type(value) is not str or len(value) > 4096 or any(ord(c)<32 or ord(c)>126 for c in value):
            raise ValueError('bounded printable ASCII header required')
        total += len(name)+len(value)
        if total > 16384: raise ValueError('header block too large')
        key = name.lower()
        if key in headers: raise ValueError('duplicate header field')
        headers[key] = value
    allowed = TELEMETRY | {'host', 'content-length', 'content-type', 'accept', 'accept-encoding', 'accept-language', 'sec-fetch-mode',
        'x-api-key', 'anthropic-version', 'anthropic-beta', 'anthropic-dangerous-direct-browser-access', 'connection'}
    if set(headers)-allowed: raise ValueError('unreviewed header field')
    if not hmac.compare_digest(headers.get('x-api-key',''), run_token):
        raise ValueError('invalid run credential')
    if headers.get('host') != gateway_authority: raise ValueError('gateway authority mismatch')
    if headers.get('content-length') != str(body_length): raise ValueError('ambiguous or incorrect request length')
    if headers.get('content-type') != 'application/json': raise ValueError('JSON content type required')
    if headers.get('anthropic-version') != '2023-06-01': raise ValueError('unreviewed API version')
    if 'accept' in headers and headers['accept'] not in ('application/json', 'text/event-stream', '*/*'):
        raise ValueError('unsupported response media type')
    if headers.get('accept-language','*') != '*' or headers.get('sec-fetch-mode','cors') != 'cors':
        raise ValueError('unreviewed fetch transport option')
    if 'connection' in headers and headers['connection'].lower() not in ('close','keep-alive'):
        raise ValueError('connection header cannot nominate fields')
    if 'accept-encoding' in headers:
        values = [v.strip() for v in headers['accept-encoding'].split(',')]
        if not values or any(v not in ('gzip','deflate','br','identity') for v in values):
            raise ValueError('unsupported response encoding negotiation')
    if headers.get('anthropic-dangerous-direct-browser-access', 'true') != 'true':
        raise ValueError('unsupported SDK browser-access option')
    beta_value = headers.get('anthropic-beta')
    selection = [] if beta_value is None else [s.strip() for s in beta_value.split(',')]
    if any(not re.fullmatch('[a-z0-9-]{1,128}', s) for s in selection) or len(set(selection)) != len(selection):
        raise ValueError('invalid beta selection')
    if frozenset(selection) not in reviewed: raise ValueError('beta combination not reviewed')
    # The outbound HTTP library regenerates Host and Content-Length for the
    # frozen upstream and same bytes. It must inject the real provider key.
    forwarded = {key:value for key,value in headers.items()
                 if key not in ('x-api-key','host','content-length','connection')}
    canonical = json.dumps(forwarded,sort_keys=True,separators=(',',':')).encode()
    return dict(headers=forwarded, headers_sha256=hashlib.sha256(canonical).hexdigest(),
                beta_selection=sorted(selection), credential_forwarded=False,
                limitation='Header validation only; run expiry/revocation, quote admission, upstream isolation and forwarding are separate requirements.')


def validate_transport(transport):
    """Check credential-free persisted metadata for the supported fixed upstream."""
    required = {'origin','method','target','headers','body_sha256','body_length','allowed_beta_sets'}
    if type(transport) is not dict or set(transport) != required:
        raise ValueError('complete transport record required')
    if transport['origin'] != 'https://api.anthropic.com' or transport['method'] != 'POST':
        raise ValueError('unreviewed upstream transport')
    if transport['target'] not in ('/v1/messages','/v1/messages?beta=true'):
        raise ValueError('unreviewed upstream target')
    if type(transport['body_sha256']) is not str or not re.fullmatch('[a-f0-9]{64}',transport['body_sha256']):
        raise ValueError('body digest required')
    headers = transport['headers']
    if type(headers) is not dict or any(type(k) is not str or k != k.lower() for k in headers):
        raise ValueError('canonical upstream headers required')
    if set(headers) & {'host','content-length','connection','x-api-key'}:
        raise ValueError('credentials or hop framing in transport record')
    # Re-run the same reviewed header rules without storing an actual run key.
    token = 'validation_only_'+'x'*32
    pairs = list(headers.items()) + [('host','gateway.invalid'),('content-length',str(transport['body_length'])),('x-api-key',token)]
    validate_headers(pairs,transport['body_length'],token,'gateway.invalid',transport['allowed_beta_sets'])


def admit_http_request(ledger, run_id, request_id, method, target, pairs, body,
                       run_token, gateway_authority, allowed_beta_sets, contract, model):
    """Validate and durably admit one complete request; does not forward it.

    Trusted caller resolves run_id/token/model/contract from the frozen schedule.
    Clients must not select that mapping. Provider credentials remain absent.
    """
    from evaluation_anthropic_request import validate_request
    if type(body) is not bytes: raise ValueError('immutable body required')
    # Freeze caller-owned policy before validating and retaining it.
    policy = json.loads(json.dumps(allowed_beta_sets,allow_nan=False))
    checked = validate_headers(pairs,len(body),run_token,gateway_authority,policy)
    output = validate_request(method,target,body,model)
    transport = dict(origin='https://api.anthropic.com',method=method,target=target,
        headers=checked['headers'],body_sha256=hashlib.sha256(body).hexdigest(),
        body_length=len(body),allowed_beta_sets=policy)
    return ledger.reserve_quoted(run_id,request_id,body,contract,'anthropic',model,output,transport=transport)

"""Validate complete Anthropic JSON/SSE usage observations, never billing approval.

Missing or unsupported usage remains uncertain. This module does not settle a
ledger, price a response or authenticate a file's provider origin.
"""
import hashlib

from evaluation_anthropic_request import parse_body
from evaluation_budget import integer

CORE = ('input_tokens','output_tokens','cache_creation_input_tokens','cache_read_input_tokens')


def counters(value, previous=None):
    if type(value) is not dict or set(value)-set(CORE)-{'cache_creation','service_tier'}:
        raise ValueError('unsupported usage/billing fields')
    merged = dict(previous or {})
    if previous is None and set(CORE)-set(value): raise ValueError('missing explicit initial usage counters')
    for key in CORE:
        if key in value:
            n=integer(value[key])
            if key in merged and n<merged[key]: raise ValueError('cumulative usage regressed')
            merged[key]=n
    if 'service_tier' in value:
        if value['service_tier'] != 'standard': raise ValueError('unreviewed service tier')
        merged['service_tier']='standard'
    if 'cache_creation' in value:
        split=value['cache_creation']
        if type(split) is not dict or set(split)!={'ephemeral_5m_input_tokens','ephemeral_1h_input_tokens'}:
            raise ValueError('incomplete cache creation breakdown')
        if sum(integer(n) for n in split.values()) != merged['cache_creation_input_tokens']:
            raise ValueError('cache creation breakdown disagrees')
        merged['cache_creation']=dict(split)
    if 'cache_creation' in merged and sum(merged['cache_creation'].values()) != merged['cache_creation_input_tokens']:
        raise ValueError('cache creation total changed without breakdown')
    return merged


def message_identity(message, model):
    if type(message) is not dict or message.get('type')!='message' or message.get('role')!='assistant' or message.get('model')!=model:
        raise ValueError('response identity differs from requested model')
    identity=message.get('id')
    if type(identity) is not str or not 1<=len(identity)<=256: raise ValueError('bounded provider message ID required')
    if type(message.get('content')) is not list: raise ValueError('content inventory required')
    for block in message['content']:
        if type(block) is not dict or block.get('type') not in ('text','thinking','redacted_thinking','tool_use'):
            raise ValueError('unreviewed server content/billing type')
    return identity


def stop_reason(value):
    if value not in ('end_turn','max_tokens','stop_sequence','tool_use','refusal','pause_turn'):
        raise ValueError('missing or unsupported stop reason')
    return value


def sse_events(raw):
    try: decoded=raw.decode('utf-8')
    except UnicodeError as error: raise ValueError('invalid SSE UTF-8') from error
    # split only on protocol LF, never Unicode text separators in JSON strings.
    if not decoded.endswith('\n\n') and not decoded.endswith('\r\n\r\n'):
        raise ValueError('truncated SSE event framing')
    name=None;data=[]
    for line in decoded.split('\n'):
        if line.endswith('\r'):line=line[:-1]
        if '\r' in line: raise ValueError('unsupported SSE line framing')
        if not line:
            if data:
                event=parse_body('\n'.join(data).encode())
                if type(event) is not dict or event.get('type') != name: raise ValueError('SSE event name/body disagree')
                yield event
            elif name is not None: raise ValueError('SSE event lacks data')
            name=None;data=[];continue
        if line.startswith(':'):continue
        key,sep,value=line.partition(':')
        if not sep: raise ValueError('malformed SSE field')
        if value.startswith(' '):value=value[1:]
        if key=='event':
            if name is not None: raise ValueError('duplicate SSE event name')
            name=value
        elif key=='data':data.append(value)
        else:raise ValueError('unsupported SSE control field')


def observe_usage(raw, content_type, model):
    if type(raw) is not bytes or not 1<=len(raw)<=16<<20: raise ValueError('bounded raw response required')
    if type(model) is not str or not model:raise ValueError('expected model required')
    digest=hashlib.sha256(raw).hexdigest()
    try:
        media=content_type.split(';',1)[0].strip().lower()
        if media=='application/json':
            message=parse_body(raw);identity=message_identity(message,model)
            usage=counters(message.get('usage'));reason=stop_reason(message.get('stop_reason'))
        elif media=='text/event-stream':
            identity=usage=reason=None;stopped=False;active=set();seen=set();final=False
            for event in sse_events(raw):
                kind=event['type']
                if stopped:raise ValueError('events after message_stop')
                if kind=='ping':continue
                if kind=='message_start':
                    if identity is not None:raise ValueError('duplicate message_start')
                    message=event.get('message');identity=message_identity(message,model)
                    if message['content'] or message.get('stop_reason') is not None:raise ValueError('unexpected initial message state')
                    usage=counters(message.get('usage'))
                elif identity is None:raise ValueError('event before message_start')
                elif kind=='content_block_start':
                    index=integer(event.get('index'))
                    if final or index in seen:raise ValueError('duplicate or late content block')
                    block=event.get('content_block')
                    if type(block) is not dict or block.get('type') not in ('text','thinking','redacted_thinking','tool_use'):
                        raise ValueError('unsupported content block/billing type')
                    seen.add(index);active.add(index)
                elif kind in ('content_block_delta','content_block_stop'):
                    index=integer(event.get('index'))
                    if final or index not in active:raise ValueError('content event outside active block')
                    if kind=='content_block_stop':active.remove(index)
                elif kind=='message_delta':
                    if active or final:raise ValueError('duplicate or premature final message delta')
                    update=event.get('usage')
                    if type(update) is not dict or 'output_tokens' not in update:raise ValueError('missing final output usage')
                    usage=counters(update,usage)
                    reason=stop_reason(event.get('delta',{}).get('stop_reason'));final=True
                elif kind=='message_stop':
                    if active or not final:raise ValueError('message stopped without complete usage/content')
                    stopped=True
                else:raise ValueError('unsupported or failed provider event')
            if not stopped:raise ValueError('missing message_stop')
        else:raise ValueError('unsupported response content type')
        total=integer(usage['input_tokens']+usage['cache_creation_input_tokens']+usage['cache_read_input_tokens'])
        return dict(status='usage_observed_unverified',message_id=identity,model=model,stop_reason=reason,
            input_tokens=total,output_tokens=usage['output_tokens'],uncached_input_tokens=usage['input_tokens'],
            cache_read_input_tokens=usage['cache_read_input_tokens'],cache_creation_input_tokens=usage['cache_creation_input_tokens'],
            provider_usage=usage,body_sha256=digest,
            limitation='Complete explicit counters only; provider origin, pricing and billing reconciliation are not established.')
    except (ValueError,TypeError,KeyError,AttributeError) as error:
        return dict(status='incomplete',body_sha256=digest,error=str(error),
                    limitation='No settlement permitted from incomplete usage observation.')


def verify_observed_usage(observation, raw, model):
    """Bind usage parsing to a complete sender observation and its exact bytes."""
    if (observation.get('status')!='response_received' or observation.get('http_status')!=200
            or observation.get('bytes_received')!=len(raw)
            or observation.get('body_sha256')!=hashlib.sha256(raw).hexdigest()):
        raise ValueError('complete matching successful upstream observation required')
    headers=observation.get('response_headers',{})
    if headers.get('content-encoding','identity')!='identity':raise ValueError('encoded response requires reviewed bounded decoding')
    return observe_usage(raw,headers.get('content-type',''),model)

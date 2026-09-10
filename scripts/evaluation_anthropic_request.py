"""Strict text/custom-tool Messages body admission; no HTTP forwarding or approval.

Based on checked-out Harness and Pi request builders. The HTTP gateway must
separately freeze upstream origin, headers/betas and credentials, reject
redirects and alternate routes, and forward the admitted bytes unchanged.
"""
import json
import math

from evaluation_budget import integer


def fields(value, allowed, required=()):
    if type(value) is not dict or set(value)-set(allowed) or set(required)-set(value):
        raise ValueError('unsupported or incomplete request fields')


def text(value):
    if type(value) is not str:
        raise ValueError('text required')


def flag(value):
    if type(value) is not bool:
        raise ValueError('boolean required')


def cache(value):
    fields(value, ('type', 'ttl'), ('type',))
    if value['type'] != 'ephemeral' or value.get('ttl', '5m') not in ('5m', '1h'):
        raise ValueError('unsupported cache policy')


def effort(value):
    fields(value, ('effort',), ('effort',))
    text(value['effort'])
    if not value['effort'] or len(value['effort']) > 64:
        raise ValueError('bounded effort required')


def content(value, allowed):
    if type(value) is str:
        return
    if type(value) is not list:
        raise ValueError('text or content list required')
    for block in value:
        if type(block) is not dict or type(block.get('type')) is not str or block['type'] not in allowed:
            raise ValueError('unsupported content/billing type')
        kind = block['type']
        if kind == 'text':
            fields(block, ('type', 'text', 'cache_control'), ('type', 'text'))
            text(block['text'])
        elif kind == 'tool_use':
            fields(block, ('type', 'id', 'name', 'input', 'cache_control'), ('type', 'id', 'name', 'input'))
            text(block['id']); text(block['name'])
            if type(block['input']) is not dict: raise ValueError('tool input object required')
        elif kind == 'tool_result':
            fields(block, ('type', 'tool_use_id', 'content', 'is_error', 'cache_control'), ('type', 'tool_use_id', 'content'))
            text(block['tool_use_id']); content(block['content'], {'text'})
            if 'is_error' in block: flag(block['is_error'])
        elif kind == 'thinking':
            fields(block, ('type', 'thinking', 'signature'), ('type', 'thinking', 'signature'))
            text(block['thinking']); text(block['signature'])
        elif kind == 'redacted_thinking':
            fields(block, ('type', 'data'), ('type', 'data')); text(block['data'])
        if 'cache_control' in block: cache(block['cache_control'])


def parse_body(raw):
    if type(raw) is not bytes or not 1 <= len(raw) <= 16 << 20:
        raise ValueError('bounded immutable body bytes required')
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result: raise ValueError('duplicate JSON field')
            result[key] = value
        return result
    def invalid(value):
        raise ValueError('nonfinite JSON number')
    try:
        value = json.loads(raw.decode('utf-8'), object_pairs_hook=unique, parse_constant=invalid)
        stack, count = [(value, 0)], 0
        while stack:
            node, depth = stack.pop(); count += 1
            if depth > 64 or count > 200000: raise ValueError('request structure limit exceeded')
            if type(node) is float and not math.isfinite(node): raise ValueError('nonfinite JSON number')
            if type(node) is str: node.encode('utf-8')  # reject unpaired surrogates
            if type(node) is dict:
                for key, item in node.items():
                    key.encode('utf-8'); stack.append((item, depth+1))
            elif type(node) is list:
                stack.extend((item, depth+1) for item in node)
        return value
    except (UnicodeError, RecursionError) as error:
        raise ValueError('invalid UTF-8 or excessive nesting') from error


def validate_request(method, target, body, model):
    # The pinned Anthropic SDK's beta Messages client adds exactly beta=true.
    # Accept that spelling unchanged; arbitrary queries/alternate paths remain
    # forbidden. The HTTP layer still needs an explicit beta-header policy.
    if method != 'POST' or target not in ('/v1/messages', '/v1/messages?beta=true'):
        raise ValueError('only the frozen Messages route is supported')
    request = parse_body(body)
    fields(request, ('model', 'max_tokens', 'messages', 'system', 'tools', 'tool_choice',
        'stream', 'temperature', 'top_p', 'top_k', 'stop_sequences', 'thinking',
        'output_config', 'cache_control', 'metadata'), ('model', 'max_tokens', 'messages'))
    if request['model'] != model: raise ValueError('request model differs from frozen model')
    output = integer(request['max_tokens'], True)
    if type(request['messages']) is not list or not request['messages']:
        raise ValueError('nonempty messages required')
    for message in request['messages']:
        fields(message, ('role', 'content', 'output_config'), ('role', 'content'))
        role = message['role']
        if role == 'system':
            if message['content'] != [] or 'output_config' not in message:
                raise ValueError('only managed-effort system markers supported')
            effort(message['output_config'])
        elif role in ('user', 'assistant'):
            if 'output_config' in message: raise ValueError('unexpected message effort')
            content(message['content'], {'text', 'tool_result'} if role == 'user' else {'text', 'tool_use', 'thinking', 'redacted_thinking'})
        else: raise ValueError('unsupported message role')
    if 'system' in request: content(request['system'], {'text'})
    if 'cache_control' in request: cache(request['cache_control'])
    if 'stream' in request: flag(request['stream'])
    for key in ('temperature', 'top_p'):
        if key in request and (type(request[key]) not in (int, float) or not 0 <= request[key] <= 1):
            raise ValueError('sampling parameter outside range')
    if 'top_k' in request: integer(request['top_k'])
    if 'stop_sequences' in request:
        if type(request['stop_sequences']) is not list: raise ValueError('stop list required')
        for stop in request['stop_sequences']: text(stop)
    if 'metadata' in request:
        fields(request['metadata'], ('user_id',))
        if 'user_id' in request['metadata']: text(request['metadata']['user_id'])
    if 'output_config' in request: effort(request['output_config'])
    if 'thinking' in request:
        thinking = request['thinking']
        fields(thinking, ('type', 'budget_tokens', 'display', 'block_binding'), ('type',))
        if thinking['type'] not in ('enabled', 'disabled', 'adaptive'): raise ValueError('unsupported thinking mode')
        if thinking['type'] == 'enabled':
            if 'budget_tokens' not in thinking or integer(thinking['budget_tokens'], True) >= output:
                raise ValueError('thinking budget must fit output cap')
        elif 'budget_tokens' in thinking: raise ValueError('unexpected thinking budget')
        if 'display' in thinking and thinking['display'] not in ('summarized', 'omitted'): raise ValueError('unsupported thinking display')
        if 'block_binding' in thinking and thinking['block_binding'] != {'prefix_mismatch_behavior':'drop_block'}:
            raise ValueError('unsupported thinking binding')
    if 'tools' in request:
        if type(request['tools']) is not list: raise ValueError('tool list required')
        for tool in request['tools']:
            fields(tool, ('type', 'name', 'description', 'input_schema', 'cache_control', 'strict', 'defer_loading', 'eager_input_streaming'), ('name', 'input_schema'))
            if tool.get('type', 'custom') != 'custom': raise ValueError('server-billed tools unsupported')
            text(tool['name'])
            if 'description' in tool: text(tool['description'])
            if type(tool['input_schema']) is not dict: raise ValueError('schema object required')
            if 'cache_control' in tool: cache(tool['cache_control'])
            for key in ('strict', 'defer_loading', 'eager_input_streaming'):
                if key in tool: flag(tool[key])
    if 'tool_choice' in request:
        choice = request['tool_choice']; fields(choice, ('type', 'name', 'disable_parallel_tool_use'), ('type',))
        if choice['type'] not in ('auto', 'any', 'tool', 'none'): raise ValueError('unsupported tool choice')
        if choice['type'] == 'tool': text(choice.get('name'))
        elif 'name' in choice: raise ValueError('unexpected selected tool name')
        if 'disable_parallel_tool_use' in choice: flag(choice['disable_parallel_tool_use'])
    return output


def admit_request(ledger, run_id, request_id, method, target, body, contract, model):
    output = validate_request(method, target, body, model)
    return ledger.reserve_quoted(run_id, request_id, body, contract, 'anthropic', model, output)

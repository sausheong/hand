"""Validate one fresh Hand JSONL session. Observation never proves task quality.

The caller owns process exit, cleanup, exact binary/model identity and billing.
"""
import json
from datetime import datetime
from evaluation_go_tests import unique_fields


class HandRunObserver:
    def __init__(self, request_id='oneshot', max_bytes=16 << 20):
        if not isinstance(request_id, str) or not request_id:
            raise ValueError('request identity required')
        if type(max_bytes) is not int or not 0 < max_bytes <= 64 << 20:
            raise ValueError('bounded transcript size required')
        self.request_id, self.max_bytes = request_id, max_bytes
        self.buffer = b''
        self.bytes = self.count = self.sequence = self.tool_results = self.tool_errors = 0
        self.identity = self.terminal = None
        self.ids, self.active = set(), {}
        self.errors = []
        self.closed = False

    def feed(self, data):
        if self.closed or not isinstance(data, bytes):
            raise ValueError('open observer and bytes required')
        self.bytes += len(data)
        if self.bytes > self.max_bytes:
            self.errors.append('transcript limit exceeded')
            self.buffer = b''
            raise ValueError(self.errors[-1])
        self.buffer += data
        while b'\n' in self.buffer:
            line, self.buffer = self.buffer.split(b'\n', 1)
            self.count += 1
            try:
                def reject(value):
                    raise ValueError('nonfinite JSON')
                self.observe(json.loads(line.decode('utf-8'), object_pairs_hook=unique_fields, parse_constant=reject))
            except (ValueError, TypeError, KeyError, OverflowError, RecursionError) as error:
                self.errors.append(type(error).__name__ + ': ' + str(error))

    def observe(self, e):
        if not isinstance(e, dict) or self.terminal is not None:
            raise ValueError('event object before terminal required')
        if type(e.get('version')) is not int or e['version'] != 1:
            raise ValueError('event version 1 required')
        if type(e.get('sequence')) is not int or e['sequence'] != self.sequence + 1:
            raise ValueError('contiguous event sequence required')
        for key in ('event_id', 'session_id', 'run_id', 'request_id', 'timestamp', 'kind'):
            if not isinstance(e.get(key), str) or not e[key]:
                raise ValueError('event identity and kind required')
        if e['event_id'] in self.ids or e['request_id'] != self.request_id:
            raise ValueError('duplicate event or wrong request')
        stamp = datetime.fromisoformat(e['timestamp'])
        if stamp.tzinfo is None:
            raise ValueError('timezone required')
        identity = (e['session_id'], e['run_id'])
        if self.identity is not None and identity != self.identity:
            raise ValueError('session/run changed')
        p = e.get('payload')
        if not isinstance(p, dict):
            raise ValueError('payload object required')
        kind = e['kind']
        if kind in ('tool_call', 'tool_result'):
            tool = p.get('tool')
            if not isinstance(tool, dict) or any(not isinstance(tool.get(k), str) or not tool[k] for k in ('id', 'name')):
                raise ValueError('tool identity required')
            tid = tool['id']
            if kind == 'tool_call':
                if tid in self.active:
                    raise ValueError('duplicate active tool')
                self.active[tid] = tool['name']
            else:
                result = p.get('result')
                if self.active.get(tid) != tool['name'] or not isinstance(result, dict):
                    raise ValueError('unmatched tool result')
                if any(not isinstance(result.get(k), str) for k in ('output', 'error')):
                    raise ValueError('tool result strings required')
                del self.active[tid]
                self.tool_results += 1
                self.tool_errors += bool(result['error'])
        if kind == 'terminal':
            if self.active or p.get('status') not in ('completed', 'verification_failed', 'budget_exhausted', 'cancelled', 'infrastructure_error'):
                raise ValueError('valid terminal with joined tools required')
            self.terminal = dict(status=p['status'], reason=p.get('reason'))
        self.identity = identity
        self.sequence = e['sequence']
        self.ids.add(e['event_id'])

    def finish(self):
        self.closed = True
        errors = list(self.errors)
        if self.buffer:
            errors.append('unterminated record')
        if self.terminal is None or self.active:
            errors.append('missing terminal or unfinished tools')
        return dict(status='incomplete' if errors else 'observed_unverified',
                    outcome=self.terminal['status'] if self.terminal else None,
                    events=self.count, tool_results=self.tool_results, tool_errors=self.tool_errors,
                    errors=errors, limitation='No task scoring, process exit/cleanup, model identity or billing verification.')

"""Observe one fresh Pi v0.85.1 RPC run; never launch or authorise it.

The caller owns raw logs, timeouts, process cleanup and budget enforcement.
Only agent_settled plus correlated final statistics closes observation. Neither
an observed completion nor reported cost proves task success or actual billing.
"""
import json
import math

from evaluation_go_tests import unique_fields


def reject_constant(value):
    raise ValueError('nonfinite JSON: ' + value)


class PiRunObserver:
    def __init__(self, prompt_id, stats_id, provider, model, max_bytes=16 << 20):
        if any(not isinstance(v, str) or not v for v in (prompt_id, stats_id, provider, model)) or prompt_id == stats_id:
            raise ValueError('distinct request IDs and exact model identity required')
        if type(max_bytes) is not int or not 0 < max_bytes <= 64 << 20:
            raise ValueError('bounded transcript size required')
        self.prompt_id, self.stats_id = prompt_id, stats_id
        self.provider, self.model = provider, model
        self.max_bytes, self.bytes = max_bytes, 0
        self.buffer = b''
        self.accepted = None
        self.started = self.ended = 0
        self.settled = False
        self.stats = None
        self.last_stop = None
        self.assistant_messages = self.assistant_errors = 0
        self.active_tools = set()
        self.errors = []
        self.event_count = 0
        self.closed = False

    @property
    def ready_for_stats(self):
        return not self.closed and self.accepted is True and self.settled and self.stats is None and not self.errors

    def feed(self, data):
        if self.closed:
            raise ValueError('observer already finished')
        if not isinstance(data, bytes):
            raise ValueError('RPC input must be bytes')
        self.bytes += len(data)
        if self.bytes > self.max_bytes:
            self.errors.append('transcript byte limit exceeded')
            self.buffer = b''
            raise ValueError(self.errors[-1])
        self.buffer += data
        # LF is the only record delimiter. Unicode line separators are JSON
        # string content, not framing boundaries.
        while b'\n' in self.buffer:
            line, self.buffer = self.buffer.split(b'\n', 1)
            try:
                event = json.loads(line.decode('utf-8'), object_pairs_hook=unique_fields,
                                   parse_constant=reject_constant)
                if not isinstance(event, dict) or not isinstance(event.get('type'), str):
                    raise ValueError('event object and type required')
                self.observe(event)
            except (ValueError, UnicodeDecodeError, TypeError, OverflowError) as error:
                self.errors.append(str(error))
            self.event_count += 1

    def observe(self, event):
        kind = event['type']
        if kind == 'response':
            identity = event.get('id')
            if identity == self.prompt_id:
                if self.accepted is not None or event.get('command') != 'prompt' or type(event.get('success')) is not bool:
                    raise ValueError('invalid or duplicate prompt acknowledgement')
                self.accepted = event['success']
            elif identity == self.stats_id:
                if not self.ready_for_stats or event.get('command') != 'get_session_stats' or event.get('success') is not True:
                    raise ValueError('statistics must follow accepted settled run exactly once')
                data = event.get('data')
                if not isinstance(data, dict) or not isinstance(data.get('tokens'), dict):
                    raise ValueError('statistics and token totals required')
                tokens = data['tokens']
                names = ('input', 'output', 'cacheRead', 'cacheWrite', 'total')
                if any(type(tokens.get(k)) is not int or tokens[k] < 0 for k in names):
                    raise ValueError('invalid token totals')
                if tokens['total'] != sum(tokens[k] for k in names[:-1]):
                    raise ValueError('inconsistent token totals')
                cost = data.get('cost')
                if cost is not None and (type(cost) not in (int, float) or not math.isfinite(cost) or cost < 0):
                    raise ValueError('invalid reported cost')
                self.stats = dict(tokens={k: tokens[k] for k in names}, reported_cost_usd=cost)
            return
        if kind in ('agent_start', 'agent_end', 'message_start', 'message_update', 'message_end',
                    'tool_execution_start', 'tool_execution_update', 'tool_execution_end',
                    'compaction_start', 'auto_retry_start', 'summarization_retry_scheduled') and self.settled:
            raise ValueError('agent activity after settlement')
        if kind == 'agent_start':
            self.started += 1
        elif kind == 'agent_end':
            self.ended += 1
            if self.ended > self.started:
                raise ValueError('agent ended without starting')
        elif kind == 'agent_settled':
            if self.settled or self.started == 0 or self.started != self.ended or self.active_tools:
                raise ValueError('invalid settlement or unfinished agent/tool work')
            self.settled = True
        elif kind == 'tool_execution_start':
            identity = event.get('toolCallId')
            if not isinstance(identity, str) or not identity or identity in self.active_tools:
                raise ValueError('invalid or duplicate active tool identity')
            self.active_tools.add(identity)
        elif kind == 'tool_execution_end':
            identity = event.get('toolCallId')
            if identity not in self.active_tools:
                raise ValueError('tool ended without starting')
            self.active_tools.remove(identity)
        elif kind == 'message_end':
            message = event.get('message')
            if not isinstance(message, dict):
                raise ValueError('message object required')
            if message.get('role') == 'assistant':
                if message.get('provider') != self.provider or message.get('model') != self.model:
                    raise ValueError('assistant model differs from frozen identity')
                stop = message.get('stopReason')
                if stop not in ('stop', 'length', 'toolUse', 'error', 'aborted'):
                    raise ValueError('unknown assistant stop reason')
                self.assistant_messages += 1
                self.assistant_errors += stop == 'error'
                self.last_stop = stop

    def finish(self):
        self.closed = True
        gaps = list(self.errors)
        if self.buffer:
            gaps.append('truncated JSONL record')
        if self.accepted is not True:
            gaps.append('prompt not accepted')
        if not self.settled:
            gaps.append('missing full agent settlement')
        if self.stats is None:
            gaps.append('missing final statistics')
        if self.last_stop is None:
            gaps.append('missing assistant outcome')
        outcome = ('cancelled' if self.last_stop == 'aborted' else
                   'completed' if self.last_stop == 'stop' else 'failed')
        return dict(status='incomplete' if gaps else 'observed_unverified',
                    outcome=outcome if not gaps else None, errors=gaps, events=self.event_count,
                    assistant_messages=self.assistant_messages, assistant_errors=self.assistant_errors,
                    statistics=self.stats,
                    limitation='Not task scoring, process cleanup, source authentication, gateway accounting or spending enforcement.')

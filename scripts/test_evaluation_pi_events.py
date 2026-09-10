import copy
import json
import gzip
from pathlib import Path
import unittest

from evaluation_pi_events import PiRunObserver


def assistant(stop='stop'):
    return dict(type='message_end', message=dict(role='assistant', provider='fixture', model='frozen-model',
                                                stopReason=stop, content=[dict(type='text', text='line\u2028separator')]))


def transcript():
    return [dict(type='response', id='prompt-1', command='prompt', success=True),
            dict(type='agent_start'), assistant(), dict(type='agent_end', willRetry=False),
            dict(type='agent_settled'),
            dict(type='response', id='stats-1', command='get_session_stats', success=True,
                 data=dict(tokens=dict(input=12, output=3, cacheRead=4, cacheWrite=1, total=20), cost=0.002))]


def encode(events):
    return b''.join((json.dumps(event, ensure_ascii=False)+'\n').encode() for event in events)


class PiObserverTests(unittest.TestCase):
    def test_recorded_gateway_tool_turn_and_missing_tool_completion(self):
        fixture = Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/pi-rpc-v0.85.1/gateway-read.jsonl.gz'
        events = [json.loads(line) for line in gzip.decompress(fixture.read_bytes()).split(b'\n') if line]
        tools = [event for event in events if event.get('type') == 'tool_execution_end']
        self.assertEqual(len(tools), 1)
        self.assertEqual(tools[0]['toolName'], 'read')
        self.assertFalse(tools[0]['isError'])
        self.assertIn('HAND_GATEWAY_READ_PROBE', tools[0]['result']['content'][0]['text'])
        for omit_completion in (False, True):
            with self.subTest(omit_completion=omit_completion):
                observer = PiRunObserver('prompt-1', 'stats-1', 'anthropic', 'claude-sonnet-4-5-20250929')
                stream = encode([e for e in events if not (omit_completion and e.get('type') == 'tool_execution_end')])
                for offset in range(0, len(stream), 71):
                    observer.feed(stream[offset:offset+71])
                result = observer.finish()
                self.assertEqual(result['status'], 'incomplete' if omit_completion else 'observed_unverified', result)
                if not omit_completion:
                    self.assertEqual(result['outcome'], 'completed')
                    self.assertEqual(result['assistant_messages'], 2)

    def test_recorded_released_runtime_streams(self):
        fixtures = Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/pi-rpc-v0.85.1'
        for name, outcome, attempts in [('success', 'completed', 1), ('retry', 'completed', 2), ('abort', 'cancelled', 1)]:
            with self.subTest(name=name):
                observer = PiRunObserver('prompt-1', 'stats-1', 'hand-offline-fixture', 'offline-fixture')
                observer.feed(gzip.decompress((fixtures/(name+'.jsonl.gz')).read_bytes()))
                result = observer.finish()
                self.assertEqual(result['status'], 'observed_unverified', result)
                self.assertEqual(result['outcome'], outcome)
                self.assertEqual(result['assistant_messages'], attempts)

    def observer(self, **kwargs):
        return PiRunObserver('prompt-1', 'stats-1', 'fixture', 'frozen-model', **kwargs)

    def result(self, events):
        observer = self.observer()
        observer.feed(encode(events))
        return observer.finish()

    def test_admission_and_agent_end_do_not_finish(self):
        for count in (1, 4):
            observer = self.observer()
            observer.feed(encode(transcript()[:count]))
            self.assertFalse(observer.ready_for_stats)
            self.assertEqual(observer.finish()['status'], 'incomplete')

    def test_full_settlement_and_correlated_stats(self):
        observer = self.observer()
        observer.feed(encode(transcript()[:-1]))
        self.assertTrue(observer.ready_for_stats)
        observer.feed(encode(transcript()[-1:]))
        result = observer.finish()
        self.assertEqual(result['status'], 'observed_unverified', result)
        self.assertEqual(result['outcome'], 'completed')
        self.assertEqual(result['statistics']['tokens']['total'], 20)
        self.assertFalse(observer.ready_for_stats)

    def test_retry_error_retained_until_eventual_settlement(self):
        events = transcript()
        events[2:4] = [assistant('error'), dict(type='agent_end', willRetry=True),
                       dict(type='auto_retry_start'), dict(type='agent_start'), assistant(),
                       dict(type='agent_end', willRetry=False), dict(type='auto_retry_end', success=True)]
        result = self.result(events)
        self.assertEqual(result['status'], 'observed_unverified', result)
        self.assertEqual(result['outcome'], 'completed')
        self.assertEqual(result['assistant_errors'], 1)
        self.assertEqual(result['assistant_messages'], 2)

    def test_unicode_separators_and_chunk_boundaries_preserve_records(self):
        raw = encode(transcript()).replace(b'\n', b'\r\n')
        observer = self.observer()
        for byte in raw:
            observer.feed(bytes([byte]))
        self.assertEqual(observer.finish()['status'], 'observed_unverified')

    def test_truncated_ambiguous_and_nonfinite_records_fail(self):
        for extra in [b'{"type":"agent_settled"}', b'{"type":"a","type":"b"}\n',
                      b'{"type":"unknown","cost":NaN}\n', b'\xff\n', b'\n']:
            observer = self.observer()
            observer.feed(encode(transcript()) + extra)
            self.assertEqual(observer.finish()['status'], 'incomplete')

    def test_missing_early_and_duplicate_stats_fail(self):
        events = transcript()
        for modified in [events[:-1], [events[-1]]+events[:-1], events+[events[-1]]]:
            self.assertEqual(self.result(modified)['status'], 'incomplete')
        other = copy.deepcopy(events)
        other[-1]['id'] = 'wrong-request'
        self.assertEqual(self.result(other)['status'], 'incomplete')

    def test_model_change_pending_tool_and_activity_after_settlement_fail(self):
        events = transcript()
        changed = copy.deepcopy(events); changed[2]['message']['model'] = 'other-model'
        pending = copy.deepcopy(events); pending.insert(2, dict(type='tool_execution_start', toolCallId='unfinished'))
        for modified in [changed, pending, events+[dict(type='agent_start')], events+[dict(type='agent_settled')]]:
            self.assertEqual(self.result(modified)['status'], 'incomplete')

    def test_joined_tool_and_cancelled_outcome(self):
        events = transcript()
        events[2:3] = [dict(type='tool_execution_start', toolCallId='tool-1'),
                       dict(type='tool_execution_end', toolCallId='tool-1'), assistant('aborted')]
        result = self.result(events)
        self.assertEqual(result['status'], 'observed_unverified', result)
        self.assertEqual(result['outcome'], 'cancelled')
        for stop in ('error', 'length', 'toolUse'):
            events[4] = assistant(stop)
            self.assertEqual(self.result(events)['outcome'], 'failed')

    def test_unknown_cost_is_not_zero_and_invalid_totals_fail(self):
        events = transcript(); del events[-1]['data']['cost']
        result = self.result(events)
        self.assertEqual(result['status'], 'observed_unverified')
        self.assertIsNone(result['statistics']['reported_cost_usd'])
        for value in (-1, True, 21):
            events[-1]['data']['tokens']['total'] = value
            self.assertEqual(self.result(events)['status'], 'incomplete')

    def test_output_limit_poisoning_and_finish_ownership(self):
        observer = self.observer(max_bytes=4)
        with self.assertRaises(ValueError): observer.feed(b'12345')
        self.assertEqual(observer.finish()['status'], 'incomplete')
        with self.assertRaises(ValueError): observer.feed(b'\n')


if __name__ == '__main__': unittest.main()

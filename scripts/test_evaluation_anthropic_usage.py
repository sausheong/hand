import copy
import hashlib
import json
import unittest

from evaluation_anthropic_usage import observe_usage, verify_observed_usage


def encode(events):
    return ''.join('event: '+event['type']+'\ndata: '+json.dumps(event,ensure_ascii=False)+'\n\n' for event in events).encode()


class UsageTests(unittest.TestCase):
    def setUp(self):
        self.usage=dict(input_tokens=5,output_tokens=1,cache_creation_input_tokens=3,cache_read_input_tokens=7)
        self.message=dict(type='message',id='msg_fixture',role='assistant',model='fixture',content=[],stop_reason=None,usage=self.usage)
        self.events=[dict(type='message_start',message=self.message),
            dict(type='content_block_start',index=0,content_block={'type':'text','text':''}),
            dict(type='content_block_delta',index=0,delta={'type':'text_delta','text':'Hello\u2028there'}),
            dict(type='content_block_stop',index=0),
            dict(type='message_delta',delta={'stop_reason':'end_turn'},usage={'output_tokens':9}),
            dict(type='message_stop')]

    def test_complete_stream_uses_cumulative_output_and_disjoint_input_counts(self):
        raw=encode(self.events);r=observe_usage(raw,'text/event-stream; charset=utf-8','fixture')
        self.assertEqual(r['status'],'usage_observed_unverified')
        self.assertEqual(r['input_tokens'],15);self.assertEqual(r['output_tokens'],9)
        self.assertEqual(r['uncached_input_tokens'],5);self.assertEqual(r['cache_read_input_tokens'],7)
        self.assertEqual(r['body_sha256'],hashlib.sha256(raw).hexdigest())
        self.assertEqual(observe_usage(raw.replace(b'\n',b'\r\n'),'text/event-stream','fixture')['input_tokens'],15)

    def test_nonstreaming_and_cache_breakdown(self):
        message=copy.deepcopy(self.message);message['stop_reason']='tool_use'
        message['usage'].update(output_tokens=9,cache_creation={'ephemeral_5m_input_tokens':1,'ephemeral_1h_input_tokens':2},service_tier='standard')
        message['content']=[dict(type='tool_use',id='tool',name='read',input={})]
        r=observe_usage(json.dumps(message).encode(),'application/json','fixture')
        self.assertEqual(r['status'],'usage_observed_unverified');self.assertEqual(r['input_tokens'],15)
        message['usage']['cache_creation']['ephemeral_1h_input_tokens']=3
        self.assertEqual(observe_usage(json.dumps(message).encode(),'application/json','fixture')['status'],'incomplete')

    def test_missing_counter_never_defaults_to_zero(self):
        for key in self.usage:
            events=copy.deepcopy(self.events);del events[0]['message']['usage'][key]
            with self.subTest(key=key):self.assertEqual(observe_usage(encode(events),'text/event-stream','fixture')['status'],'incomplete')
        events=copy.deepcopy(self.events);events[-2]['usage']={}
        self.assertEqual(observe_usage(encode(events),'text/event-stream','fixture')['status'],'incomplete')

    def test_truncation_duplicate_and_out_of_order_events_fail(self):
        variants=[self.events[:-1],self.events[1:],self.events[:3]+self.events[4:],
            self.events[:1]+self.events,self.events+self.events[-1:],self.events[:-1]+self.events[-2:]]
        for events in variants:
            with self.subTest(events=[e['type'] for e in events]):self.assertEqual(observe_usage(encode(events),'text/event-stream','fixture')['status'],'incomplete')
        for raw in [encode(self.events)[:-1],encode(self.events).replace(b'event: message_start',b'event: ping',1),
                    encode(self.events).replace(b'"input_tokens": 5',b'"input_tokens": 5, "input_tokens": 9',1)]:
            self.assertEqual(observe_usage(raw,'text/event-stream','fixture')['status'],'incomplete')

    def test_model_regression_unknown_billing_and_server_tools_fail(self):
        variants=[]
        events=copy.deepcopy(self.events);events[0]['message']['model']='other';variants.append(events)
        events=copy.deepcopy(self.events);events[-2]['usage']['input_tokens']=4;variants.append(events)
        for key,value in [('server_tool_use',{'web_search_requests':1}),('service_tier','priority'),('output_tokens',True),('cache_creation_input_tokens',-1)]:
            events=copy.deepcopy(self.events);events[0]['message']['usage'][key]=value;variants.append(events)
        events=copy.deepcopy(self.events);events[1]['content_block']['type']='server_tool_use';variants.append(events)
        for events in variants:self.assertEqual(observe_usage(encode(events),'text/event-stream','fixture')['status'],'incomplete')

    def test_sender_identity_and_encoding_must_match_before_parsing(self):
        raw=encode(self.events)
        observation=dict(status='response_received',http_status=200,bytes_received=len(raw),body_sha256=hashlib.sha256(raw).hexdigest(),response_headers={'content-type':'text/event-stream'})
        self.assertEqual(verify_observed_usage(observation,raw,'fixture')['status'],'usage_observed_unverified')
        for key,value in [('status','incomplete'),('http_status',500),('bytes_received',1),('body_sha256','a'*64)]:
            with self.assertRaises(ValueError):verify_observed_usage(dict(observation,**{key:value}),raw,'fixture')
        observation['response_headers']['content-encoding']='gzip'
        with self.assertRaises(ValueError):verify_observed_usage(observation,raw,'fixture')


if __name__ == '__main__':unittest.main()

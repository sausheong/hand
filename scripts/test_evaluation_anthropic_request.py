import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from evaluation_anthropic_request import admit_request, validate_request
from evaluation_budget import EvaluationBudget


class AnthropicRequestTests(unittest.TestCase):
    def setUp(self):
        self.request = dict(model='fixture', max_tokens=100, stream=True,
            messages=[dict(role='user', content='Review this code')])

    def validate(self, request=None):
        body = json.dumps(self.request if request is None else request).encode()
        return validate_request('POST', '/v1/messages', body, 'fixture')

    def test_coding_tools_cache_and_thinking_preserved(self):
        self.request.update(system=[dict(type='text', text='Coding agent', cache_control=dict(type='ephemeral', ttl='1h'))],
            tools=[dict(name='edit', input_schema={'type':'object'}, strict=True, eager_input_streaming=True)],
            tool_choice=dict(type='auto', disable_parallel_tool_use=False),
            thinking=dict(type='enabled', budget_tokens=50, display='summarized'))
        self.request['messages'] += [dict(role='assistant', content=[
            dict(type='thinking', thinking='Inspect first', signature='opaque'),
            dict(type='redacted_thinking', data='opaque'),
            dict(type='tool_use', id='id', name='edit', input={'type':'image', 'value':'ordinary tool input'})]),
            dict(role='user', content=[dict(type='tool_result', tool_use_id='id', content=[dict(type='text', text='done')], is_error=False)])]
        before = copy.deepcopy(self.request)
        self.assertEqual(self.validate(), 100)
        self.assertEqual(self.request, before)
        self.request['thinking'] = dict(type='adaptive', block_binding={'prefix_mismatch_behavior':'drop_block'})
        self.request['messages'].insert(0,dict(role='system', content=[], output_config={'effort':'high'}))
        self.assertEqual(self.validate(), 100)

    def test_billed_features_and_unknown_fields_rejected(self):
        for key, value in [('service_tier','priority'), ('speed','fast'), ('container',{}),
                           ('mcp_servers',[]), ('betas',['unknown']), ('unknown',True)]:
            request = dict(self.request, **{key:value})
            with self.subTest(key=key), self.assertRaises(ValueError): self.validate(request)
        for kind in ('image','document','server_tool_use','web_search_tool_result'):
            request = copy.deepcopy(self.request)
            request['messages'][0]['content'] = [{'type':kind, 'source':{}}]
            with self.subTest(kind=kind), self.assertRaises(ValueError): self.validate(request)
        request = dict(self.request, tools=[dict(type='web_search_20250305', name='search', input_schema={})])
        with self.assertRaises(ValueError): self.validate(request)
        request = copy.deepcopy(self.request)
        request['messages'][0]['content'] = [dict(type='tool_result',tool_use_id='id',content=[dict(type='image')])]
        with self.assertRaises(ValueError): self.validate(request)

    def test_routes_models_limits_and_thinking_budget_cannot_change(self):
        for method,target in [('GET','/v1/messages'),('POST','https://elsewhere/v1/messages'),
            ('POST','/v1/messages?beta=true&speed=fast'),('POST','/v1/messages/batches'),('POST','/v1/%6dessages')]:
            with self.assertRaises(ValueError): validate_request(method,target,json.dumps(self.request).encode(),'fixture')
        for key,value in [('model','other'),('max_tokens',True),('max_tokens',0),('max_tokens',1.5)]:
            with self.assertRaises(ValueError): self.validate(dict(self.request, **{key:value}))
        for thinking in [dict(type='enabled',budget_tokens=100),dict(type='enabled'),dict(type='adaptive',budget_tokens=10)]:
            with self.assertRaises(ValueError): self.validate(dict(self.request,thinking=thinking))

    def test_ambiguous_json_and_structural_limits_rejected(self):
        for body in [b'{"model":"one","model":"two"}', b'{"x":NaN}', b'{"x":1e999}',
            b'{"x":"\\ud800"}', b'{"x":"\xff"}', b'['*100+b'0'+b']'*100,
            b'{"nested":{"x":1,"x":2}}', b'x'*(16*1024*1024+1)]:
            with self.subTest(body=body[:40]), self.assertRaises(ValueError): validate_request('POST','/v1/messages',body,'fixture')

    def test_captured_pinned_pi_provider_requests(self):
        self.check_captured_requests('pi-requests-v0.85.1')

    def test_captured_local_harness_provider_requests(self):
        self.check_captured_requests('harness-requests-development-20260909')

    def check_captured_requests(self, directory):
        from urllib.parse import urlsplit
        root = Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata'/directory
        report = json.loads((root/'report.json').read_text())
        self.assertEqual(report['status'], 'requests_captured')
        self.assertEqual({c['name'] for c in report['cases']}, {'text','tool-roundtrip','thinking'})
        for case in report['cases']:
            self.assertEqual(len(case['captures']), 1)
            capture = case['captures'][0]
            raw = (root/capture['body']).read_bytes()
            self.assertEqual(hashlib.sha256(raw).hexdigest(), capture['body_sha256'])
            url = urlsplit(capture['url'])
            target = url.path+('?' + url.query if url.query else '')
            with self.subTest(case=case['name']):
                self.assertEqual(validate_request(capture['method'],target,raw,'claude-sonnet-4-5-20250929'),4096)

    def test_admission_binds_exact_bytes_and_rejects_without_reserving(self):
        contract = dict(schema_version=1, provider='anthropic', model='fixture',
            source_sha256='a'*64,billing='text_tokens_with_partitioned_cache',context_tokens=1000,
            max_output_tokens=100,max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
        with tempfile.TemporaryDirectory() as directory:
            ledger=EvaluationBudget(Path(directory).resolve()/'ledger.sqlite','b'*64,3000000,clock=lambda:100)
            try:
                ledger.add_run('run',10,10000,1000,3000000,60)
                body=json.dumps(self.request,indent=2).encode()
                receipt=admit_request(ledger,'run','one','POST','/v1/messages',body,contract,'fixture')
                self.assertEqual(receipt['request_sha256'],hashlib.sha256(body).hexdigest())
                self.assertEqual(ledger.binding('one')['request_sha256'],receipt['request_sha256'])
                bad=dict(self.request,max_tokens=101)
                with self.assertRaises(ValueError): admit_request(ledger,'run','two','POST','/v1/messages',json.dumps(bad).encode(),contract,'fixture')
                bad=dict(self.request,service_tier='priority')
                with self.assertRaises(ValueError): admit_request(ledger,'run','two','POST','/v1/messages',json.dumps(bad).encode(),contract,'fixture')
                self.assertEqual(ledger.totals()['requests'],1)
            finally: ledger.close()


if __name__ == '__main__': unittest.main()

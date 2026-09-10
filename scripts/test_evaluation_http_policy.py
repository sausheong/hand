import copy
import json
from pathlib import Path
import unittest

from evaluation_http_policy import validate_headers


class HTTPPolicyTests(unittest.TestCase):
    def setUp(self):
        self.token = 'test_only_'+'a'*32
        self.authority = 'gateway.local:1234'
        self.policy = [[], ['interleaved-thinking-2025-05-14']]
        self.headers = [('Host',self.authority),('Content-Length','10'),('Content-Type','application/json'),
            ('Anthropic-Version','2023-06-01'),('X-Api-Key',self.token)]

    def validate(self, headers=None):
        return validate_headers(self.headers if headers is None else headers,10,self.token,self.authority,self.policy)

    def test_credentials_and_hop_headers_never_forwarded(self):
        result = self.validate(self.headers+[('Connection','keep-alive'),('User-Agent','actual SDK')])
        self.assertEqual(result['headers']['user-agent'],'actual SDK')
        for key in ['host','content-length','x-api-key','connection']: self.assertNotIn(key,result['headers'])
        self.assertNotIn(self.token,json.dumps(result))
        self.assertFalse(result['credential_forwarded'])

    def test_duplicate_names_smuggling_and_route_headers_rejected(self):
        for extra in [('content-LENGTH','10'),('Transfer-Encoding','chunked'),('Content-Encoding','gzip'),
            ('Forwarded','host=other'),('X-Forwarded-Host','other'),('Authorization','Bearer other'),
            ('Anthropic-Service-Tier','priority'),('X-Unknown','value'),('Accept','application/json\r\nX-Injected:1')]:
            with self.subTest(extra=extra),self.assertRaises(ValueError): self.validate(self.headers+[extra])
        for key,value in [('Host','elsewhere'),('Content-Length','010'),('Content-Length','11'),
            ('Content-Length','10,10'),('X-Api-Key','wrong'),('Anthropic-Version','2024-01-01')]:
            headers=[(k,value if k==key else v) for k,v in self.headers]
            with self.subTest(key=key,value=value),self.assertRaises(ValueError): self.validate(headers)
        with self.assertRaises(ValueError): self.validate(self.headers+[('Connection','x-api-key')])

    def test_beta_combinations_require_explicit_review(self):
        headers=self.headers+[('Anthropic-Beta','interleaved-thinking-2025-05-14')]
        self.assertEqual(self.validate(headers)['beta_selection'],self.policy[1])
        for value in ['unknown-beta','interleaved-thinking-2025-05-14,unknown-beta','',
            'interleaved-thinking-2025-05-14,interleaved-thinking-2025-05-14']:
            with self.assertRaises(ValueError): self.validate(self.headers+[('Anthropic-Beta',value)])
        with self.assertRaises(ValueError): validate_headers(self.headers,10,self.token,self.authority,[self.policy[1]])

    def test_actual_sdk_headers_with_http_framing_and_run_key(self):
        root=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata'
        for folder in ['pi-requests-v0.85.1','harness-requests-development-20260909']:
            report=json.loads((root/folder/'report.json').read_text())
            for case in report['cases']:
                capture=case['captures'][0];pairs=[]
                for key,values in capture['headers'].items():
                    for value in values if isinstance(values,list) else [values]: pairs.append((key,value))
                length=len((root/folder/capture['body']).read_bytes())
                pairs += [('Host',self.authority),('Content-Length',str(length)),('X-Api-Key',self.token)]
                before=copy.deepcopy(pairs)
                with self.subTest(folder=folder,case=case['name']):
                    result=validate_headers(pairs,length,self.token,self.authority,self.policy)
                    for key,value in pairs:
                        if key.lower() not in ('host','content-length','x-api-key'):
                            self.assertEqual(result['headers'][key.lower()],value)
                    self.assertEqual(before,pairs)

    def test_header_size_and_malformed_policy_fail(self):
        for extra in [('User-Agent','x'*4097),('Bad Name','value'),('User-Agent','\tvalue')]:
            with self.assertRaises(ValueError): self.validate(self.headers+[extra])
        for policy in [[],None,[['same','same']],[['bad beta']]]:
            with self.assertRaises(ValueError): validate_headers(self.headers,10,self.token,self.authority,policy)

    def test_actual_node_fetch_network_headers(self):
        root=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/pi-network-v0.85.1/headers.json'
        captures=json.loads(root.read_text());self.assertEqual(len(captures),3)
        for capture in captures:
            pairs=capture['headers']+[('x-api-key',self.token)]
            authority=next(v for k,v in pairs if k.lower()=='host')
            result=validate_headers(pairs,capture['body_length'],self.token,authority,self.policy)
            self.assertEqual(result['headers']['accept-language'],'*')
            self.assertEqual(result['headers']['sec-fetch-mode'],'cors')
        for key,value in [('accept-language','other'),('sec-fetch-mode','navigate')]:
            with self.assertRaises(ValueError):self.validate(self.headers+[(key,value)])

    def test_actual_go_network_headers(self):
        root=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata/harness-network-development-20260909/headers.json'
        captures=json.loads(root.read_text());self.assertEqual(len(captures),3)
        for capture in captures:
            pairs=capture['headers']+[('x-api-key',self.token)]
            authority=next(v for k,v in pairs if k.lower()=='host')
            result=validate_headers(pairs,capture['body_length'],self.token,authority,self.policy)
            self.assertEqual(result['headers']['x-stainless-lang'],'go')
            self.assertEqual(result['headers']['accept-encoding'],'gzip')


if __name__ == '__main__': unittest.main()

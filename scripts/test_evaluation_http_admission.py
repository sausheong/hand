import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from urllib.parse import urlsplit

from evaluation_budget import BudgetExceeded, EvaluationBudget
from evaluation_http_policy import admit_http_request


class HTTPAdmissionTests(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        self.path=Path(temp.name).resolve()/'budget.sqlite'
        self.ledger=EvaluationBudget(self.path,'a'*64,200000000,clock=lambda:100)
        self.addCleanup(self.ledger.close)
        self.ledger.add_run('run',10,100000,50000,200000000,60)
        self.token='fixture_'+'x'*32;self.policy=[[],['interleaved-thinking-2025-05-14']]
        self.contract=dict(schema_version=1,provider='anthropic',model='claude-sonnet-4-5-20250929',source_sha256='b'*64,
            billing='text_tokens_with_partitioned_cache',context_tokens=10000,max_output_tokens=4096,
            max_request_fee_usd='0',price_sets=[dict(input='1',output='2',cache_read='1',cache_write='1')])
        self.body=json.dumps(dict(model=self.contract['model'],max_tokens=4096,messages=[dict(role='user',content='Read code')])).encode()
        self.headers=[('Host','gateway:123'),('Content-Type','application/json'),('Anthropic-Version','2023-06-01'),
            ('X-Api-Key',self.token),('Content-Length',str(len(self.body)))]

    def admit(self, identity='one', headers=None, body=None, target='/v1/messages'):
        return admit_http_request(self.ledger,'run',identity,'POST',target,
            self.headers if headers is None else headers,self.body if body is None else body,
            self.token,'gateway:123',self.policy,self.contract,self.contract['model'])

    def test_all_captured_provider_requests_have_durable_joint_identity(self):
        root=Path(__file__).resolve().parents[1]/'docs/acceptance/evaluation/testdata'
        for folder in ['pi-requests-v0.85.1','harness-requests-development-20260909']:
            report=json.loads((root/folder/'report.json').read_text())
            for case in report['cases']:
                c=case['captures'][0];body=(root/folder/c['body']).read_bytes();u=urlsplit(c['url'])
                headers=[]
                for k,vs in c['headers'].items():
                    headers.extend((k,v) for v in (vs if isinstance(vs,list) else [vs]))
                headers += [('Host','gateway:123'),('Content-Length',str(len(body))),('X-Api-Key',self.token)]
                identity=folder+'-'+case['name'];receipt=self.admit(identity,headers,body,u.path+('?' +u.query if u.query else ''))
                stored=self.ledger.transport(identity)
                self.assertEqual(stored['body_sha256'],c['body_sha256'])
                canonical=json.dumps(stored,sort_keys=True,separators=(',',':')).encode()
                self.assertEqual(receipt['transport_sha256'],hashlib.sha256(canonical).hexdigest())
                self.assertEqual(stored['origin'],'https://api.anthropic.com')
                self.assertNotIn(self.token,json.dumps(stored))
        self.assertEqual(self.ledger.totals()['requests'],6)

    def test_denied_header_body_and_revoked_run_leave_no_partial_records(self):
        with self.assertRaises(ValueError):self.admit(headers=self.headers+[('Authorization','other')])
        bad=json.dumps(dict(model='other',max_tokens=4096,messages=[dict(role='user',content='Read code')])).encode()
        headers=[(k,str(len(bad)) if k=='Content-Length' else v) for k,v in self.headers]
        with self.assertRaises(ValueError):self.admit(headers=headers,body=bad)
        self.ledger.revoke('run')
        with self.assertRaises(BudgetExceeded):self.admit()
        for table in ('reservations','reservation_bindings','transport_admissions'):
            self.assertEqual(self.ledger.db.execute('SELECT count(*) FROM '+table).fetchone()[0],0)

    def test_transport_write_failure_rolls_back_quote_and_budget(self):
        self.ledger.db.execute("CREATE TRIGGER fail_transport BEFORE INSERT ON transport_admissions BEGIN SELECT RAISE(ABORT,'injected'); END")
        with self.assertRaises(sqlite3.IntegrityError):self.admit()
        for table in ('reservations','reservation_bindings','transport_admissions'):
            self.assertEqual(self.ledger.db.execute('SELECT count(*) FROM '+table).fetchone()[0],0)

    def test_crash_preserves_transport_and_duplicate_cannot_replay(self):
        fixture=self.path.parent/'input.json'
        fixture.write_text(json.dumps(dict(headers=self.headers,body=self.body.decode(),token=self.token,policy=self.policy,contract=self.contract)))
        code="""import json,os,sys
from evaluation_budget import EvaluationBudget
from evaluation_http_policy import admit_http_request
ledger=EvaluationBudget(sys.argv[1],'a'*64,clock=lambda:100)
x=json.load(open(sys.argv[2]))
admit_http_request(ledger,'run','crashed','POST','/v1/messages',x['headers'],x['body'].encode(),x['token'],'gateway:123',x['policy'],x['contract'],x['contract']['model'])
os._exit(77)
"""
        child=subprocess.run([sys.executable,'-c',code,str(self.path),str(fixture)],cwd=Path(__file__).parent,
            env={'PATH':os.environ['PATH'],'PYTHONDONTWRITEBYTECODE':'1'},timeout=10)
        self.assertEqual(child.returncode,77)
        reopened=EvaluationBudget(self.path,'a'*64,clock=lambda:100);self.addCleanup(reopened.close)
        self.assertEqual(reopened.transport('crashed')['body_sha256'],hashlib.sha256(self.body).hexdigest())
        self.assertEqual(reopened.totals()['uncertain'],1)
        with self.assertRaises(ValueError):self.admit('crashed')
        self.assertEqual(self.ledger.totals()['requests'],1)

    def test_transport_body_mismatch_blocks_recovery(self):
        self.admit();stored=self.ledger.transport('one');stored['body_sha256']='c'*64
        self.ledger.db.execute('UPDATE transport_admissions SET transport_json=?',(json.dumps(stored),))
        with self.assertRaises(ValueError):self.admit('two')
        with self.assertRaises(ValueError):EvaluationBudget(self.path,'a'*64)


if __name__ == '__main__':unittest.main()

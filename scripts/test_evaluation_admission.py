"""Offline admission provenance tests; no actual provider or spending approval."""
import hashlib
from contextlib import closing
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest

from evaluation_budget import BudgetExceeded, EvaluationBudget


class AdmissionTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.path = Path(temp.name).resolve()/'ledger.sqlite'
        self.ledger = EvaluationBudget(self.path, 'a'*64, 1000000, clock=lambda:100)
        self.addCleanup(self.ledger.close)
        self.ledger.add_run('run', 2, 100, 10, 1000000, 60)
        self.contract = dict(schema_version=1, provider='fixture', model='model',
            source_sha256='b'*64, billing='text_tokens_with_partitioned_cache',
            context_tokens=10, max_output_tokens=4, max_request_fee_usd='0',
            price_sets=[dict(input='1', output='2', cache_read='1', cache_write='1')])
        self.body = b'{"model":"model","max_tokens":4,"messages":[]}'

    def admit(self, identity='request', body=None):
        return self.ledger.reserve_quoted('run', identity, self.body if body is None else body,
                                          self.contract, 'fixture', 'model', 4)

    def test_frozen_binding_survives_restart_and_never_allows_replay(self):
        receipt = self.admit()
        self.contract['price_sets'][0]['input'] = '999'
        reopened = EvaluationBudget(self.path, 'a'*64, clock=lambda:100)
        self.addCleanup(reopened.close)
        binding = reopened.binding('request')
        self.assertEqual(binding['request_sha256'], hashlib.sha256(self.body).hexdigest())
        self.assertEqual(hashlib.sha256(binding['contract_json'].encode()).hexdigest(), receipt['contract_sha256'])
        self.assertNotIn(self.body.decode(), json.dumps(binding))
        with self.assertRaises(ValueError):
            reopened.reserve_quoted('run', 'request', b'different', json.loads(binding['contract_json']), 'fixture', 'model', 4)
        self.assertEqual(reopened.totals()['requests'], 1)

    def test_binding_insert_failure_rolls_back_reservation(self):
        self.ledger.db.execute("CREATE TRIGGER fail_binding BEFORE INSERT ON reservation_bindings BEGIN SELECT RAISE(ABORT, 'injected storage failure'); END")
        with self.assertRaises(sqlite3.IntegrityError): self.admit()
        self.assertEqual(self.ledger.totals()['requests'], 0)
        self.assertEqual(self.ledger.db.execute('SELECT count(*) FROM reservation_bindings').fetchone()[0], 0)

    def test_denied_and_invalid_requests_leave_no_admission(self):
        for body in (b'', bytearray(b'body'), 'body', b'x'*(16*1024*1024+1)):
            with self.assertRaises(ValueError): self.admit(body=body)
        self.ledger.revoke('run')
        with self.assertRaises(BudgetExceeded): self.admit()
        self.assertEqual(self.ledger.totals()['requests'], 0)
        with self.assertRaises(ValueError): self.ledger.binding('request')

    def test_corrupt_quote_blocks_existing_connection_and_reopen(self):
        self.admit()
        with closing(sqlite3.connect(self.path)) as db:
            with db:
                db.execute('UPDATE reservations SET cost_bound=1')
        with self.assertRaises(ValueError): self.admit('next')
        with self.assertRaises(ValueError): EvaluationBudget(self.path, 'a'*64)

    def test_process_crash_preserves_binding_and_reservation_together(self):
        contract = self.path.parent/'contract.json'
        contract.write_text(json.dumps(self.contract))
        code = """import json,os,sys
from evaluation_budget import EvaluationBudget
ledger=EvaluationBudget(sys.argv[1], 'a'*64, clock=lambda:100)
ledger.reserve_quoted('run','crashed',b'exact outbound bytes',json.load(open(sys.argv[2])),'fixture','model',4)
os._exit(77)
"""
        child = subprocess.run([sys.executable, '-c', code, str(self.path), str(contract)],
            cwd=Path(__file__).parent, env={'PATH':os.environ['PATH'], 'PYTHONDONTWRITEBYTECODE':'1'}, timeout=10)
        self.assertEqual(child.returncode, 77)
        reopened = EvaluationBudget(self.path, 'a'*64, clock=lambda:100)
        self.addCleanup(reopened.close)
        self.assertEqual(reopened.binding('crashed')['request_sha256'], hashlib.sha256(b'exact outbound bytes').hexdigest())
        self.assertEqual(reopened.totals()['uncertain'], 1)
        self.assertEqual(reopened.totals()['cost_nanos'], 18000)


if __name__ == '__main__': unittest.main()

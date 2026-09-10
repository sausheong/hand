import os
from contextlib import closing
from pathlib import Path
import subprocess
import sqlite3
import sys
import tempfile
import threading
import unittest

from evaluation_budget import BudgetExceeded, EvaluationBudget


class BudgetTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.path = Path(temp.name).resolve()/'budget.sqlite'
        self.now = 100
        self.ledger = EvaluationBudget(self.path, 'a'*64, 10, clock=lambda: self.now)
        self.addCleanup(self.ledger.close)
        self.ledger.add_run('run', 3, 20, 20, 10, 60)

    def test_pending_and_uncertain_reservations_survive_reopen(self):
        self.ledger.reserve('run', 'first', 10, 10, 7)
        self.ledger.mark_uncertain('first')
        other = EvaluationBudget(self.path, 'a'*64, clock=lambda: self.now)
        self.addCleanup(other.close)
        with self.assertRaises(BudgetExceeded): other.reserve('run', 'second', 1, 1, 4)
        self.assertEqual(other.totals()['cost_nanos'], 7)
        self.assertEqual(other.totals()['uncertain'], 1)

    def test_settlement_refunds_only_verified_difference_and_is_idempotent(self):
        self.ledger.reserve('run', 'first', 10, 10, 7)
        self.ledger.settle('first', 3, 2, 2, 'b'*64)
        self.ledger.settle('first', 3, 2, 2, 'b'*64)
        self.ledger.reserve('run', 'second', 1, 1, 8)
        self.assertEqual(self.ledger.totals()['cost_nanos'], 10)
        with self.assertRaises(ValueError): self.ledger.reserve('run', 'first', 0, 0, 0)

    def test_each_limit_and_expiry_prevent_admission(self):
        for run, requests, inputs, outputs, cost, reservation in [
            ('request', 1, 10, 10, 10, (0, 0, 0)),
            ('input', 5, 1, 10, 10, (1, 0, 0)),
            ('output', 5, 10, 1, 10, (0, 1, 0)),
            ('cost', 5, 10, 10, 1, (0, 0, 1))]:
            self.ledger.add_run(run, requests, inputs, outputs, cost, 1)
            self.ledger.reserve(run, run+'-first', *reservation)
            with self.assertRaises(BudgetExceeded): self.ledger.reserve(run, run+'-second', *reservation)
        self.now = 160
        with self.assertRaises(BudgetExceeded): self.ledger.reserve('run', 'expired', 0, 0, 0)

    def test_revoke_stops_new_calls_without_refunding_pending_work(self):
        self.ledger.reserve('run', 'first', 1, 1, 7)
        self.ledger.revoke('run')
        with self.assertRaises(BudgetExceeded): self.ledger.reserve('run', 'next', 0, 0, 0)
        self.assertEqual(self.ledger.totals()['cost_nanos'], 7)

    def test_concurrent_runs_cannot_double_spend_global_budget(self):
        self.ledger.add_run('other', 3, 20, 20, 10, 60)
        barrier = threading.Barrier(2)
        outcomes = []
        def reserve(run):
            ledger = EvaluationBudget(self.path, 'a'*64, clock=lambda: self.now)
            try:
                barrier.wait(timeout=5)
                ledger.reserve(run, run+'-call', 1, 1, 7)
                outcomes.append('admitted')
            except BudgetExceeded:
                outcomes.append('denied')
            finally:
                ledger.close()
        threads = [threading.Thread(target=reserve, args=(run,)) for run in ('run', 'other')]
        for thread in threads: thread.start()
        for thread in threads:
            thread.join(timeout=10)
            self.assertFalse(thread.is_alive())
        self.assertCountEqual(outcomes, ['admitted', 'denied'])
        self.assertEqual(self.ledger.totals()['cost_nanos'], 7)

    def test_committed_reservation_survives_process_crash(self):
        code = ("import os,sys; from evaluation_budget import EvaluationBudget; "
                "ledger=EvaluationBudget(sys.argv[1], 'a'*64, clock=lambda:100); "
                "ledger.reserve('run','crashed',1,1,9); os._exit(77)")
        process = subprocess.run([sys.executable, '-c', code, str(self.path)],
                                 cwd=Path(__file__).parent, timeout=10,
                                 env={'PATH':os.environ['PATH'], 'PYTHONDONTWRITEBYTECODE':'1'})
        self.assertEqual(process.returncode, 77)
        with self.assertRaises(BudgetExceeded): self.ledger.reserve('run', 'next', 0, 0, 2)
        self.assertEqual(self.ledger.totals()['uncertain'], 1)

    def test_misquoted_or_conflicting_usage_poisons_future_admission(self):
        self.ledger.reserve('run', 'first', 1, 1, 2)
        with self.assertRaises(BudgetExceeded): self.ledger.settle('first', 1, 1, 3, 'b'*64)
        reopened = EvaluationBudget(self.path, 'a'*64, clock=lambda: self.now)
        self.addCleanup(reopened.close)
        with self.assertRaises(BudgetExceeded): reopened.reserve('run', 'next', 0, 0, 0)
        with self.assertRaises(BudgetExceeded): reopened.settle('first', 1, 1, 4, 'c'*64)
        self.assertEqual(reopened.totals()['cost_nanos'], 3)
        self.assertEqual([v['cost_nanos'] for v in reopened.violations()], [3, 4])
        self.assertEqual(reopened.violations()[-1]['evidence'], 'c'*64)

    def test_manifest_mismatch_recreation_and_invalid_quantities_fail(self):
        with self.assertRaises(ValueError): EvaluationBudget(self.path, 'c'*64)
        with self.assertRaises(FileExistsError): EvaluationBudget(self.path, 'a'*64, 100)
        for value in (True, -1, 0.1, float('nan'), 1 << 63):
            with self.assertRaises(ValueError): self.ledger.reserve('run', 'invalid', 1, 1, value)
        self.assertEqual(self.ledger.totals()['requests'], 0)

    def test_observed_expiry_cannot_be_reversed_by_clock_rollback(self):
        self.now = 160
        with self.assertRaises(BudgetExceeded): self.ledger.reserve('run', 'expired', 0, 0, 0)
        self.now = 100
        reopened = EvaluationBudget(self.path, 'a'*64, clock=lambda: self.now)
        self.addCleanup(reopened.close)
        with self.assertRaises(BudgetExceeded): reopened.reserve('run', 'revived', 0, 0, 0)
        self.assertEqual(reopened.totals()['requests'], 0)

    def test_corrupt_reservation_cannot_create_budget_on_reopen(self):
        self.ledger.reserve('run', 'first', 1, 1, 7)
        self.ledger.close()
        with closing(sqlite3.connect(self.path)) as database:
            with database:
                database.execute('PRAGMA ignore_check_constraints=ON')
                database.execute('UPDATE reservations SET cost_bound=-7')
        with self.assertRaises(ValueError):
            reopened = EvaluationBudget(self.path, 'a'*64)
            self.addCleanup(reopened.close)

    def test_malformed_state_blocks_existing_connection_admission(self):
        self.ledger.reserve('run', 'first', 1, 1, 7)
        for corrupt, restore in [
            ("UPDATE reservations SET cost_bound=-7", "UPDATE reservations SET cost_bound=7"),
            ("UPDATE reservations SET state='lost'", "UPDATE reservations SET state='pending'"),
            ("UPDATE reservations SET actual_cost=0", "UPDATE reservations SET actual_cost=NULL"),
            ("UPDATE reservations SET run_id='missing'", "UPDATE reservations SET run_id='run'")]:
            with self.subTest(corrupt=corrupt):
                database = sqlite3.connect(self.path)
                try:
                    database.execute(corrupt); database.commit()
                    with self.assertRaises(ValueError): self.ledger.reserve('run', 'new', 0, 0, 0)
                    database.execute(restore); database.commit()
                finally:
                    database.close()
        self.assertEqual(self.ledger.totals()['requests'], 1)


if __name__ == '__main__': unittest.main()

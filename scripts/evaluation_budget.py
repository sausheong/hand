"""Durable evaluation reservations, not spending authorisation or a gateway.

Trusted gateway code must establish request upper bounds, validate approval and
keep this ledger outside agent access. Money is integer billionths of a USD.
"""
from contextlib import contextmanager
import os
import math
import hashlib
import json
from pathlib import Path
import re
import sqlite3
import time


class BudgetExceeded(ValueError):
    pass


def integer(value, positive=False):
    if type(value) is not int or not (1 if positive else 0) <= value <= (1 << 63)-1:
        raise ValueError('bounded nonnegative integer required')
    return value


def identity(value):
    if not isinstance(value, str) or not 1 <= len(value) <= 256 or '\0' in value:
        raise ValueError('nonempty bounded identity required')
    return value


class EvaluationBudget:
    def __init__(self, path, manifest_sha256, total_cost_nanos=None, clock=time.time):
        path = Path(path)
        if not isinstance(manifest_sha256, str) or not re.fullmatch('[a-f0-9]{64}', manifest_sha256):
            raise ValueError('exact manifest digest required')
        if not path.is_absolute() or path != path.resolve() or not path.parent.is_dir():
            raise ValueError('canonical absolute ledger path required')
        if path.parent.stat().st_mode & 0o077:
            raise ValueError('ledger parent must be private')
        if total_cost_nanos is not None:
            integer(total_cost_nanos)
            fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
            os.close(fd)
        if not path.is_file() or path.stat().st_mode & 0o077:
            raise ValueError('private regular ledger file required')
        self.clock = clock
        self.manifest_sha256 = manifest_sha256
        self.ready = False
        self.db = sqlite3.connect(path, isolation_level=None, timeout=10)
        self.db.row_factory = sqlite3.Row
        self.db.execute('PRAGMA synchronous=FULL')
        self.db.execute('PRAGMA foreign_keys=ON')
        try:
            if total_cost_nanos is not None:
                with self.transaction():
                    self.db.execute('CREATE TABLE budget (manifest TEXT NOT NULL, cap INTEGER NOT NULL, poisoned INTEGER NOT NULL DEFAULT 0)')
                    self.db.execute('INSERT INTO budget(manifest,cap) VALUES (?,?)', (manifest_sha256, total_cost_nanos))
                    self.db.execute('CREATE TABLE runs (id TEXT PRIMARY KEY, requests INTEGER NOT NULL, input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL, cost INTEGER NOT NULL, expires REAL NOT NULL, revoked INTEGER NOT NULL DEFAULT 0)')
                    self.db.execute('CREATE TABLE reservations (id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(id), input_bound INTEGER NOT NULL, output_bound INTEGER NOT NULL, cost_bound INTEGER NOT NULL, state TEXT NOT NULL, actual_input INTEGER, actual_output INTEGER, actual_cost INTEGER, evidence TEXT)')
                    self.db.execute('CREATE INDEX reservation_run ON reservations(run_id)')
                    self.db.execute('CREATE TABLE reservation_bindings (request_id TEXT PRIMARY KEY REFERENCES reservations(id), request_sha256 TEXT NOT NULL, contract_json TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL)')
                    self.db.execute('CREATE TABLE transport_admissions (request_id TEXT PRIMARY KEY REFERENCES reservation_bindings(request_id), transport_json TEXT NOT NULL, dispatched INTEGER NOT NULL DEFAULT 0)')
                    self.db.execute('CREATE TABLE violations (request_id TEXT NOT NULL, reason TEXT NOT NULL, input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL, cost_nanos INTEGER NOT NULL, evidence TEXT NOT NULL)')
            metadata = self.db.execute('SELECT * FROM budget').fetchall()
            if len(metadata) != 1 or metadata[0]['manifest'] != manifest_sha256:
                raise ValueError('ledger manifest does not match frozen schedule')
            self.check_state()
            self.ready = True
        except Exception:
            self.db.close()
            raise

    @contextmanager
    def transaction(self):
        self.db.execute('BEGIN IMMEDIATE')
        try:
            if self.ready: self.check_state()
            yield
        except Exception:
            self.db.execute('ROLLBACK')
            raise
        else:
            self.db.execute('COMMIT')

    def close(self):
        self.db.close()

    def check_state(self):
        if [row[0] for row in self.db.execute('PRAGMA quick_check')] != ['ok'] or self.db.execute('PRAGMA foreign_key_check').fetchone():
            raise ValueError('ledger integrity check failed')
        metadata = self.db.execute('SELECT * FROM budget').fetchall()
        if len(metadata) != 1 or metadata[0]['manifest'] != self.manifest_sha256 or metadata[0]['poisoned'] not in (0, 1):
            raise ValueError('invalid ledger metadata')
        integer(metadata[0]['cap'])
        for row in self.db.execute('SELECT * FROM runs'):
            identity(row['id'])
            for key in ('requests', 'input_tokens', 'output_tokens'): integer(row[key], True)
            integer(row['cost'])
            if (type(row['expires']) not in (float, int) or not math.isfinite(row['expires'])
                    or row['expires'] <= 0 or row['revoked'] not in (0, 1)):
                raise ValueError('invalid persisted run limits')
        for row in self.db.execute('SELECT * FROM reservations'):
            identity(row['id']); identity(row['run_id'])
            for key in ('input_bound', 'output_bound', 'cost_bound'): integer(row[key])
            values = [row[key] for key in ('actual_input', 'actual_output', 'actual_cost')]
            if row['state'] == 'settled':
                for value in values: integer(value)
                if not isinstance(row['evidence'], str) or not re.fullmatch('[a-f0-9]{64}', row['evidence']):
                    raise ValueError('invalid settled evidence digest')
            elif row['state'] not in ('pending', 'uncertain') or any(value is not None for value in values+[row['evidence']]):
                raise ValueError('invalid reservation transition data')

        from evaluation_quote import quote_request
        for row in self.db.execute('SELECT b.*, r.input_bound, r.output_bound, r.cost_bound FROM reservation_bindings b JOIN reservations r ON r.id=b.request_id'):
            if not re.fullmatch('[a-f0-9]{64}', row['request_sha256']):
                raise ValueError('invalid admitted request digest')
            quote = quote_request(json.loads(row['contract_json']), row['provider'], row['model'], row['output_bound'])
            if any(quote[key] != row[key] for key in ('input_bound', 'output_bound', 'cost_bound')):
                raise ValueError('stored quote differs from admitted bounds')
        from evaluation_http_policy import validate_transport
        for row in self.db.execute('SELECT t.*, b.request_sha256, b.provider FROM transport_admissions t JOIN reservation_bindings b ON b.request_id=t.request_id'):
            transport = json.loads(row['transport_json'])
            if row['dispatched'] not in (0, 1):
                raise ValueError('invalid dispatch state')
            validate_transport(transport)
            if transport['body_sha256'] != row['request_sha256'] or row['provider'] != 'anthropic':
                raise ValueError('transport differs from quoted request')

    def add_run(self, run_id, requests, input_tokens, output_tokens, cost_nanos, timeout_seconds):
        identity(run_id)
        for value in (requests, input_tokens, output_tokens, timeout_seconds): integer(value, True)
        integer(cost_nanos)
        expires = self.clock()+timeout_seconds
        if not math.isfinite(expires) or expires <= 0: raise ValueError('finite run expiry required')
        with self.transaction():
            self.db.execute('INSERT INTO runs(id,requests,input_tokens,output_tokens,cost,expires) VALUES (?,?,?,?,?,?)',
                            (run_id, requests, input_tokens, output_tokens, cost_nanos, expires))

    def totals(self, run_id=None):
        query = 'SELECT * FROM reservations'
        rows = self.db.execute(query if run_id is None else query+' WHERE run_id=?',
                               () if run_id is None else (run_id,))
        total = dict(requests=0, input_tokens=0, output_tokens=0, cost_nanos=0, uncertain=0)
        for row in rows:
            total['requests'] += 1
            settled = row['state'] == 'settled'
            total['uncertain'] += not settled
            for key, actual, bound in [('input_tokens', 'actual_input', 'input_bound'),
                                       ('output_tokens', 'actual_output', 'output_bound'),
                                       ('cost_nanos', 'actual_cost', 'cost_bound')]:
                total[key] += row[actual] if settled else row[bound]
        return total

    def reserve(self, run_id, request_id, input_bound, output_bound, cost_bound):
        return self._reserve(run_id, request_id, input_bound, output_bound, cost_bound)

    def reserve_quoted(self, run_id, request_id, request_bytes, contract, provider, model, max_output_tokens, transport=None):
        """Bind exact outbound body bytes and a complete quote atomically.

        The provider adapter must validate the body, endpoint, model and output
        limit before calling, then forward these same bytes only after return.
        No body, credential, approval or billing evidence is stored here.
        """
        from evaluation_quote import quote_request
        if type(request_bytes) is not bytes or not 1 <= len(request_bytes) <= 16 << 20:
            raise ValueError('bounded immutable outbound request bytes required')
        frozen = json.dumps(contract, sort_keys=True, separators=(',', ':'), allow_nan=False)
        quote = quote_request(json.loads(frozen), provider, model, max_output_tokens)
        binding = (hashlib.sha256(request_bytes).hexdigest(), frozen, provider, model)
        transport_json = None
        if transport is not None:
            from evaluation_http_policy import validate_transport
            transport_json = json.dumps(transport, sort_keys=True, separators=(',', ':'), allow_nan=False)
            transport = json.loads(transport_json)
            validate_transport(transport)
            if (provider != 'anthropic' or transport['body_sha256'] != binding[0]
                    or transport['body_length'] != len(request_bytes)):
                raise ValueError('transport does not match quoted request')
        self._reserve(run_id, request_id, quote['input_bound'], quote['output_bound'], quote['cost_bound'], binding, transport_json)
        receipt = dict(request_id=request_id, request_sha256=binding[0], **quote)
        if transport_json is not None:
            receipt['transport_sha256'] = hashlib.sha256(transport_json.encode()).hexdigest()
        return receipt

    def binding(self, request_id):
        """Return persisted admission identity for reconciliation, not replay permission."""
        self.check_state()
        row = self.db.execute('SELECT * FROM reservation_bindings WHERE request_id=?', (request_id,)).fetchone()
        if row is None: raise ValueError('no quoted admission for request')
        return dict(row)

    def transport(self, request_id):
        """Read immutable transport evidence; never permission to replay."""
        self.check_state()
        row = self.db.execute('SELECT transport_json FROM transport_admissions WHERE request_id=?', (request_id,)).fetchone()
        if row is None: raise ValueError('no HTTP admission for request')
        return json.loads(row['transport_json'])

    def claim_dispatch(self, run_id, request_id):
        """Commit a single-use dispatch claim before opening any upstream connection.

        A crash after this returns leaves the full reservation charged. It never
        permits retrying the same identity, even if no bytes reached upstream.
        """
        with self.transaction():
            if self.db.execute('SELECT poisoned FROM budget').fetchone()[0]:
                raise BudgetExceeded('ledger requires reconciliation')
            row = self.db.execute('SELECT r.run_id,r.state,t.dispatched,u.expires,u.revoked FROM reservations r JOIN transport_admissions t ON t.request_id=r.id JOIN runs u ON u.id=r.run_id WHERE r.id=?', (request_id,)).fetchone()
            if row is None or row['run_id'] != run_id: raise ValueError('unknown run admission')
            now = self.clock()
            if not math.isfinite(now): raise ValueError('finite clock required')
            expired = row['revoked'] or now >= row['expires']
            if expired:
                self.db.execute('UPDATE runs SET revoked=1 WHERE id=?', (run_id,))
            else:
                if row['state'] != 'pending' or row['dispatched']:
                    raise ValueError('request cannot be dispatched again')
                self.db.execute('UPDATE transport_admissions SET dispatched=1 WHERE request_id=?', (request_id,))
                remaining = row['expires']-now
        if expired: raise BudgetExceeded('run revoked or expired')
        return remaining

    def _reserve(self, run_id, request_id, input_bound, output_bound, cost_bound, binding=None, transport_json=None):
        identity(run_id); identity(request_id)
        for value in (input_bound, output_bound, cost_bound): integer(value)
        with self.transaction():
            budget = self.db.execute('SELECT * FROM budget').fetchone()
            if budget['poisoned']: raise BudgetExceeded('ledger requires reconciliation after inconsistent usage')
            run = self.db.execute('SELECT * FROM runs WHERE id=?', (run_id,)).fetchone()
            if run is None: raise ValueError('unknown scheduled run')
            now = self.clock()
            if not math.isfinite(now): raise ValueError('finite clock required')
            expired = run['revoked'] or now >= run['expires']
            if expired:
                self.db.execute('UPDATE runs SET revoked=1 WHERE id=?', (run_id,))
            else:
                if self.db.execute('SELECT 1 FROM reservations WHERE id=?', (request_id,)).fetchone():
                    raise ValueError('request already reserved; no permission to forward again')
                total = self.totals(run_id)
                if (total['requests']+1 > run['requests'] or total['input_tokens']+input_bound > run['input_tokens']
                        or total['output_tokens']+output_bound > run['output_tokens']
                        or total['cost_nanos']+cost_bound > run['cost']
                        or self.totals()['cost_nanos']+cost_bound > budget['cap']):
                    raise BudgetExceeded('request upper bound exceeds remaining budget')
                self.db.execute('INSERT INTO reservations(id,run_id,input_bound,output_bound,cost_bound,state) VALUES (?,?,?,?,?,?)',
                                (request_id, run_id, input_bound, output_bound, cost_bound, 'pending'))
                if binding is not None:
                    self.db.execute('INSERT INTO reservation_bindings VALUES (?,?,?,?,?)', (request_id,)+binding)
                if transport_json is not None:
                    self.db.execute('INSERT INTO transport_admissions(request_id,transport_json) VALUES (?,?)', (request_id, transport_json))
        if expired: raise BudgetExceeded('run revoked or expired')
        # Returning only after durable commit is the admission boundary.
        return request_id

    def mark_uncertain(self, request_id):
        with self.transaction():
            changed = self.db.execute("UPDATE reservations SET state='uncertain' WHERE id=? AND state='pending'", (request_id,)).rowcount
            if changed != 1: raise ValueError('pending request required')

    def settle(self, request_id, input_tokens, output_tokens, cost_nanos, evidence_sha256):
        for value in (input_tokens, output_tokens, cost_nanos): integer(value)
        if not isinstance(evidence_sha256, str) or not re.fullmatch('[a-f0-9]{64}', evidence_sha256):
            raise ValueError('usage evidence digest required')
        violation = None
        with self.transaction():
            row = self.db.execute('SELECT * FROM reservations WHERE id=?', (request_id,)).fetchone()
            if row is None: raise ValueError('unknown reservation')
            actual = (input_tokens, output_tokens, cost_nanos, evidence_sha256)
            if row['state'] == 'settled':
                previous = tuple(row[k] for k in ('actual_input', 'actual_output', 'actual_cost', 'evidence'))
                if actual == previous: return
                violation = 'conflicting usage for settled request'
            else:
                self.db.execute("UPDATE reservations SET state='settled',actual_input=?,actual_output=?,actual_cost=?,evidence=? WHERE id=?", actual+(request_id,))
                if input_tokens > row['input_bound'] or output_tokens > row['output_bound'] or cost_nanos > row['cost_bound']:
                    violation = 'actual usage exceeds reserved upper bound'
            if violation:
                self.db.execute('UPDATE budget SET poisoned=1')
                self.db.execute('INSERT INTO violations VALUES (?,?,?,?,?,?)',
                                (request_id, violation, input_tokens, output_tokens, cost_nanos, evidence_sha256))
        if violation: raise BudgetExceeded(violation)

    def violations(self):
        return [dict(row) for row in self.db.execute('SELECT * FROM violations ORDER BY rowid')]

    def revoke(self, run_id):
        with self.transaction():
            if self.db.execute('UPDATE runs SET revoked=1 WHERE id=?', (run_id,)).rowcount != 1:
                raise ValueError('unknown run')

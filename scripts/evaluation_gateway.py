"""Trusted run controller for the evaluation gateway; no listening HTTP server.

Construction requires an existing manifest-bound budget and explicit run settings.
It does not grant spending approval, create/reset budgets, discover credentials,
or establish the external network sandbox. Clients cannot select run settings.
"""
import hashlib
import hmac
import json
from pathlib import Path
import re
import threading
import uuid

from evaluation_budget import EvaluationBudget, identity
from evaluation_forward import forward, https_connection
from evaluation_http_policy import admit_http_request, validate_headers
from evaluation_quote import quote_request


class GatewayController:
    def __init__(self, ledger_path, manifest_sha256, evidence_directory, runs, provider_key,
                 gateway_authority, *, connection_factory=https_connection):
        self.path=Path(ledger_path)
        self.manifest=manifest_sha256
        if type(provider_key) is not str or not 1<=len(provider_key)<=4096 or any(ord(c)<33 or ord(c)>126 for c in provider_key):
            raise ValueError('explicit provider credential required')
        self.evidence=Path(evidence_directory)
        if (not self.evidence.is_absolute() or self.evidence != self.evidence.resolve()
                or not self.evidence.is_dir() or self.evidence.stat().st_mode & 0o077):
            raise ValueError('existing canonical private evidence directory required')
        if type(runs) is not list or not 1<=len(runs)<=1024:raise ValueError('bounded trusted run configuration required')
        frozen=json.loads(json.dumps(runs,allow_nan=False))
        self.runs={};tokens=set()
        for run in frozen:
            if type(run) is not dict or set(run)!={'id','token','model','contract','allowed_beta_sets','timeout_seconds'}:
                raise ValueError('complete trusted run configuration required')
            identity(run['id']);identity(run['model'])
            if type(run['token']) is not str or not re.fullmatch('[A-Za-z0-9_-]{32,256}',run['token']):raise ValueError('opaque run token required')
            if run['id'] in self.runs or run['token'] in tokens:raise ValueError('duplicate run identity or credential')
            if type(run['timeout_seconds']) not in (int,float) or not 0<run['timeout_seconds']<=3600:raise ValueError('bounded run request timeout required')
            quote_request(run['contract'],'anthropic',run['model'],run['contract']['max_output_tokens'])
            # Validate the explicit policy without requiring beta omission to be
            # allowed. Use its first permitted selection for this config check.
            beta_sets=run['allowed_beta_sets']
            if type(beta_sets) is not list or not beta_sets:raise ValueError('explicit beta policy required')
            if type(beta_sets[0]) is not list:raise ValueError('beta selection list required')
            probe=[('host',gateway_authority),('content-length','1'),('content-type','application/json'),
                   ('anthropic-version','2023-06-01'),('x-api-key',run['token'])]
            if beta_sets[0]:probe.append(('anthropic-beta',','.join(beta_sets[0])))
            validate_headers(probe,1,run['token'],gateway_authority,beta_sets)
            self.runs[run['id']]=run;tokens.add(run['token'])
        ledger=EvaluationBudget(self.path,self.manifest)
        try:
            existing={row[0] for row in ledger.db.execute('SELECT id FROM runs')}
            if set(self.runs)-existing:raise ValueError('run absent from existing frozen budget')
        finally:ledger.close()
        self.key=provider_key;self.authority=gateway_authority;self.factory=connection_factory
        self.lock=threading.Lock();self.active={};self.closed=False

    def _resolve(self,pairs):
        # Duplicate/auth syntax remains subject to raw-header validation. Resolve
        # only one exact credential here, before touching any run's budget.
        if type(pairs) not in (list,tuple) or not 1<=len(pairs)<=64:raise ValueError('bounded raw headers required')
        for pair in pairs:
            if type(pair) not in (list,tuple) or len(pair)!=2 or any(type(v) is not str or len(v)>4096 for v in pair):
                raise ValueError('bounded raw header pair required')
        values=[v for k,v in pairs if k.lower()=='x-api-key']
        if len(values)!=1 or type(values[0]) is not str or not values[0].isascii():
            raise ValueError('one run credential required')
        selected=None
        for run in self.runs.values():
            if hmac.compare_digest(values[0],run['token']):selected=run
        if selected is None:raise ValueError('unknown run credential')
        return selected

    def preflight(self,pairs,body_length):
        """Authenticate and validate framing before the HTTP server reads a body."""
        with self.lock:
            if self.closed:raise ValueError('gateway controller closed')
            run=self._resolve(pairs)
            validate_headers(pairs,body_length,run['token'],self.authority,run['allowed_beta_sets'])

    def handle(self,method,target,pairs,body,sink=None,on_response=None):
        """Process one SDK request. sink receives chunks after private capture.

        Return metadata only. Provider bytes are kept in a fresh private evidence
        file. Any upstream/consumer failure retains the full reserved bound.
        """
        if sink is not None and not callable(sink):raise ValueError('callable downstream sink required')
        if on_response is not None and not callable(on_response):raise ValueError('callable response observer required')
        with self.lock:
            if self.closed:raise ValueError('gateway controller closed')
            run=self._resolve(pairs)
            ledger=EvaluationBudget(self.path,self.manifest)
            request_id=uuid.uuid4().hex;cancel=threading.Event()
            try:
                receipt=admit_http_request(ledger,run['id'],request_id,method,target,pairs,body,
                    run['token'],self.authority,run['allowed_beta_sets'],run['contract'],run['model'])
                self.active[request_id]=(run['id'],cancel)
            except Exception:
                ledger.close();raise
        report=dict(status='incomplete',run_id=run['id'],request_id=request_id,admission=receipt)
        path=self.evidence/request_id
        try:
            path.mkdir(mode=0o700)
            raw_path=path/'response.bin'
            # Unbuffered capture precedes each optional downstream write; final
            # report hashes the bytes read back, not only an in-memory digest.
            with raw_path.open('xb',buffering=0) as raw:
                def capture(chunk):
                    pending=memoryview(chunk)
                    while pending:
                        written=raw.write(pending)
                        if not written:raise OSError('evidence write made no progress')
                        pending=pending[written:]
                    if sink is not None:sink(chunk)
                result=forward(ledger,run['id'],request_id,body,self.key,capture,
                    timeout_seconds=run['timeout_seconds'],cancel=cancel,connection_factory=self.factory,on_response=on_response)
            report['observation']=result
            report['response_file']=str(raw_path)
            report['response_file_sha256']=hashlib.sha256(raw_path.read_bytes()).hexdigest()
            report['status']=result['status']
        except Exception as error:
            report['error_type']=type(error).__name__
        finally:
            ledger.close()
            with self.lock:self.active.pop(request_id,None)
            if path.is_dir():
                with (path/'report.json').open('x') as stream:json.dump(report,stream,indent=2);stream.write('\n')
        return report

    def revoke(self,run_id):
        """Commit durable revocation before signalling every active run transport."""
        with self.lock:
            if run_id not in self.runs:raise ValueError('unknown configured run')
            ledger=EvaluationBudget(self.path,self.manifest)
            try:ledger.revoke(run_id)
            finally:ledger.close()
            for active_run,cancel in self.active.values():
                if active_run==run_id:cancel.set()

    def close(self):
        """Reject new requests and cancel active work; callers must join workers."""
        with self.lock:
            self.closed=True
            for _,cancel in self.active.values():cancel.set()

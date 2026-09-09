//go:build darwin || linux

package rpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sausheong/hand/protocol"
)

func ledgerPath(t *testing.T) string { return filepath.Join(t.TempDir(), "private", "requests.jsonl") }
func promptRequest() protocol.Request {
	return protocol.Request{Version: 1, ID: "request-1", Method: "prompt", Params: json.RawMessage(`{"prompt":"fix it"}`)}
}
func TestLedgerDuplicateAndRestartDoNotGrantExecution(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	req := promptRequest()
	if _, execute, err := l.Begin(req, "session"); err != nil || !execute {
		t.Fatalf("begin %t %v", execute, err)
	}
	if err = l.Bind(req.ID, "run-1"); err != nil {
		t.Fatal(err)
	}
	if r, execute, err := l.Begin(req, "session"); err != nil || execute || r.RunID != "run-1" {
		t.Fatalf("duplicate %+v %t %v", r, execute, err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if r, execute, err := l.Begin(req, "session"); err != nil || execute || r.State != "uncertain" || r.RunID != "run-1" {
		t.Fatalf("restart %+v %t %v", r, execute, err)
	}
	req.Params = json.RawMessage(`{"prompt":"different"}`)
	if _, _, err = l.Begin(req, "session"); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
}
func TestLedgerCompletedResultSurvivesRestartAndIsCopied(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	req := promptRequest()
	if _, _, err = l.Begin(req, "session"); err != nil {
		t.Fatal(err)
	}
	if err = l.Bind(req.ID, "run-1"); err != nil {
		t.Fatal(err)
	}
	result := ledgerTerminal()
	if err = l.Complete(req.ID, result); err != nil {
		t.Fatal(err)
	}
	result[2] = 'X'
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	r, execute, err := l.Begin(req, "session")
	if err != nil || execute || string(r.Result) != string(ledgerTerminal()) {
		t.Fatalf("replay %+v %t %v", r, execute, err)
	}
	r.Result[2] = 'X'
	r, _, _ = l.Begin(req, "session")
	if string(r.Result) != string(ledgerTerminal()) {
		t.Fatal("result mutation escaped")
	}
}
func TestLedgerExclusiveWriterAndTruncatedTail(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenLedger(path); err == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"id":"partial`), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenLedger(path); err == nil {
		other.Close()
		t.Fatal("truncated ledger accepted")
	}
}
func TestLedgerPendingIntentIsUncertainAfterRestart(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	req := promptRequest()
	req.Params = nil
	if _, execute, err := l.Begin(req, "session"); err != nil || !execute {
		t.Fatalf("default params %t %v", execute, err)
	}
	l.Close()
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	r, execute, err := l.Begin(req, "session")
	if err != nil || execute || r.State != "uncertain" {
		t.Fatalf("intent %+v %t %v", r, execute, err)
	}
}

func TestLedgerConcurrentDuplicateHasOneExecutionGrant(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var workers sync.WaitGroup
	var grants atomic.Int32
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, execute, err := l.Begin(promptRequest(), "session")
			if err != nil {
				t.Error(err)
			}
			if execute {
				grants.Add(1)
			}
		}()
	}
	workers.Wait()
	if grants.Load() != 1 {
		t.Fatalf("execution grants %d", grants.Load())
	}
	if err = l.Complete("request-1", json.RawMessage(`{}`)); err == nil {
		t.Fatal("completed before binding a run")
	}
}

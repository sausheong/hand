//go:build darwin || linux

package rpc

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sausheong/hand/protocol"
)

func TestLedgerRejectsCorruptTransitionsWithoutGrantingReplay(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RequestRecord)
		want   string
	}{
		{"unknown kind", func(r *RequestRecord) { r.Kind = "other" }, "invalid request identity"},
		{"control with run", func(r *RequestRecord) { r.Kind = "control" }, "invalid request identity"},
		{"prompt with operation", func(r *RequestRecord) { r.OperationID = "operation" }, "invalid request identity"},
		{"empty id", func(r *RequestRecord) { r.ID = "" }, "invalid request identity"},
		{"oversized id", func(r *RequestRecord) { r.ID = strings.Repeat("x", protocol.MaxRequestIDBytes+1) }, "invalid request identity"},
		{"empty session", func(r *RequestRecord) { r.SessionID = "" }, "invalid request identity"},
		{"short fingerprint", func(r *RequestRecord) { r.Fingerprint = "abc" }, "invalid request identity"},
		{"nonhex fingerprint", func(r *RequestRecord) { r.Fingerprint = strings.Repeat("z", 64) }, "invalid byte"},
		{"changed fingerprint", func(r *RequestRecord) { r.Fingerprint = strings.Repeat("0", 64) }, "request identity changed"},
		{"changed session", func(r *RequestRecord) { r.SessionID = "other" }, "request identity changed"},
		{"changed run", func(r *RequestRecord) { r.RunID = "other" }, "invalid request state transition"},
		{"reverted pending", func(r *RequestRecord) { r.State = "pending" }, "invalid request state transition"},
		{"repeated accepted", func(r *RequestRecord) { r.State = "accepted" }, "invalid request state transition"},
		{"unknown state", func(r *RequestRecord) { r.State = "finished" }, "invalid request state transition"},
		{"missing result", func(r *RequestRecord) { r.Result = nil }, "invalid request state transition"},
		{"unknown completed request", func(r *RequestRecord) { r.ID = "other" }, "invalid initial request record"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := ledgerPath(t)
			ledger, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			request := promptRequest()
			record, execute, err := ledger.Begin(request, "session")
			if err != nil || !execute {
				t.Fatalf("initial intent: %v %t", err, execute)
			}
			if err = ledger.Bind(request.ID, "run-1"); err != nil {
				t.Fatal(err)
			}
			if err = ledger.Close(); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			record.State = "completed"
			record.RunID = "run-1"
			record.Result = ledgerTerminal()
			tc.change(&record)
			bad, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := append(append(bytes.Clone(original), bad...), '\n')
			if err = os.WriteFile(path, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := OpenLedger(path)
			if opened != nil {
				opened.Close()
				t.Fatal("corrupt ledger granted a usable handle")
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong rejection: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, corrupt) {
				t.Fatal("opening corrupt ledger modified evidence", err)
			}
			// Restore only the valid prefix. A successful reopen proves the failed
			// attempt released its lock; the prior intent must still deny execution.
			if err = os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err = OpenLedger(path)
			if err != nil {
				t.Fatal("failed open leaked lock", err)
			}
			defer opened.Close()
			replayed, execute, err := opened.Begin(request, "session")
			if err != nil || execute || replayed.State != "uncertain" || replayed.RunID != "run-1" {
				t.Fatalf("unsafe replay %+v execute=%t err=%v", replayed, execute, err)
			}
		})
	}
}

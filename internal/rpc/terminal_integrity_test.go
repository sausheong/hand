//go:build darwin || linux

package rpc

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
)

func ledgerTerminal() json.RawMessage {
	raw, _ := json.Marshal(protocol.Event{Version: 1, EventID: "event-1", Sequence: 1, SessionID: "session", RunID: "run-1", RequestID: "request-1", Timestamp: time.Unix(1, 0).UTC(), Kind: "terminal", Payload: json.RawMessage(`{"status":"completed","reason":"completed","verified":false}`)})
	return raw
}

func TestLedgerTerminalMustMatchRunAndOutcomeBeforeWriteOrReplay(t *testing.T) {
	for name, change := range map[string]func(*protocol.Event){
		"request":          func(e *protocol.Event) { e.RequestID = "other" },
		"session":          func(e *protocol.Event) { e.SessionID = "other" },
		"run":              func(e *protocol.Event) { e.RunID = "other" },
		"version":          func(e *protocol.Event) { e.Version = 2 },
		"nonterminal":      func(e *protocol.Event) { e.Kind = "text" },
		"missing_event":    func(e *protocol.Event) { e.EventID = "" },
		"missing_sequence": func(e *protocol.Event) { e.Sequence = 0 },
		"missing_time":     func(e *protocol.Event) { e.Timestamp = time.Time{} },
		"null_verified": func(e *protocol.Event) {
			e.Payload = json.RawMessage(`{"status":"completed","reason":"done","verified":null}`)
		},
		"duplicate_status": func(e *protocol.Event) {
			e.Payload = json.RawMessage(`{"status":"cancelled","status":"completed","reason":"done","verified":false}`)
		},
		"case_alias_verified": func(e *protocol.Event) {
			e.Payload = json.RawMessage(`{"status":"completed","reason":"done","verified":false,"Verified":true}`)
		},
		"null_payload": func(e *protocol.Event) { e.Payload = json.RawMessage(`null`) },
		"unknown_status": func(e *protocol.Event) {
			e.Payload = json.RawMessage(`{"status":"success","reason":"done","verified":false}`)
		},
		"missing_reason": func(e *protocol.Event) { e.Payload = json.RawMessage(`{"status":"completed","verified":false}`) },
		"failed_verified": func(e *protocol.Event) {
			e.Payload = json.RawMessage(`{"status":"cancelled","reason":"cancelled","verified":true}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := ledgerPath(t)
			l, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			req := promptRequest()
			if _, _, err = l.Begin(req, "session"); err != nil {
				t.Fatal(err)
			}
			if err = l.Bind(req.ID, "run-1"); err != nil {
				t.Fatal(err)
			}
			record, err := l.Lookup(req.ID)
			if err != nil {
				t.Fatal(err)
			}
			prefix, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var event protocol.Event
			if err = json.Unmarshal(ledgerTerminal(), &event); err != nil {
				t.Fatal(err)
			}
			change(&event)
			bad, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if err = l.Complete(req.ID, bad); err == nil {
				t.Fatal("invalid terminal persisted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, prefix) {
				t.Fatal("rejected terminal altered ledger", err)
			}
			if err = l.Complete(req.ID, ledgerTerminal()); err != nil {
				t.Fatal(err)
			}
			if err = l.Close(); err != nil {
				t.Fatal(err)
			}
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			record.State = "completed"
			record.Result = bad
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := append(append(bytes.Clone(prefix), encoded...), '\n')
			if err = os.WriteFile(path, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := OpenLedger(path)
			if opened != nil {
				opened.Close()
			}
			if err == nil || opened != nil {
				t.Fatal("invalid terminal replay accepted")
			}
			after, err = os.ReadFile(path)
			if err != nil || !bytes.Equal(after, corrupt) {
				t.Fatal("failed reopen altered evidence", err)
			}
			if err = os.WriteFile(path, valid, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err = OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer opened.Close()
			replayed, execute, err := opened.Begin(req, "session")
			if err != nil || execute || !bytes.Equal(replayed.Result, ledgerTerminal()) {
				t.Fatal("valid terminal replay changed", err)
			}
		})
	}
}

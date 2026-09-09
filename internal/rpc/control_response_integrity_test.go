//go:build darwin || linux

package rpc

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestControlResponseAmbiguityRejectedBeforePersistence(t *testing.T) {
	for name, raw := range map[string]string{
		"duplicate_request":    `{"version":1,"request_id":"other","request_id":"control","result":{}}`,
		"escaped_request":      `{"version":1,"request_id":"other","\u0072equest_id":"control","result":{}}`,
		"duplicate_version":    `{"version":2,"version":1,"request_id":"control","result":{}}`,
		"case_alias":           `{"Version":1,"request_id":"control","result":{}}`,
		"unknown_field":        `{"version":1,"request_id":"control","result":{},"other":true}`,
		"duplicate_result":     `{"version":1,"request_id":"control","result":{"a":1},"result":{"a":2}}`,
		"empty_error":          `{"version":1,"request_id":"control","error":{}}`,
		"duplicate_error_code": `{"version":1,"request_id":"control","error":{"code":"denied","code":"allowed","message":"ambiguous"}}`,
		"alias_error_code":     `{"version":1,"request_id":"control","error":{"Code":"denied","message":"ambiguous"}}`,
		"null_error":           `{"version":1,"request_id":"control","result":{},"error":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := ledgerPath(t)
			l, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			req := rpcRequest("control", "session.new", `{}`)
			if _, _, err = l.BeginControl(req, "session"); err != nil {
				t.Fatal(err)
			}
			if err = l.Bind(req.ID, "operation:1"); err != nil {
				t.Fatal(err)
			}
			record, err := l.Lookup(req.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = l.Complete(req.ID, json.RawMessage(raw)); err == nil {
				t.Fatal("ambiguous response persisted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected response changed ledger", err)
			}
			if err = l.Complete(req.ID, json.RawMessage(`{"version":1,"request_id":"control","error":{"code":"denied","message":""}}`)); err != nil {
				t.Fatal("valid error response rejected", err)
			}
			if err = l.Close(); err != nil {
				t.Fatal(err)
			}
			valid, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			record.State = "completed"
			record.Result = json.RawMessage(raw)
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := append(append(bytes.Clone(before), encoded...), '\n')
			if err = os.WriteFile(path, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := OpenLedger(path)
			if opened != nil {
				opened.Close()
			}
			if err == nil || opened != nil {
				t.Fatal("ambiguous stored control response accepted")
			}
			after, err = os.ReadFile(path)
			if err != nil || !bytes.Equal(after, corrupt) {
				t.Fatal("failed reopen changed evidence", err)
			}
			if err = os.WriteFile(path, valid, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err = OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer opened.Close()
			replayed, execute, err := opened.BeginControl(req, "session")
			if err != nil || execute || replayed.State != "completed" {
				t.Fatal("restored control granted execution", err)
			}
		})
	}
}

func TestLedgerEnvelopeRejectsAmbiguityWithoutChangingEvidence(t *testing.T) {
	for _, field := range []string{"id", "session_id", "state", "fingerprint"} {
		for _, alias := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/duplicate", true: "/alias"}[alias], func(t *testing.T) {
				path := ledgerPath(t)
				l, err := OpenLedger(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = l.Begin(promptRequest(), "session"); err != nil {
					t.Fatal(err)
				}
				if err = l.Close(); err != nil {
					t.Fatal(err)
				}
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				key := `"` + field + `":`
				replacement := key + `"ignored",` + key
				if alias {
					replacement = `"` + strings.ToUpper(field) + `":`
				}
				corrupt := bytes.Replace(original, []byte(key), []byte(replacement), 1)
				if err = os.WriteFile(path, corrupt, 0600); err != nil {
					t.Fatal(err)
				}
				opened, err := OpenLedger(path)
				if opened != nil {
					opened.Close()
				}
				if err == nil || opened != nil {
					t.Fatal("ambiguous record accepted")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, corrupt) {
					t.Fatal("failed open modified evidence", err)
				}
				if err = os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				opened, err = OpenLedger(path)
				if err != nil {
					t.Fatal("failed open leaked lock", err)
				}
				defer opened.Close()
				if _, execute, err := opened.Begin(promptRequest(), "session"); err != nil || execute {
					t.Fatal("original intent replayed", err)
				}
			})
		}
	}
}

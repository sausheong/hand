//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"strings"
	"testing"
)

func TestControlQueueReplayPersistsAcrossReopen(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	service := app.New(&queuedBackend{}, app.Options{SessionID: "session"})
	d := NewDispatcher(service, l)
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	request := rpcRequest("enqueue", "followup.enqueue", `{"text":"one input"}`)
	first := d.Dispatch(context.Background(), request)
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	replay := d.Dispatch(context.Background(), request)
	if string(first.Result) != string(replay.Result) || len(service.QueuedInputs()) != 1 {
		t.Fatal("duplicate queue mutation")
	}
	if changed := d.Dispatch(context.Background(), rpcRequest("enqueue", "followup.enqueue", `{"text":"different"}`)); changed.Error == nil {
		t.Fatal("changed request accepted")
	}
	d.Close()
	l.Close()
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	service = app.New(&queuedBackend{}, app.Options{SessionID: "different-session"})
	d = NewDispatcher(service, l)
	defer d.Close()
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	replay = d.Dispatch(context.Background(), request)
	if replay.Error != nil || string(replay.Result) != string(first.Result) || len(service.QueuedInputs()) != 0 {
		t.Fatalf("replayed mutation after restart: %+v", replay)
	}
	record, err := l.Lookup("enqueue")
	if err != nil || record.Kind != "control" || record.OperationID == "" || record.RunID != "" {
		t.Fatalf("control identity %+v %v", record, err)
	}
}
func TestControlUncertainDoesNotExecute(t *testing.T) {
	path := ledgerPath(t)
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	request := rpcRequest("uncertain", "followup.enqueue", `{"text":"not again"}`)
	if _, _, err = l.BeginControl(request, "session"); err != nil {
		t.Fatal(err)
	}
	if err = l.Bind(request.ID, "operation:prior"); err != nil {
		t.Fatal(err)
	}
	l.Close()
	l, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	service := app.New(&queuedBackend{}, app.Options{SessionID: "session"})
	d := NewDispatcher(service, l)
	defer d.Close()
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	response := d.Dispatch(context.Background(), request)
	if response.Error == nil || response.Error.Code != "request_uncertain" || len(service.QueuedInputs()) != 0 {
		t.Fatalf("uncertain control reexecuted: %+v", response)
	}
}
func TestControlLargeQueueResponsePersists(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := app.New(&queuedBackend{}, app.Options{SessionID: "session"})
	d := NewDispatcher(s, l)
	defer d.Close()
	d.Dispatch(context.Background(), rpcRequest("h", "hello", `{}`))
	raw, _ := json.Marshal(map[string]string{"text": strings.Repeat("\t", 65535) + "x"})
	r := d.Dispatch(context.Background(), rpcRequest("large", "followup.enqueue", string(raw)))
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	record, err := l.Lookup("large")
	if err != nil || record.State != "completed" {
		t.Fatalf("large response not durable: %v", err)
	}
}

func TestControlLedgerRejectsInvalidResponseIdentity(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	request := rpcRequest("control", "session.new", `{}`)
	if _, _, err = l.BeginControl(request, "session"); err != nil {
		t.Fatal(err)
	}
	if err = l.Bind(request.ID, "operation:1"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":1,"request_id":"other","result":{}}`, `{"version":1,"request_id":"control","result":{},"error":{"code":"bad","message":"bad"}}`, `{"version":1,"request_id":"control"}`} {
		if err = l.Complete(request.ID, json.RawMessage(raw)); err == nil {
			t.Fatal("invalid response persisted")
		}
	}
	if err = l.Complete(request.ID, json.RawMessage(`{"version":1,"request_id":"control","result":{"session_id":"created"}}`)); err != nil {
		t.Fatal(err)
	}
}

//go:build darwin || linux

package rpc

import (
	"context"
	"testing"

	"github.com/sausheong/hand/internal/app"
)

func TestEmptyParameterMethodsRejectUnknownFieldsWithoutEffects(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	backend := &rpcBackend{hang: true, joined: make(chan struct{})}
	d := NewDispatcher(app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}), l)
	defer d.Close()
	ctx := context.Background()
	badHello := d.Dispatch(ctx, rpcRequest("bad-hello", "hello", `{"ignored":true}`))
	if badHello.Error == nil || badHello.Error.Code != "invalid_params" {
		t.Errorf("malformed hello accepted: %+v", badHello)
	}
	if r := d.Dispatch(ctx, rpcRequest("before-hello", "state", `{}`)); r.Error == nil || r.Error.Code != "not_negotiated" {
		t.Errorf("malformed hello negotiated: %+v", r)
	}
	if r := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := d.Dispatch(ctx, rpcRequest("p1", "prompt", `{"text":"keep running"}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	for _, method := range []string{"state", "approval.pending", "cancel"} {
		t.Run(method, func(t *testing.T) {
			r := d.Dispatch(ctx, rpcRequest("bad-"+method, method, `{"ignored":true}`))
			if r.Error == nil || r.Error.Code != "invalid_params" || len(r.Result) != 0 {
				t.Fatalf("unknown field accepted: %+v", r)
			}
		})
	}
	// Replaying a rejected durable control must preserve its original response;
	// changing its payload must not turn that request ID into a valid cancel.
	if r := d.Dispatch(ctx, rpcRequest("bad-cancel", "cancel", `{"ignored":true}`)); r.Error == nil || r.Error.Code != "invalid_params" {
		t.Fatalf("rejected cancel replay changed: %+v", r)
	}
	if r := d.Dispatch(ctx, rpcRequest("bad-cancel", "cancel", `{}`)); r.Error == nil || r.Error.Code != "request_conflict" {
		t.Fatalf("reused cancel ID accepted changed payload: %+v", r)
	}
	// A rejected cancel must leave the live backend owned by this dispatcher.
	select {
	case <-backend.joined:
		t.Fatal("malformed cancel stopped the backend")
	default:
	}
	if r := d.Dispatch(ctx, rpcRequest("stop", "cancel", `{}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	d.Close()
	select {
	case <-backend.joined:
	default:
		t.Fatal("valid cancellation did not join backend")
	}
	completedRequest(t, l)
}

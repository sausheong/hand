//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/local"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRPCRejectedControlsPreserveSessionAndDoNotExecute(t *testing.T) {
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	selected.Session.Append(session.UserMessageEntry("preserved work"))
	rt := &runtime.Runtime{AgentID: "hand", Session: selected.Session, Provider: "local", Model: "original"}
	defer func() { rt.Session.Close() }()
	backend := &rpcBackend{joined: make(chan struct{})}
	c := &app.Controller{Rt: rt, Sessions: manager, SessionKey: selected.Record.StoreKey, Owner: app.New(backend, app.Options{SessionID: selected.Session.ID, MaxIterations: 1})}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	if r := d.Dispatch(context.Background(), rpcRequest("hello", "hello", `{}`)); r.Error != nil {
		t.Fatal(r.Error)
	}
	before := d.Dispatch(context.Background(), rpcRequest("before", "session.list", `{}`))
	if before.Error != nil {
		t.Fatal(before.Error)
	}
	tests := []struct{ method, params string }{
		{"session.new", `{"unexpected":true}`},
		{"session.select", `{}`},
		{"session.select", `{"id":42}`},
		{"session.select", `{"id":"missing-session"}`},
		{"session.select", `{"id":"../outside"}`},
		{"session.name", `{"name":42}`},
		{"session.name", `{"name":"changed","unexpected":true}`},
		{"profile.select", `{}`},
		{"profile.select", `{"name":false}`},
		{"profile.select", `{"name":"missing-profile"}`},
		{"session.list", `{"offset":-1}`},
		{"session.list", `{"limit":-1}`},
		{"session.list", `{"limit":101}`},
		{"session.list", `{"offset":100}`},
		{"session.list", `{"limit":"all"}`},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%s/%d", tc.method, i), func(t *testing.T) {
			id := fmt.Sprintf("rejected-%d", i)
			r := d.Dispatch(context.Background(), rpcRequest(id, tc.method, tc.params))
			if r.Error == nil || r.Error.Code != "operation_rejected" || r.RequestID != id || len(r.Result) != 0 {
				t.Fatalf("unexpected rejection %+v", r)
			}
			after := d.Dispatch(context.Background(), rpcRequest("after", "session.list", `{}`))
			if after.Error != nil || string(after.Result) != string(before.Result) {
				t.Fatalf("rejected control changed session listing: %s -> %s", before.Result, after.Result)
			}
			if c.SessionID() != selected.Session.ID || rt.Model != "original" || len(rt.Session.History()) != 1 || backend.calls.Load() != 0 {
				t.Fatal("rejected control changed active state or executed backend")
			}
		})
	}
}

func TestRPCSessionAndProfileControls(t *testing.T) {
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	selected.Session.Append(session.UserMessageEntry("preserved work"))
	rt := &runtime.Runtime{AgentID: "hand", Session: selected.Session, Provider: "local", Model: "old"}
	defer func() { rt.Session.Close() }()
	controller := &app.Controller{Rt: rt, Sessions: manager, SessionKey: selected.Record.StoreKey, Owner: app.New(&rpcBackend{hang: true, joined: make(chan struct{})}, app.Options{SessionID: selected.Session.ID, MaxIterations: 1})}
	controller.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) {
		return local.NewLocalProvider("http://localhost:1"), nil
	}
	if err = controller.ConfigureProfiles(map[string]config.ModelProfile{"new": {Provider: "local", Model: "new-model"}}, ""); err != nil {
		t.Fatal(err)
	}
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(controller, ledger)
	defer d.Close()
	call := func(method, params string) json.RawMessage {
		t.Helper()
		r := d.Dispatch(context.Background(), rpcRequest(method, method, params))
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		return r.Result
	}
	call("hello", `{}`)
	call("session.new", `{}`)
	if controller.SessionID() == selected.Session.ID {
		t.Fatal("new session not selected")
	}
	call("session.name", `{"name":"RPC work"}`)
	raw := call("session.list", `{"limit":1}`)
	var page struct {
		Sessions []json.RawMessage `json:"sessions"`
		Total    int               `json:"total"`
		Next     int               `json:"next"`
	}
	if err = json.Unmarshal(raw, &page); err != nil || len(page.Sessions) != 1 || page.Total != 2 || page.Next != 1 {
		t.Fatalf("pagination %s %v", raw, err)
	}
	params, _ := json.Marshal(map[string]string{"id": selected.Session.ID})
	call("session.select", string(params))
	if controller.SessionID() != selected.Session.ID || len(rt.Session.History()) != 1 {
		t.Fatal("resume lost prior history")
	}
	call("profile.select", `{"name":"new"}`)
	if controller.CurrentProfile() != "new" || rt.Model != "new-model" {
		t.Fatal("profile not applied")
	}
	if r := d.Dispatch(context.Background(), rpcRequest("invalid", "session.list", `{"limit":101}`)); r.Error == nil {
		t.Fatal("unbounded page accepted")
	}
	// The RPC owner must settle a terminal result before a mutation can change
	// the session associated with the accepted request.
	call("prompt", `{"text":"keep running"}`)
	if r := d.Dispatch(context.Background(), rpcRequest("busy", "session.new", `{}`)); r.Error == nil {
		t.Fatal("active run allowed session mutation")
	}
	d.Close()
}

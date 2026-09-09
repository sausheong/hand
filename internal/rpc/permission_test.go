//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/permissions"
	"path/filepath"
	"strings"
	"testing"
)

func TestPermissionInspectionRevocationAndReplay(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	authority, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "authority"), workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	g := permissions.ScopedGrant{ID: "grant", Operation: permissions.FileWrite, Scope: permissions.TreeScope, Resource: workspace, Lifetime: permissions.PersistentGrant, Provenance: permissions.UserDecision, Workspace: workspace, ConfigDigest: digest}
	if err = authority.Grant(g); err != nil {
		t.Fatal(err)
	}
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	d := NewControllerDispatcher(&app.Controller{Authority: authority, Owner: app.New(&queuedBackend{}, app.Options{SessionID: "session"})}, l)
	defer d.Close()
	d.Dispatch(context.Background(), rpcRequest("hello", "hello", `{}`))
	response := d.Dispatch(context.Background(), rpcRequest("list", "permission.list", `{}`))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var listed struct {
		Grants []permissions.ScopedGrant
		Total  int
	}
	if err = json.Unmarshal(response.Result, &listed); err != nil || listed.Total != 1 || listed.Grants[0] != g {
		t.Fatalf("inspection mismatch %s %v", response.Result, err)
	}
	revoke := rpcRequest("revoke", "permission.revoke", `{"id":"grant"}`)
	first := d.Dispatch(context.Background(), revoke)
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	again := d.Dispatch(context.Background(), revoke)
	if again.Error != nil || string(first.Result) != string(again.Result) {
		t.Fatal("revocation replay changed")
	}
	if authority.Allowed(permissions.AuthorityContext{Workspace: workspace, ConfigDigest: digest}, permissions.AccessRequest{Operation: permissions.FileWrite, Resource: "file"}) {
		t.Fatal("revoked authority remained active")
	}
	response = d.Dispatch(context.Background(), rpcRequest("list2", "permission.list", `{}`))
	if err = json.Unmarshal(response.Result, &listed); err != nil || listed.Total != 0 {
		t.Fatal("revoked grant still listed")
	}
}

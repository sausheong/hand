//go:build darwin || linux

package rpc

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceLedgerIdentityAndIsolation(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	l, err := OpenWorkspaceLedger(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = l.Begin(rpcRequest("persist", "prompt", `{"text":"hello"}`), "session-a"); err != nil {
		t.Fatal(err)
	}
	if competing, err := OpenWorkspaceLedger(root, filepath.Join(workspace, ".")); err == nil {
		competing.Close()
		t.Fatal("second owner acquired same workspace")
	}
	l.Close()
	reopened, err := OpenWorkspaceLedger(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, err := reopened.Lookup("persist")
	if err != nil || r.State != "uncertain" || r.SessionID != "session-a" {
		t.Fatalf("lost original intent %+v %v", r, err)
	}
	other, err := OpenWorkspaceLedger(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err = other.Lookup("persist"); err == nil {
		t.Fatal("request crossed workspaces")
	}
}

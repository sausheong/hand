package app

import (
	"context"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestControllerNewUsesCatalogueAndPreservesPriorSession(t *testing.T) {
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	selected.Session.Append(session.UserMessageEntry("original work"))
	rt := &runtime.Runtime{AgentID: "hand", Session: selected.Session}
	controller := &Controller{Rt: rt, Sessions: manager, SessionKey: selected.Record.StoreKey}
	if err := controller.NewSession(); err != nil {
		t.Fatal(err)
	}
	defer rt.Session.Close()
	if rt.Session.ID == selected.Session.ID || len(rt.Session.Entries()) != 0 || controller.owner().options.SessionID != rt.Session.ID {
		t.Fatal("new session identity not committed")
	}
	snapshot, err := manager.Catalogue().Snapshot()
	if err != nil || len(snapshot.Sessions) != 2 || snapshot.LastActiveID != rt.Session.ID {
		t.Fatal("catalogue lost old session", err)
	}
	old, err := manager.Open(context.Background(), selected.Session.ID, false)
	if err != nil {
		t.Fatal("old writer lease not released", err)
	}
	defer old.Session.Close()
	if len(old.Session.History()) != 1 {
		t.Fatal("new discarded original history")
	}
}

func TestControllerResumeFailureKeepsCurrentLeaseAndSameSessionIsIdempotent(t *testing.T) {
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	rt := &runtime.Runtime{AgentID: "hand", Session: current.Session}
	controller := &Controller{Rt: rt, Sessions: manager, SessionKey: current.Record.StoreKey}
	defer rt.Session.Close()
	if err := controller.ResumeSession("missing"); err == nil {
		t.Fatal("unknown session accepted")
	}
	if rt.Session != current.Session {
		t.Fatal("failed resume changed backend")
	}
	if err := controller.ResumeSession(current.Session.ID); err != nil {
		t.Fatal("same-session resume tried to reacquire its own lease", err)
	}
	current.Session.Append(session.UserMessageEntry("still owned"))
	if err := current.Session.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := controller.RenameSession("retained name"); err != nil {
		t.Fatal(err)
	}
	records, err := controller.ListSessions()
	if err != nil || len(records) != 1 || records[0].Name != "retained name" {
		t.Fatal("name not listed", err)
	}
}

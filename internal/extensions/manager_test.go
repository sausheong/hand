//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTransactionalReloadPreservesAndReusesPeers(t *testing.T) {
	launched := []*Connection{}
	fail := false
	m, err := NewManager(context.Background(), func(operation, lifetime context.Context, s Specification) (*Connection, error) {
		if fail && s.Name == "unavailable" {
			return nil, errors.New("fixture launch failure")
		}
		c := fixtureConnection(t, "normal")
		launched = append(launched, c)
		context.AfterFunc(lifetime, func() { c.stop(context.Canceled) })
		return c, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	spec := Specification{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands"}}
	report, err := m.Reload(context.Background(), []Specification{spec})
	if err != nil || !report.Committed || len(launched) != 1 {
		t.Fatal(report, err)
	}
	report, err = m.Reload(context.Background(), []Specification{spec})
	if err != nil || len(report.Reused) != 1 || len(launched) != 1 {
		t.Fatal("unchanged peer restarted", report, err)
	}
	spec.Digest = strings.Repeat("b", 64)
	fail = true
	report, err = m.Reload(context.Background(), []Specification{spec, {Name: "unavailable", Digest: strings.Repeat("c", 64)}})
	if err == nil || report.Committed || len(launched) != 2 {
		t.Fatal("failed reload committed", report, err)
	}
	select {
	case <-launched[1].done:
	default:
		t.Fatal("staged peer not joined")
	}
	raw, err := m.Call(context.Background(), "fixture", "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal("previous working peer lost", err)
	}
	var reply struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(raw, &reply) != nil || reply.Count != 2 {
		t.Fatal("previous peer restarted", string(raw))
	}
	report, err = m.Reload(context.Background(), []Specification{spec})
	if err != nil || !report.Committed || len(launched) != 3 {
		t.Fatal(report, err)
	}
	select {
	case <-launched[0].done:
	default:
		t.Fatal("replaced peer not joined")
	}
	commands := m.Commands()
	if len(commands) != 1 || commands[0].Extension != "fixture" || commands[0].Name != "inspect" {
		t.Fatal(commands)
	}
	report, err = m.Reload(context.Background(), nil)
	if err != nil || len(report.Removed) != 1 || len(m.Commands()) != 0 {
		t.Fatal(report, err)
	}
	select {
	case <-launched[2].done:
	default:
		t.Fatal("removed peer not joined")
	}
}
func TestManagerRejectsReloadDuringActiveCallAndCloseJoins(t *testing.T) {
	var peer *Connection
	m, err := NewManager(context.Background(), func(operation, lifetime context.Context, s Specification) (*Connection, error) {
		peer = fixtureConnection(t, "normal")
		return peer, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Reload(context.Background(), []Specification{{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands"}}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := m.Call(context.Background(), "fixture", "hang", json.RawMessage(`{}`)); done <- err }()
	until := time.Now().Add(2 * time.Second)
	for {
		peer.mu.Lock()
		active := peer.pending != nil
		peer.mu.Unlock()
		if active {
			break
		}
		if time.Now().After(until) {
			t.Fatal("call not active")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = m.Reload(context.Background(), nil); !errors.Is(err, ErrBusy) {
		t.Fatal("reload interrupted active call", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("manager shutdown did not join")
	}
	if err := <-done; err == nil {
		t.Fatal("cancelled call succeeded")
	}
	if _, err = m.Reload(context.Background(), nil); err == nil {
		t.Fatal("closed manager reactivated")
	}
}

func TestReloadFactoryCannotRetireActivePeerOnRollback(t *testing.T) {
	var active *Connection
	m, err := NewManager(context.Background(), func(operation, lifetime context.Context, s Specification) (*Connection, error) {
		if active == nil {
			active = fixtureConnection(t, "normal")
		}
		return active, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	spec := Specification{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands"}}
	if _, err = m.Reload(context.Background(), []Specification{spec}); err != nil {
		t.Fatal(err)
	}
	spec.Digest = strings.Repeat("b", 64)
	if report, err := m.Reload(context.Background(), []Specification{spec}); err == nil || report.Committed {
		t.Fatal("reused active peer accepted", report, err)
	}
	if _, err = m.Call(context.Background(), "fixture", "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal("rollback closed active peer", err)
	}
}

func TestReloadFactoryCommitsOnlyWithPeers(t *testing.T) {
	ctx := context.Background()
	oldStarts, newStarts := 0, 0
	old := func(context.Context, context.Context, Specification) (*Connection, error) {
		oldStarts++
		return fixtureConnection(t, "normal"), nil
	}
	manager, err := NewManager(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	spec := Specification{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands"}}
	if _, err = manager.Reload(ctx, []Specification{spec}); err != nil {
		t.Fatal(err)
	}
	changed := spec
	changed.Digest = strings.Repeat("b", 64)
	failed := func(context.Context, context.Context, Specification) (*Connection, error) {
		return nil, errors.New("launch rejected")
	}
	report, err := manager.ReloadWithFactory(ctx, []Specification{changed}, failed)
	if err == nil || report.Committed {
		t.Fatal("failed factory committed")
	}
	if _, err = manager.Call(ctx, "fixture", "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal("old peer lost", err)
	}
	if _, err = manager.Reload(ctx, []Specification{changed}); err != nil || oldStarts != 2 {
		t.Fatal("old factory lost", err, oldStarts)
	}
	replacement := func(context.Context, context.Context, Specification) (*Connection, error) {
		newStarts++
		return fixtureConnection(t, "normal"), nil
	}
	if _, err = manager.ReloadWithFactory(ctx, []Specification{spec}, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Reload(ctx, []Specification{changed}); err != nil || newStarts != 2 {
		t.Fatal("new factory not committed", err, newStarts)
	}
}

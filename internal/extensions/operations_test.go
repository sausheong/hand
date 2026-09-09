//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func operationsFixture(t *testing.T, mode string, caps []string) *Manager {
	t.Helper()
	m, err := NewManager(context.Background(), func(operation, lifetime context.Context, s Specification) (*Connection, error) {
		return fixtureConnection(t, mode), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if _, err = m.Reload(context.Background(), []Specification{{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: caps, Mandatory: true}}); err != nil {
		t.Fatal(err)
	}
	return m
}
func TestManagerCommandsAndLifecycleValidateResults(t *testing.T) {
	for _, mode := range []string{"operations-normal", "operations-escape", "operations-outcome"} {
		t.Run(mode, func(t *testing.T) {
			m := operationsFixture(t, mode, []string{"commands", "lifecycle"})
			if _, err := m.ExecuteCommand(context.Background(), "fixture", "unregistered", ""); err == nil {
				t.Fatal("unregistered command dispatched")
			}
			out, err := m.ExecuteCommand(context.Background(), "fixture", "inspect", "args")
			if mode == "operations-normal" {
				if err != nil || len(out.Blocks) != 1 {
					t.Fatal(out, err)
				}
			} else if err == nil || len(out.Blocks) != 0 {
				t.Fatal("unsafe presentation accepted", out, err)
			}
			diagnostics, err := m.NotifyLifecycle(context.Background(), "run.finish", json.RawMessage(`{"run_id":"run-1"}`))
			if mode == "operations-outcome" {
				if err == nil || len(diagnostics) != 1 {
					t.Fatal("observer outcome override accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestManagerRoutesRequiredHooksAndRejectsCapabilityOmission(t *testing.T) {
	m := operationsFixture(t, "hook-deny", []string{"commands", "context.transform", "policy.check"})
	got, _, err := m.Transform(context.Background(), []ContextItem{{ID: "data", Kind: "retrieval", Text: "original"}})
	if err != nil || got[0].Text != "original/hook-deny" {
		t.Fatal(got, err)
	}
	if _, err = m.CheckPolicy(context.Background(), PolicyRequest{Action: "write", Resource: "source.go"}, true); !errors.Is(err, ErrPolicyDenied) {
		t.Fatal(err)
	}
	m2, err := NewManager(context.Background(), func(operation, lifetime context.Context, s Specification) (*Connection, error) {
		return fixtureConnection(t, "normal"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	report, err := m2.Reload(context.Background(), []Specification{{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands", "policy.check"}, Mandatory: true}})
	if err == nil || report.Committed {
		t.Fatal("peer silently omitted configured policy hook", report, err)
	}
}

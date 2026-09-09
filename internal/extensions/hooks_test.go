//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func hookFixture(t *testing.T, mode string) *Connection {
	t.Helper()
	c := fixtureConnection(t, mode)
	if _, err := c.Initialize(context.Background(), "fixture", []string{"commands", "context.transform", "policy.check"}); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestTransformsOrderedAndProtected(t *testing.T) {
	a, b := hookFixture(t, "hook-a"), hookFixture(t, "hook-b")
	entries := []Hook{{ID: "b", Order: 1, Mandatory: true, Connection: b}, {ID: "a", Order: 1, Mandatory: true, Connection: a}}
	h, err := NewHooks("context.transform", entries)
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Order = -10
	input := []ContextItem{{ID: "policy", Kind: "policy", Text: "never altered"}, {ID: "user", Kind: "user", Text: "user instruction"}, {ID: "data", Kind: "tool_result", Text: "original"}}
	got, diagnostics, err := h.Transform(context.Background(), input)
	if err != nil || len(diagnostics) != 0 || got[0] != input[0] || got[1] != input[1] || got[2].Text != "original/hook-a/hook-b" || input[2].Text != "original" {
		t.Fatal(got, diagnostics, err)
	}
}
func TestTransformFailureIsAtomicAndMandatoryFails(t *testing.T) {
	for _, mandatory := range []bool{false, true} {
		c := hookFixture(t, "hook-protected")
		h, err := NewHooks("context.transform", []Hook{{ID: "bad", Mandatory: mandatory, Connection: c}})
		if err != nil {
			t.Fatal(err)
		}
		input := []ContextItem{{ID: "policy", Kind: "policy", Text: "keep"}, {ID: "data", Kind: "retrieval", Text: "unchanged"}}
		got, diagnostics, err := h.Transform(context.Background(), input)
		if len(diagnostics) != 1 {
			t.Fatal("failure not disclosed")
		}
		if mandatory {
			if err == nil || got != nil {
				t.Fatal("mandatory transform failed open")
			}
		} else if err != nil || got[1].Text != "unchanged" {
			t.Fatal("partial transform committed", got, err)
		}
	}
}
func TestPolicyHooksCannotOverrideHostOrFailOpen(t *testing.T) {
	c := hookFixture(t, "hook-allow")
	h, err := NewHooks("policy.check", []Hook{{ID: "allow", Mandatory: true, Connection: c}})
	if err != nil {
		t.Fatal(err)
	}
	req := PolicyRequest{Action: "file.write", Resource: "source.go"}
	if _, err = h.Check(context.Background(), req, false); !errors.Is(err, ErrPolicyDenied) {
		t.Fatal("host denial overridden", err)
	}
	raw, err := c.Call(context.Background(), "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var count struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(raw, &count) != nil || count.Count != 2 {
		t.Fatal("host-denied action reached peer", string(raw))
	}
	for _, mode := range []string{"hook-deny", "hook-missing", "hook-crash"} {
		c := hookFixture(t, mode)
		h, err := NewHooks("policy.check", []Hook{{ID: "required", Mandatory: true, Connection: c}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.Check(context.Background(), req, true); !errors.Is(err, ErrPolicyDenied) {
			t.Fatal("mandatory hook failed open", mode, err)
		}
	}
	c = hookFixture(t, "hook-crash")
	h, err = NewHooks("policy.check", []Hook{{ID: "optional", Connection: c}})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := h.Check(context.Background(), req, true)
	if err != nil || len(diagnostics) != 1 {
		t.Fatal("optional failure not disclosed", diagnostics, err)
	}
}

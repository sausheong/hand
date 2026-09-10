package app

import (
	"context"
	"errors"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestContextPinUpdatesPreserveOtherPins(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{Session: session.NewSession("hand", "pins")}}
	ctx := context.Background()
	for _, pin := range []runtime.ContextPin{{ID: "one", Kind: "objective", Text: "first"}, {ID: "two", Kind: "constraint", Text: "second"}, {ID: "one", Kind: "objective", Text: "updated"}} {
		if err := c.SetContextPin(ctx, pin); err != nil {
			t.Fatal(err)
		}
	}
	pins, err := c.ContextPins(ctx)
	if err != nil || len(pins) != 2 || pins[0].Text != "updated" || pins[1].Text != "second" {
		t.Fatalf("pins %+v %v", pins, err)
	}
	pins[0].Text = "caller change"
	again, _ := c.ContextPins(ctx)
	if again[0].Text != "updated" {
		t.Fatal("returned state leaked")
	}
	if err = c.SetContextPin(ctx, runtime.ContextPin{ID: "one", Kind: "invalid", Text: "bad"}); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	if err = c.RemoveContextPin(ctx, "missing"); err == nil {
		t.Fatal("missing pin removal accepted")
	}
	if err = c.RemoveContextPin(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	pins, _ = c.ContextPins(ctx)
	if len(pins) != 1 || pins[0].ID != "two" {
		t.Fatal("removed wrong pin")
	}
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatal(err)
	}
	err = c.RemoveContextPin(ctx, "two")
	release()
	if !errors.Is(err, ErrBusy) {
		t.Fatal("busy mutation accepted")
	}
}

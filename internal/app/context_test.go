package app

import (
	"context"
	"errors"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

func TestContextInspectionOwnershipAndSnapshot(t *testing.T) {
	s := session.NewSession("hand", "context")
	s.Append(session.UserMessageEntry("hello"))
	c := &Controller{Rt: &runtime.Runtime{StaticSystemPrompt: "12345678", Session: s}}
	report, err := c.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Contributions[0].EstimatedTokens != 2 || report.Contributions[1].Count != 1 {
		t.Fatalf("report %+v", report)
	}
	report.Contributions[0].Name = "mutated returned view"
	again, err := c.InspectContext(context.Background())
	if err != nil || again.Contributions[0].Name == report.Contributions[0].Name {
		t.Fatal("inspection shares mutable state")
	}
	_, release, err := c.owner().reserve(context.Background(), Idle)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.InspectContext(context.Background())
	release()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("busy inspection: %v", err)
	}
}

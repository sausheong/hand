package app

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/verification"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
)

func TestVerificationContextLimitsPreserveExistingFacts(t *testing.T) {
	ctx := context.Background()
	r := &runtime.Runtime{Session: session.NewSession("hand", "refs")}
	c := &Controller{Rt: r}
	items := []runtime.ContextStateItem{}
	for i := 0; i < 32; i++ {
		items = append(items, runtime.ContextStateItem{ID: fmt.Sprintf("fact-%d", i), Kind: "decision", Text: "preserve"})
	}
	if err := r.SetContextState(ctx, 0, items); err != nil {
		t.Fatal(err)
	}
	result := VerificationResult{ID: "saved-record", Record: verification.View{Profile: "test", Before: "snapshot"}, Assessment: verification.Assessment{Status: "passed"}}
	if err := c.retainVerificationContext(ctx, result); err == nil || !strings.Contains(err.Error(), "saved-record") {
		t.Fatal("capacity failure not disclosed", err)
	}
	got, err := r.ContextState(ctx)
	if err != nil || got.Revision != 1 || len(got.Items) != 32 {
		t.Fatal("capacity failure changed facts", got, err)
	}
	if err = r.SetContextState(ctx, 1, nil); err != nil {
		t.Fatal(err)
	}
	if err = c.retainVerificationContext(ctx, result); err != nil {
		t.Fatal(err)
	}
	got, err = r.ContextState(ctx)
	if err != nil || len(got.Items) != 1 {
		t.Fatal(got, err)
	}
	conflicting := []runtime.ContextStateItem{{ID: got.Items[0].ID, Kind: "decision", Text: "user-authored fact"}}
	if err = r.SetContextState(ctx, got.Revision, conflicting); err != nil {
		t.Fatal(err)
	}
	if err = c.retainVerificationContext(ctx, result); err == nil {
		t.Fatal("automatic capture overwrote user fact")
	}
	got, err = r.ContextState(ctx)
	if err != nil || got.Items[0].Text != "user-authored fact" {
		t.Fatal(got, err)
	}
}

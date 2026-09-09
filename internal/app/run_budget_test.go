package app

import (
	"context"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestSelectedRunBudgetPersistsAcrossContinuationAndRestart(t *testing.T) {
	ctx := context.Background()
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "budget"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "budget")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sess != nil {
			sess.Close()
		}
	}()
	makeService := func() (*Controller, *Service) {
		r := &runtime.Runtime{Session: sess, LLM: &persistedUsageProvider{}, Tools: tool.NewRegistry(), Model: "test", MaxTurns: 1}
		s := New(&HarnessBackend{Runtime: r}, Options{SessionID: sess.ID, MaxIterations: 2, Check: func(_ context.Context, _ string, iteration int) agentio.GoalLoopOutcome {
			return agentio.GoalLoopOutcome{Continue: iteration == 1, NextPrompt: "continue", Verified: iteration == 2}
		}})
		return &Controller{Rt: r, Owner: s}, s
	}
	c, s := makeService()
	if err = c.DecideRunTokenBudget(ctx, "logical-job", 1); err != nil {
		t.Fatal(err)
	}
	if err = c.SelectRunBudget(ctx, "logical-job"); err != nil {
		t.Fatal(err)
	}
	result, err := s.Execute(ctx, "blocked", nil, nil)
	if err != nil || result.Status != agentio.BudgetExhausted || result.Reason != "run_token_budget" || result.Verified {
		t.Fatalf("blocked %+v %v", result, err)
	}
	if err = c.DecideRunTokenBudget(ctx, "logical-job", 100000); err != nil {
		t.Fatal(err)
	}
	result, err = s.Execute(ctx, "resumed", nil, nil)
	if err != nil || result.Status != agentio.Completed || result.Iterations != 2 {
		t.Fatalf("resumed %+v %v", result, err)
	}
	state, err := c.RunBudget(ctx, "logical-job")
	if err != nil || state.Tokens.Committed != 90 {
		t.Fatalf("continuation charges %+v %v", state, err)
	}
	if err = sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess = nil
	sess, err = store.LoadExclusive("hand", "budget")
	if err != nil {
		t.Fatal(err)
	}
	c, s = makeService()
	result, err = s.Execute(ctx, "after restart", nil, nil)
	if err != nil || result.Status != agentio.Completed {
		t.Fatalf("restart %+v %v", result, err)
	}
	state, err = c.RunBudget(ctx, "logical-job")
	if err != nil || state.Tokens.Committed != 180 {
		t.Fatalf("restart reset charges: %+v %v", state, err)
	}
	runs, err := ReadSessionRuns(sess)
	if err != nil || len(runs) != 3 {
		t.Fatalf("runs %+v %v", runs, err)
	}
	for _, run := range runs {
		if run.Start.BudgetID != "logical-job" || run.Finish == nil || run.Finish.BudgetID != "logical-job" {
			t.Fatalf("budget association lost %+v", run)
		}
	}
}
func TestRunDeadlineCoversCompletionValidator(t *testing.T) {
	ctx := context.Background()
	sess := session.NewSession("hand", "validator-time")
	r := &runtime.Runtime{Session: sess, LLM: &persistedUsageProvider{}, Tools: tool.NewRegistry(), Model: "test", MaxTurns: 1}
	checked := false
	s := New(&HarnessBackend{Runtime: r}, Options{SessionID: sess.ID, MaxIterations: 1, Check: func(ctx context.Context, _ string, _ int) agentio.GoalLoopOutcome {
		checked = true
		<-ctx.Done()
		return agentio.GoalLoopOutcome{Verified: true}
	}})
	c := &Controller{Rt: r, Owner: s}
	if err := c.DecideRunTimeBudget(ctx, "timed", 1); err != nil {
		t.Fatal(err)
	}
	if err := c.SelectRunBudget(ctx, "timed"); err != nil {
		t.Fatal(err)
	}
	terminals := 0
	result, err := s.Execute(ctx, "verify", nil, func(e Event) {
		if e.Kind == "terminal" {
			terminals++
		}
	})
	if err != nil || !checked || terminals != 1 || result.Status != agentio.BudgetExhausted || result.Reason != "run_time_budget" || result.Verified {
		t.Fatalf("expiry %+v err=%v checked=%v terminals=%d", result, err, checked, terminals)
	}
	result, err = s.Execute(ctx, "cannot renew", nil, nil)
	if err != nil || result.Reason != "run_time_budget" || result.Iterations != 0 {
		t.Fatalf("expired restart %+v %v", result, err)
	}
}

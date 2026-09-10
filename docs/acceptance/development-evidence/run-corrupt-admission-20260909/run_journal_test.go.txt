package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestRunJournalPersistsFinalOutcomesBeforeTerminal(t *testing.T) {
	for _, status := range []agentio.RunStatus{agentio.Completed, agentio.VerificationFailed, agentio.BudgetExhausted, agentio.Cancelled, agentio.InfrastructureError} {
		t.Run(string(status), func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess, LLM: &persistedUsageProvider{fail: status == agentio.InfrastructureError}, Tools: tool.NewRegistry(), Model: "model", MaxTurns: 1}}
			options := Options{SessionID: sess.ID, MaxIterations: 1, Model: ModelInfo{Profile: "coding", RequestedModel: "provider/model"}}
			options.Check = func(context.Context, string, int) agentio.GoalLoopOutcome {
				if status == agentio.VerificationFailed {
					return agentio.GoalLoopOutcome{Err: errors.New("mandatory check failed")}
				}
				return agentio.GoalLoopOutcome{Verified: true}
			}
			if status == agentio.BudgetExhausted {
				options.MaxIterations = 0
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if status == agentio.Cancelled {
				cancel()
			}
			service := New(backend, options)
			terminalSeen := false
			result, err := service.Execute(ctx, "work", nil, func(event Event) {
				if event.Kind != "terminal" {
					return
				}
				terminalSeen = true
				runs, err := ReadSessionRuns(sess)
				if err != nil || len(runs) != 1 || runs[0].Finish == nil || runs[0].Finish.Outcome.Status != event.Status {
					t.Fatalf("terminal before durable outcome: %+v %v", runs, err)
				}
			})
			if err != nil || result.Status != status || !terminalSeen {
				t.Fatal(result, err)
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			sess, err = store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			runs, err := ReadSessionRuns(sess)
			if err != nil || len(runs) != 1 || runs[0].Finish == nil {
				t.Fatal(runs, err)
			}
			saved := runs[0].Finish
			if saved.Outcome.Status != status || saved.Model != options.Model || saved.Model.ServingModelKnown || saved.Outcome.Verified != (status == agentio.Completed) {
				t.Fatal(saved)
			}
		})
	}
}

func TestRunJournalRetainsInterruptedStartAndModelHistory(t *testing.T) {
	store := session.NewStore(t.TempDir())
	if err := store.Create("hand", "key"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
	for i, model := range []string{"provider/first", "provider/second"} {
		record := RunRecord{Version: 1, ID: model, RunID: uint64(i + 1), Phase: "start", At: time.Now().UTC(), Model: ModelInfo{RequestedModel: model}}
		if err := backend.RecordRun(record); err != nil {
			t.Fatal(err)
		}
	}
	if sess.LeafID() != "" {
		t.Fatal("journal moved conversation leaf")
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	sess, err = store.LoadExclusive("hand", "key")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	runs, err := ReadSessionRuns(sess)
	if err != nil || len(runs) != 2 || runs[0].Finish != nil || runs[1].Finish != nil || runs[0].Start.Model.RequestedModel == runs[1].Start.Model.RequestedModel {
		t.Fatal(runs, err)
	}
}

type journalFailureBackend struct {
	backendFixture
	journal    *HarnessBackend
	calls      int
	failFinish bool
}

func (b *journalFailureBackend) RecordRun(r RunRecord) error {
	if r.Phase == "finish" && b.failFinish {
		b.journal.Runtime.Session.Close()
	}
	return b.journal.RecordRun(r)
}
func (b *journalFailureBackend) Run(ctx context.Context, prompt string, images []llm.ImageContent) (<-chan BackendEvent, error) {
	b.calls++
	return b.backendFixture.Run(ctx, prompt, images)
}

func TestRunJournalWriteFailurePreventsSuccessfulCompletion(t *testing.T) {
	for _, phase := range []string{"start", "finish"} {
		t.Run(phase, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			backend := &journalFailureBackend{backendFixture: backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}, journal: &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}, failFinish: phase == "finish"}
			if phase == "start" {
				sess.Close()
			}
			service := New(backend, Options{MaxIterations: 1, SessionID: sess.ID})
			result, err := service.Execute(context.Background(), "work", nil, nil)
			if err != nil || result.Status != agentio.InfrastructureError || result.Reason != "run_metadata_"+phase+"_failed" {
				t.Fatal(result, err)
			}
			if phase == "start" && backend.calls != 0 {
				t.Fatal("provider ran without durable start")
			}
		})
	}
}

func TestRunJournalRejectsFutureSchema(t *testing.T) {
	sess := session.NewSession("hand", "key")
	if err := sess.Annotate(runJournalKind, json.RawMessage(`{"version":2}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSessionRuns(sess); err == nil {
		t.Fatal("future journal accepted")
	}
	backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
	record := RunRecord{Version: 1, ID: "new", RunID: 1, Phase: "start", At: time.Now().UTC()}
	if err := backend.RecordRun(record); err == nil {
		t.Fatal("new run appended to unsupported journal")
	}
	if len(sess.Annotations(runJournalKind)) != 1 {
		t.Fatal("invalid journal mutated")
	}
}

func TestRunJournalRejectsOrphanAndDuplicateTransitions(t *testing.T) {
	sess := session.NewSession("hand", "key")
	backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
	start := RunRecord{Version: 1, ID: "run", RunID: 1, Phase: "start", At: time.Now().UTC(), Model: ModelInfo{RequestedModel: "provider/model"}}
	finish := start
	finish.Phase = "finish"
	finish.Outcome = &agentio.RunOutcome{Status: agentio.Completed, Reason: "answer_completed", Iterations: 1}
	if err := backend.RecordRun(finish); err == nil {
		t.Fatal("orphan finish accepted")
	}
	if len(sess.Annotations(runJournalKind)) != 0 {
		t.Fatal("orphan mutated session")
	}
	if err := backend.RecordRun(start); err != nil {
		t.Fatal(err)
	}
	if err := backend.RecordRun(start); err == nil {
		t.Fatal("duplicate start accepted")
	}
	wrongModel := finish
	wrongModel.Model.RequestedModel = "provider/different"
	if err := backend.RecordRun(wrongModel); err == nil {
		t.Fatal("changed model accepted within one run")
	}
	if err := backend.RecordRun(finish); err != nil {
		t.Fatal(err)
	}
	if err := backend.RecordRun(finish); err == nil {
		t.Fatal("duplicate outcome accepted")
	}
	if len(sess.Annotations(runJournalKind)) != 2 {
		t.Fatal("rejected transitions changed journal")
	}
}

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRunJournalAmbiguitySurvivesReopenWithoutMutation(t *testing.T) {
	for _, field := range []string{"status", "verified"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			store := session.NewStore(dir)
			if err := store.Create("hand", "key"); err != nil {
				t.Fatal(err)
			}
			sess, err := store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if sess != nil {
					sess.Close()
				}
			}()
			backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
			start := RunRecord{Version: 1, ID: "run", RunID: 1, Phase: "start", At: time.Now().UTC()}
			if err := backend.RecordRun(start); err != nil {
				t.Fatal(err)
			}
			finish := start
			finish.Phase = "finish"
			finish.Outcome = &agentio.RunOutcome{Status: agentio.Completed, Reason: "answer_completed", Iterations: 1, Verified: true}
			raw, err := json.Marshal(finish)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := strings.Replace(string(raw), `"status":"completed"`, `"status":"cancelled","status":"completed"`, 1)
			if field == "verified" {
				corrupt = strings.Replace(string(raw), `"verified":true`, `"verified":false,"verified":true`, 1)
			}
			if err := sess.Annotate(runJournalKind, json.RawMessage(corrupt)); err != nil {
				t.Fatal(err)
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "hand", "key.jsonl")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(before, []byte(corrupt)) {
				t.Fatal("fixture did not preserve ambiguous payload on disk")
			}
			sess, err = store.LoadExclusive("hand", "key")
			if err != nil {
				t.Fatal(err)
			}
			controller := &Controller{Rt: &runtime.Runtime{Session: sess}}
			for i := 0; i < 2; i++ {
				if runs, err := controller.SessionRuns(); err == nil || runs != nil {
					t.Fatal("reopened corrupt history accepted", runs, err)
				}
				next := start
				next.ID = "new"
				next.RunID = 2
				backend.Runtime.Session = sess
				if err := backend.RecordRun(next); err == nil {
					t.Fatal("reopened corruption permitted append")
				}
			}

			probe := &journalFailureBackend{journal: backend, backendFixture: backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}}
			service := New(probe, Options{SessionID: sess.ID, MaxIterations: 1})
			for attempt := 0; attempt < 2; attempt++ {
				var events []Event
				result, err := service.Execute(context.Background(), "must not reach provider", []llm.ImageContent(nil), func(event Event) { events = append(events, event) })
				if err != nil || result.Status != agentio.InfrastructureError || result.Reason != "run_metadata_start_failed" || result.Verified || result.Iterations != 0 {
					t.Fatal("corruption did not fail admission", result, err)
				}
				terminals := 0
				for _, event := range events {
					if event.Kind == "terminal" {
						terminals++
						if event.Status != agentio.InfrastructureError || event.Verified || event.Error == "" {
							t.Fatal("invalid terminal failure", event)
						}
					}
					if event.Kind == "text" || event.Kind == "tool_call" {
						t.Fatal("work emitted after rejected admission", event)
					}
				}
				if terminals != 1 || probe.calls != 0 || service.Snapshot().State != Idle {
					t.Fatal("failed admission ran work, duplicated completion or retained ownership", terminals, probe.calls, service.Snapshot())
				}
			}
			if err := sess.Close(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("read or rejected append changed stored bytes")
			}
		})
	}
}

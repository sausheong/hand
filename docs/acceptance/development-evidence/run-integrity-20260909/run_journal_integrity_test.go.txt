package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestRunJournalRejectsAmbiguousRecordsWithoutAppending(t *testing.T) {
	start := RunRecord{Version: 1, ID: "one", RunID: 1, Phase: "start", At: time.Now().UTC()}
	finish := start
	finish.Phase = "finish"
	finish.Outcome = &agentio.RunOutcome{Status: agentio.Completed, Reason: "answer_completed", Iterations: 1}
	raw, err := json.Marshal(finish)
	if err != nil {
		t.Fatal(err)
	}
	for name, corrupt := range map[string]string{
		"duplicate outcome":    strings.Replace(string(raw), `"status":"completed"`, `"status":"cancelled","status":"completed"`, 1),
		"escaped duplicate":    strings.Replace(string(raw), `"status":"completed"`, `"st\u0061tus":"cancelled","status":"completed"`, 1),
		"case alias":           strings.Replace(string(raw), `"status":"completed"`, `"STATUS":"completed"`, 1),
		"unknown field":        strings.Replace(string(raw), `"status":"completed"`, `"status":"completed","future":true`, 1),
		"missing verification": strings.Replace(string(raw), `,"verified":false`, "", 1),
		"null verification":    strings.Replace(string(raw), `"verified":false`, `"verified":null`, 1),
		"duplicate identity":   strings.Replace(string(raw), `"id":"one"`, `"id":"other","id":"one"`, 1),
		"model alias":          strings.Replace(string(raw), `"RequestedModel"`, `"requestedmodel"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			sess := session.NewSession("hand", "key")
			defer sess.Close()
			backend := &HarnessBackend{Runtime: &runtime.Runtime{Session: sess}}
			if err := backend.RecordRun(start); err != nil {
				t.Fatal(err)
			}
			if err := sess.Annotate(runJournalKind, json.RawMessage(corrupt)); err != nil {
				t.Fatal(err)
			}
			before := sess.Annotations(runJournalKind)
			if runs, err := ReadSessionRuns(sess); err == nil || runs != nil {
				t.Fatal("ambiguous history accepted", runs, err)
			}
			next := start
			next.ID = "next"
			next.RunID = 2
			if err := backend.RecordRun(next); err == nil {
				t.Fatal("appended to corrupt history")
			}
			if !reflect.DeepEqual(before, sess.Annotations(runJournalKind)) {
				t.Fatal("rejected operation changed history")
			}
		})
	}
}

func TestSessionRunsUnavailable(t *testing.T) {
	for _, c := range []*Controller{{}, {Rt: &runtime.Runtime{}}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("unavailable history panicked: %v", r)
				}
			}()
			if _, err := c.SessionRuns(); err == nil {
				t.Error("unavailable history claimed success")
			}
		}()
	}
}

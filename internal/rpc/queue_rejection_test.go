//go:build darwin || linux

package rpc

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/sausheong/hand/internal/app"
)

func TestRejectedQueueControlsPreservePendingWork(t *testing.T) {
	cases := []struct{ name, method, params, code string }{
		{"steering_auto", "steer", `{"text":"unexpected","auto":true}`, "invalid_params"},
		{"steering_alias", "steer", `{"Text":"unexpected"}`, "invalid_params"},
		{"empty_followup", "followup.enqueue", `{"text":""}`, "queue_rejected"},
		{"duplicate_auto", "followup.enqueue", `{"text":"unexpected","auto":false,"auto":true}`, "invalid_params"},
		{"negative_offset", "queue.list", `{"offset":-1}`, "invalid_params"},
		{"past_end", "queue.list", `{"offset":3}`, "invalid_params"},
		{"invalid_offset", "queue.list", `{"offset":"0"}`, "invalid_params"},
		{"edit_unknown", "queue.edit", `{"id":"missing","text":"changed"}`, "queue_rejected"},
		{"edit_empty", "queue.edit", `{"id":"%s","text":""}`, "queue_rejected"},
		{"edit_duplicate", "queue.edit", `{"id":"%s","text":"old","text":"changed"}`, "invalid_params"},
		{"remove_unknown", "queue.remove", `{"id":"missing"}`, "queue_rejected"},
		{"remove_alias", "queue.remove", `{"ID":"%s"}`, "invalid_params"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger, err := OpenLedger(ledgerPath(t))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			backend := &queuedBackend{prompts: make(chan string, 4)}
			service := app.New(backend, app.Options{SessionID: "queue-rejection", MaxIterations: 1})
			d := NewDispatcher(service, ledger)
			defer d.Close()
			ctx := context.Background()
			if r := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); r.Error != nil {
				t.Fatal(r.Error)
			}
			first, err := service.EnqueueInput(app.SteeringQueue, "keep steering")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.EnqueueInput(app.FollowupQueue, "keep followup"); err != nil {
				t.Fatal(err)
			}
			before := service.QueuedInputs()
			params := tc.params
			if tc.name == "edit_empty" || tc.name == "edit_duplicate" || tc.name == "remove_alias" {
				params = fmt.Sprintf(params, first.ID)
			}
			for retry := 0; retry < 2; retry++ {
				r := d.Dispatch(ctx, rpcRequest("rejected", tc.method, params))
				if r.Error == nil || r.Error.Code != tc.code || len(r.Result) != 0 {
					t.Fatalf("wrong rejection: %+v", r)
				}
				if !reflect.DeepEqual(before, service.QueuedInputs()) {
					t.Fatal("rejected request changed pending queue")
				}
				if backend.calls.Load() != 0 || len(d.automatic) != 0 {
					t.Fatal("rejected request triggered automatic work")
				}
			}
			// Rejection must not poison admission of an explicit valid correction.
			params = fmt.Sprintf(`{"id":%q,"text":"corrected steering"}`, first.ID)
			if r := d.Dispatch(ctx, rpcRequest("correction", "queue.edit", params)); r.Error != nil {
				t.Fatal(r.Error)
			}
			after := service.QueuedInputs()
			if len(after) != 2 || after[0].ID != first.ID || after[0].Text != "corrected steering" || after[1] != before[1] {
				t.Fatal("valid correction disturbed unrelated pending work")
			}
		})
	}
}

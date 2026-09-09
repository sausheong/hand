package rpc

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"testing"
)

type summaryRPCProvider struct{ llm.LLMProvider }

func TestSummarizerRPCReviewConfirmationAndReplay(t *testing.T) {
	ctx := context.Background()
	c := &app.Controller{Owner: app.New(nil, app.Options{SessionID: "summary"}), Rt: &runtime.Runtime{Session: session.NewSession("hand", "summary"), Compaction: &compaction.Manager{}}}
	if err := c.ConfigureProfiles(map[string]config.ModelProfile{"summary": {Provider: "local", Model: "summary-model", Endpoint: "http://localhost:9000/v1"}}, ""); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { calls++; return &summaryRPCProvider{}, nil }
	ledger, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	d := NewControllerDispatcher(c, ledger)
	defer d.Close()
	d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
	response := d.Dispatch(ctx, rpcRequest("review", "summarizer.review", `{"profile":"summary","max_output_tokens":1024,"timeout_seconds":30}`))
	var review app.SummarizerView
	if response.Error != nil || json.Unmarshal(response.Result, &review) != nil || review.Destination != "http://localhost:9000/v1" || calls != 0 {
		t.Fatalf("review %+v", response)
	}
	payload := map[string]any{"options": review.Options, "digest": review.Digest, "confirmed": false}
	raw, _ := json.Marshal(payload)
	refused := d.Dispatch(ctx, rpcRequest("unconfirmed", "summarizer.select", string(raw)))
	if refused.Error == nil || calls != 0 {
		t.Fatal("unconfirmed selection executed")
	}
	payload["confirmed"] = true
	raw, _ = json.Marshal(payload)
	request := rpcRequest("select", "summarizer.select", string(raw))
	first := d.Dispatch(ctx, request)
	if first.Error != nil || calls != 1 {
		t.Fatalf("selection %+v calls %d", first, calls)
	}
	again := d.Dispatch(ctx, request)
	if again.Error != nil || string(again.Result) != string(first.Result) || calls != 1 {
		t.Fatal("replay rebuilt provider")
	}
	status := d.Dispatch(ctx, rpcRequest("status", "summarizer.status", `{}`))
	var value struct {
		Independent bool               `json:"independent"`
		Selection   app.SummarizerView `json:"selection"`
	}
	if status.Error != nil || json.Unmarshal(status.Result, &value) != nil || !value.Independent || value.Selection.Digest != review.Digest {
		t.Fatalf("status %+v", status)
	}
}

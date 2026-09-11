package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type steeringProvider struct {
	llmtest.Base
	calls     int
	corrected bool
}

func (p *steeringProvider) ChatStream(_ context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	ch := make(chan llm.ChatEvent, 3)
	if p.calls == 1 {
		for _, id := range []string{"one", "two"} {
			ch <- llm.ChatEvent{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: id, Name: "read", Input: json.RawMessage(`{}`)}}
		}
	} else {
		raw, _ := json.Marshal(req.Messages)
		p.corrected = strings.Contains(string(raw), "answer instead")
	}
	if p.calls > 1 {
		ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "Answer"}
	}
	ch <- llm.ChatEvent{Type: llm.EventDone}
	close(ch)
	return ch, nil
}

type steeringExecutor struct {
	entered, release chan struct{}
	calls            int
}

func (e *steeringExecutor) Execute(ctx context.Context, _ string, _ json.RawMessage) (tool.ToolResult, error) {
	e.calls++
	if e.calls == 1 {
		close(e.entered)
		select {
		case <-e.release:
		case <-ctx.Done():
			return tool.ToolResult{}, ctx.Err()
		}
	}
	return tool.ToolResult{Output: "read result"}, nil
}
func (*steeringExecutor) ToolDefs() []llm.ToolDef      { return []llm.ToolDef{{Name: "read"}} }
func (*steeringExecutor) Names() []string              { return []string{"read"} }
func (*steeringExecutor) Get(string) (tool.Tool, bool) { return nil, false }

func TestHandSteeringReachesModelBeforeRemainingTool(t *testing.T) {
	provider := &steeringProvider{}
	executor := &steeringExecutor{entered: make(chan struct{}), release: make(chan struct{})}
	sess := session.NewSession("hand", "key")
	rt := &runtime.Runtime{LLM: provider, Tools: executor, Session: sess, AgentID: "hand", Model: "test", MaxTurns: 3}
	owner := New(&HarnessBackend{Runtime: rt}, Options{SessionID: sess.ID, MaxIterations: 1})
	stream, err := owner.Start(context.Background(), "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	go func() {
		for range stream.Events {
		}
		close(drained)
	}()
	<-executor.entered
	input, err := owner.EnqueueInput(SteeringQueue, "answer instead")
	if err != nil {
		t.Fatal(err)
	}
	close(executor.release)
	<-drained
	outcome, err := stream.Wait()
	if err != nil || outcome.Cause != nil {
		t.Fatal(outcome, err)
	}
	if executor.calls != 1 || !provider.corrected || len(owner.QueuedInputs()) != 0 {
		t.Fatal("steering not delivered", executor.calls, provider.corrected, owner.QueuedInputs())
	}
	found := false
	for _, entry := range sess.Entries() {
		if entry.ID == "steering_"+input.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("stable correction ID missing")
	}
}

func TestSteeringClaimFreezesTextAndAcknowledgesOnce(t *testing.T) {
	owner := New(nil, Options{SessionID: "s"})
	input, _ := owner.EnqueueInput(SteeringQueue, "original")
	owner.active = true
	owner.state = Running
	message, err := owner.steeringSource(context.Background())
	if err != nil || message == nil {
		t.Fatal(message, err)
	}
	if owner.EditQueuedInput(input.ID, "changed") == nil || owner.RemoveQueuedInput(input.ID) == nil {
		t.Fatal("claimed correction changed")
	}
	retry, err := owner.steeringSource(context.Background())
	if err != nil || retry.ID != message.ID || retry.Text != message.Text {
		t.Fatal(retry, err)
	}
	if err := message.Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if err := message.Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if len(owner.QueuedInputs()) != 0 {
		t.Fatal("acknowledged input remained")
	}
}

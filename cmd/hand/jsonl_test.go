package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func jsonlRuntime(t *testing.T) *runtime.Runtime {
	return &runtime.Runtime{LLM: eventProvider{events: []llm.ChatEvent{{Type: llm.EventTextDelta, Text: "answer\nwith newline"}, {Type: llm.EventDone}}}, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), AgentID: "hand", Model: "test", Workspace: t.TempDir(), MaxTurns: 1}
}
func TestJSONLOneShotOrderedSingleTerminal(t *testing.T) {
	rt := jsonlRuntime(t)
	var sink bytes.Buffer
	reason := ""
	ctx := context.WithValue(context.Background(), jsonOutputKey{}, io.Writer(&sink))
	if err := runOneShot(ctx, rt, "hello", nil, rt.Workspace, &reason, 1); err != nil {
		t.Fatal(err)
	}
	reader := protocol.NewReader(&sink)
	last := uint64(0)
	terminal := 0
	text := ""
	ids := make(map[string]bool)
	for {
		frame, err := reader.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var e protocol.Event
		if err = json.Unmarshal(frame, &e); err != nil {
			t.Fatal(err)
		}
		if e.Version != 1 || e.Sequence <= last || ids[e.EventID] || e.SessionID == "" || e.RequestID == "" || e.Timestamp.IsZero() {
			t.Fatalf("invalid event %+v", e)
		}
		if terminal > 0 {
			t.Fatal("event after terminal")
		}
		ids[e.EventID] = true
		last = e.Sequence
		var payload struct {
			Text   string `json:"text"`
			Status string `json:"status"`
		}
		if err = json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if e.Kind == "text" {
			text += payload.Text
		}
		if e.Kind == "terminal" {
			terminal++
			if payload.Status != string(agentio.Completed) {
				t.Fatalf("terminal %s", e.Payload)
			}
		}
	}
	if terminal != 1 || text != "answer\nwith newline" {
		t.Fatalf("terminal=%d text=%q", terminal, text)
	}
}

type failingJSONWriter struct{}

func (failingJSONWriter) Write([]byte) (int, error) { return 0, errors.New("disconnected") }
func TestJSONLOutputFailureJoinsRun(t *testing.T) {
	rt := jsonlRuntime(t)
	reason := ""
	ctx := context.WithValue(context.Background(), jsonOutputKey{}, io.Writer(failingJSONWriter{}))
	outcome := runOneShotOutcome(ctx, rt, "hello", nil, rt.Workspace, &reason, 1)
	if outcome.Status != agentio.InfrastructureError || outcome.Reason != "event_output_failed" {
		t.Fatalf("outcome %+v", outcome)
	}
	// A new runtime run is accepted only after the failed-output run releases ownership.
	events, err := rt.Run(context.Background(), "again", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
}

type forbiddenTrustInput struct{}

func (forbiddenTrustInput) Read([]byte) (int, error) { panic("JSONL must not read hidden trust input") }
func TestJSONLUntrustedWorkspaceDoesNotReadStdin(t *testing.T) {
	workspace := t.TempDir()
	if err := permissions.Save(permissions.DefaultPath(workspace), permissions.Settings{AlwaysAllow: []string{"bash"}}); err != nil {
		t.Fatal(err)
	}
	store, err := loadPermissionsForInterface(workspace, forbiddenTrustInput{}, false)
	if err != nil || store.IsAlwaysAllowed("bash") {
		t.Fatalf("untrusted permission: %v", err)
	}
}

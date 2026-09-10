package tui

import (
	"context"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"strings"
	"testing"
)

func TestTruncatedApplicationOutputLoadsCompleteSessionResult(t *testing.T) {
	s := session.NewSession("hand", "key")
	raw := strings.Repeat("complete line\n", 20000) + "final marker"
	s.Append(session.ToolResultEntry("call", raw, "", nil))
	controller := &app.Controller{Rt: &runtime.Runtime{Session: s}}
	m := NewModel(controller, t.TempDir())
	m.SetController(controller)
	m.identity.SessionID = s.ID
	m.renderApplicationEvent(app.Event{Kind: "tool_result", Details: app.Details{ToolID: "call", Output: "prefix", Truncated: true}})
	cmd := m.showOutput("")
	if cmd == nil {
		t.Fatal("full result load not requested")
	}
	result := cmd()
	m.Update(result)
	if m.outputView.block.Truncated || m.outputView.block.Output != raw {
		t.Fatal("complete result not installed")
	}
	m.outputView.viewport.GotoBottom()
	if !strings.Contains(m.View(), "final marker") {
		t.Fatal("final content not reachable")
	}
	old := m.outputView
	m.showOutput("")
	m.Update(outputLoaded{viewer: old, block: ToolOutput{Output: "stale"}})
	if m.outputView.block.Output == "stale" {
		t.Fatal("stale load overwrote new viewer")
	}
}

func TestOutputLoadCancelledAndJoinedOnShutdown(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.toolOutputs = []ToolOutput{{Output: "prefix"}}
	m.showOutput("")
	viewer := m.outputView
	entered := make(chan struct{})
	finished := make(chan struct{})
	cmd := m.startOutputLoad(viewer, func(ctx context.Context) (ToolOutput, error) {
		close(entered)
		<-ctx.Done()
		close(finished)
		return ToolOutput{}, ctx.Err()
	})
	<-entered
	m.closeOutputView()
	m.CloseApplication()
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before load stopped")
	}
	if len(m.outputLoads) != 0 {
		t.Fatal("joined load retained")
	}
	m.Update(cmd())
	if m.outputView != nil {
		t.Fatal("cancelled result restored closed viewer")
	}
}

func TestOutputLoadsBoundedWhileReadersStop(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	release := make(chan struct{})
	defer m.CloseApplication()
	defer close(release)
	for i := 0; i < 4; i++ {
		v := &outputViewer{}
		if m.startOutputLoad(v, func(context.Context) (ToolOutput, error) { <-release; return ToolOutput{}, nil }) == nil {
			t.Fatal("early load rejection")
		}
	}
	v := &outputViewer{}
	if m.startOutputLoad(v, func(context.Context) (ToolOutput, error) { t.Error("fifth reader started"); return ToolOutput{}, nil }) != nil {
		t.Fatal("unbounded readers")
	}
	if !strings.Contains(v.status, "still stopping") {
		t.Fatal("capacity error hidden")
	}
	// Deferred release runs before the deferred join.
}

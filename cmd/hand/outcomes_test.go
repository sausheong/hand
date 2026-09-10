package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sausheong/harness/llm"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

func TestOneShotIterationExhaustionIsNotSuccess(t *testing.T) {
	workspace := t.TempDir()
	reason := ""
	hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 2"}}}
	rt := &runtime.Runtime{LLM: completedProvider{}, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), AgentID: "hand", Model: "test", Workspace: workspace, MaxTurns: 1}
	rt.AgentLoop.Hooks = agentio.BuildLifecycleHooks(hooks, workspace, nil, &reason)
	if err := runOneShot(context.Background(), rt, "hello", hooks, workspace, &reason, 1); err == nil {
		t.Fatal("iteration exhaustion returned success")
	}
}

type eventProvider struct {
	completedProvider
	events []llm.ChatEvent
	err    error
}

func (p eventProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	if p.err != nil {
		return nil, p.err
	}
	ch := make(chan llm.ChatEvent, len(p.events))
	for _, e := range p.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func TestOneShotOutcomeMatrix(t *testing.T) {
	done := []llm.ChatEvent{{Type: llm.EventTextDelta, Text: "answer"}, {Type: llm.EventDone}}
	for _, tc := range []struct {
		name      string
		hooks     []config.HookConfig
		provider  eventProvider
		cancelled bool
		status    agentio.RunStatus
		reason    string
		code      int
		verified  bool
	}{
		{name: "answer", provider: eventProvider{events: done}, status: agentio.Completed, reason: "answer_completed", code: 0},
		{name: "verified", provider: eventProvider{events: done}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}}}, status: agentio.Completed, reason: "answer_completed", code: 0, verified: true},
		{name: "optional warning", provider: eventProvider{events: done}, hooks: []config.HookConfig{{Event: "Stop", FailurePolicy: "warn", Command: "sh", Args: []string{"-c", "exit 1"}}}, status: agentio.Completed, reason: "answer_completed", code: 0},
		{name: "stop failure", provider: eventProvider{events: done}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 1"}}}, status: agentio.VerificationFailed, reason: "stop_validator_failed", code: 3},
		{name: "prompt failure", provider: eventProvider{events: done}, hooks: []config.HookConfig{{Event: "UserPromptSubmit", Command: "sh", Args: []string{"-c", "exit 2"}}}, status: agentio.VerificationFailed, reason: "prompt_validator_failed", code: 3},
		{name: "iteration limit", provider: eventProvider{events: done}, hooks: []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 2"}}}, status: agentio.BudgetExhausted, reason: "max_iterations", code: 4},
		{name: "turn limit", provider: eventProvider{events: []llm.ChatEvent{{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: "t1", Name: "absent", Input: json.RawMessage(`{}`)}}, {Type: llm.EventDone}}}, status: agentio.BudgetExhausted, reason: "max_turns", code: 4},
		{name: "provider failure", provider: eventProvider{err: errors.New("fixture failure")}, status: agentio.InfrastructureError, reason: "runtime_error", code: 5},
		{name: "cancelled", cancelled: true, provider: eventProvider{events: done}, status: agentio.Cancelled, reason: "context_cancelled", code: 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			reason := "stale_previous_reason"
			rt := &runtime.Runtime{LLM: tc.provider, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), AgentID: "hand", Model: "test", Workspace: workspace, MaxTurns: 1}
			rt.AgentLoop.Hooks = agentio.BuildLifecycleHooks(tc.hooks, workspace, nil, &reason)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			got := runOneShotOutcome(ctx, rt, "hello", tc.hooks, workspace, &reason, 1)
			if got.Status != tc.status || got.Reason != tc.reason || got.Verified != tc.verified || exitCode(got.Err()) != tc.code {
				t.Fatalf("outcome %+v, exit %d; want %s/%s/%d verified=%v", got, exitCode(got.Err()), tc.status, tc.reason, tc.code, tc.verified)
			}
		})
	}
	if got := exitCode(&invocationError{errors.New("invalid config")}); got != 2 {
		t.Fatalf("configuration exit %d", got)
	}
}

type interruptProvider struct {
	completedProvider
	ready string
}

func (p interruptProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	if err := os.WriteFile(p.ready, []byte("ready"), 0600); err != nil {
		return nil, err
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

// The child runs the production signal context and one-shot driver with a
// local blocking provider. No network call or API credential is involved.
func TestOneShotSignalShutdown(t *testing.T) {
	if ready := os.Getenv("HAND_TEST_SIGNAL_READY"); ready != "" {
		ctx, stop := oneShotSignalContext()
		reason := ""
		rt := &runtime.Runtime{LLM: interruptProvider{ready: ready}, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "test"), Model: "test", MaxTurns: 1}
		rt.AgentLoop.Hooks = agentio.BuildLifecycleHooks(nil, "", nil, &reason)
		err := runOneShot(ctx, rt, "hello", nil, "", &reason, 1)
		stop()
		os.Exit(exitCode(err))
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ready := filepath.Join(t.TempDir(), "ready")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOneShotSignalShutdown$")
			child.Env = append(os.Environ(), "HAND_TEST_SIGNAL_READY="+ready)
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer child.Process.Kill()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					child.Wait()
					t.Fatal("child never reached provider")
				}
			}
			if err := child.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err := child.Wait()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 130 {
				t.Fatalf("signal %v returned %v", sig, err)
			}
		})
	}
}

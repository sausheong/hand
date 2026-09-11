package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
)

type goalJourney struct {
	command      string
	limit        int
	fail, cancel bool
}

// Run through the production constructor and service, including real Stop
// subprocesses. Counters are read only after driveApplication joins the stream.
func runGoalJourney(t *testing.T, scenario goalJourney) (*Model, []string, int) {
	t.Helper()
	workspace := t.TempDir()
	var prompts []string
	checks := 0
	backend := applicationBackend{run: func(ctx context.Context, prompt string) (<-chan app.BackendEvent, error) {
		prompts = append(prompts, prompt)
		events := make(chan app.BackendEvent, 1)
		if scenario.cancel {
			go func() { <-ctx.Done(); close(events) }()
		} else {
			if scenario.fail {
				events <- app.BackendEvent{Err: errors.New("provider failed")}
			} else {
				events <- app.BackendEvent{Done: true}
			}
			close(events)
		}
		return events, nil
	}}
	options := app.Options{MaxIterations: scenario.limit}
	if scenario.command != "" {
		hooks := []config.HookConfig{{Event: "Stop", Command: "sh", Args: []string{"-c", scenario.command}}}
		options.Check = func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
			checks++
			return agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
		}
	}
	m := applicationModel(t, app.New(backend, options), workspace)
	cmd := m.startRun("initial")
	if scenario.cancel {
		m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	}
	driveApplication(t, m, cmd)
	if m.lastOutcome == nil || m.running {
		t.Fatal("goal did not settle")
	}
	if strings.Count(strings.Join(m.transcript, "\n"), "[result]") != 1 {
		t.Fatal("expected exactly one terminal result")
	}
	return m, prompts, checks
}

// Names retain the original regression mapping; all exercise service ownership.
func TestMaybeContinueGoalLoop_NoHooksConfigured(t *testing.T) {
	m, prompts, checks := runGoalJourney(t, goalJourney{limit: 10})
	if len(prompts) != 1 || checks != 0 || m.lastOutcome.Status != agentio.Completed || m.lastOutcome.Verified {
		t.Fatal(prompts, checks, m.lastOutcome)
	}
}

func TestMaybeContinueGoalLoop_SkipsOnTurnErrored(t *testing.T) {
	m, prompts, checks := runGoalJourney(t, goalJourney{limit: 10, command: "exit 2", fail: true})
	if len(prompts) != 1 || checks != 0 || m.lastOutcome.Status != agentio.InfrastructureError {
		t.Fatal(prompts, checks, m.lastOutcome)
	}
}

func TestMaybeContinueGoalLoop_SkipsOnGoalCancelled(t *testing.T) {
	m, prompts, checks := runGoalJourney(t, goalJourney{limit: 10, command: "exit 2", cancel: true})
	if len(prompts) > 1 || checks != 0 || m.lastOutcome.Status != agentio.Cancelled {
		t.Fatal(prompts, checks, m.lastOutcome)
	}
}

func TestMaybeContinueGoalLoop_ChecksThenStopsAtIterationCap(t *testing.T) {
	m, prompts, checks := runGoalJourney(t, goalJourney{limit: 2, command: "exit 2"})
	if len(prompts) != 2 || checks != 2 || m.lastOutcome.Status != agentio.BudgetExhausted || m.lastOutcome.Reason != "max_iterations" || m.lastOutcome.Iterations != 2 {
		t.Fatal(prompts, checks, m.lastOutcome)
	}
}

func TestMaybeContinueGoalLoop_ReturnsCmdThatEvaluatesTheConfiguredHook(t *testing.T) {
	_, prompts, checks := runGoalJourney(t, goalJourney{limit: 2, command: "echo 'next step'; exit 2"})
	if checks != 2 || len(prompts) != 2 || prompts[1] != "next step" {
		t.Fatal(prompts, checks)
	}
}

func TestStartAutoContinue_IncrementsIterationAndUsesDistinctTranscriptLine(t *testing.T) {
	m, prompts, _ := runGoalJourney(t, goalJourney{limit: 2, command: "echo 'next step'; exit 2"})
	text := strings.Join(m.transcript, "\n")
	if m.lastOutcome.Iterations != 2 || len(prompts) != 2 || prompts[1] != "next step" || !strings.Contains(text, "attempt 2") || !strings.Contains(text, "next step") {
		t.Fatal(text, prompts, m.lastOutcome)
	}
	for _, block := range m.sourceBlocks {
		if block.Block.Kind == "user" && block.Block.Text == "next step" {
			t.Fatal("continuation rendered as user input")
		}
	}
}

func TestUpdate_RunEndedMsg_DispatchesGoalLoopCheckWhenHooksConfigured(t *testing.T) {
	m, _, checks := runGoalJourney(t, goalJourney{limit: 10, command: "exit 0"})
	if checks != 1 || !m.lastOutcome.Verified || m.lastOutcome.Status != agentio.Completed {
		t.Fatal(checks, m.lastOutcome)
	}
}

func TestUpdate_GoalLoopResultMsg_ContinuesWhenOutcomeSaysSo(t *testing.T) {
	m, prompts, _ := runGoalJourney(t, goalJourney{limit: 2, command: "echo 'keep going'; exit 2"})
	if m.lastOutcome.Iterations != 2 || len(prompts) != 2 || prompts[1] != "keep going" {
		t.Fatal(prompts, m.lastOutcome)
	}
}

func TestUpdate_GoalLoopResultMsg_NoOpWhenOutcomeSaysDone(t *testing.T) {
	m, prompts, checks := runGoalJourney(t, goalJourney{limit: 10, command: "exit 0"})
	if len(prompts) != 1 || checks != 1 || m.lastOutcome.Iterations != 1 {
		t.Fatal(prompts, checks, m.lastOutcome)
	}
}

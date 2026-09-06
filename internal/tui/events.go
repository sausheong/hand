// Package tui is Hand's Bubble Tea terminal UI.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
)

// runEndedMsg marks that an AgentEvent channel has closed. harness's
// Run goroutine can exit several error paths without ever emitting a
// terminal EventDone (see runtime.go's early r.emit(EventError); return
// paths), so relying on EventDone alone to know a turn ended would
// leave the UI stuck in the "running" state on those paths. StreamEvents
// sends this exactly once, after ranging over events completes.
type runEndedMsg struct{}

// goalLoopResultMsg carries the result of evaluating a turn's Stop-event
// hooks (see agentio.EvaluateStopHooks) — produced by the tea.Cmd
// Model.maybeContinueGoalLoop returns from runEndedMsg's handling in
// Update, so the (potentially slow, subprocess-spawning) hook check runs
// off the Update goroutine like any other tea.Cmd.
type goalLoopResultMsg struct{ outcome agentio.GoalLoopOutcome }

// programSender is satisfied by *tea.Program. Declared as an interface
// (rather than taking *tea.Program directly) so StreamEvents is
// testable without a real terminal.
type programSender interface {
	Send(msg tea.Msg)
}

// StreamEvents forwards every AgentEvent from events into p as a
// tea.Msg, in order, then sends a single runEndedMsg once events
// closes. Intended to run in its own goroutine for the lifetime of one
// agent turn — an ordinary tea.Cmd cannot represent an unbounded stream
// like this, since a Cmd only ever produces one terminal Msg.
func StreamEvents(p programSender, events <-chan runtime.AgentEvent) {
	for ev := range events {
		p.Send(ev)
	}
	p.Send(runEndedMsg{})
}

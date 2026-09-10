package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/harness/llm"
)

// NewApplicationModel constructs a terminal whose turns and goal checks are
// owned by the application service. No runtime runner is installed.
func NewApplicationModel(service *app.Service, workspace string) (*Model, error) {
	if service == nil {
		return nil, errors.New("terminal requires an application service")
	}
	m := newPresentationModel(workspace)
	m.service = service
	return m, nil
}

const maxPresentationEvents = 32

type applicationMsg struct {
	following []app.Event
	stream    *app.Stream
	event     app.Event
	outcome   *agentio.RunOutcome
}

// One pending command reads one event. There is no unbounded forwarding queue
// or goroutine blocked in Program.Send when the terminal closes.
func nextApplicationEvent(stream *app.Stream) tea.Cmd {
	return func() tea.Msg {
		if event, ok := <-stream.Events; ok {
			msg := applicationMsg{stream: stream, event: event}
			if event.Kind != "text" {
				return msg
			}
			bytes := len(event.Text)
			for len(msg.following)+1 < maxPresentationEvents && bytes < app.MaxEventTextBytes {
				select {
				case next, ok := <-stream.Events:
					if !ok {
						return msg
					}
					msg.following = append(msg.following, next)
					bytes += len(next.Text)
					if next.Kind != "text" {
						return msg
					}
				default:
					return msg
				}
			}
			return msg
		}
		outcome, err := stream.Wait()
		if err != nil {
			outcome = agentio.RunOutcome{Status: agentio.InfrastructureError, Reason: "application_failed", Cause: err}
		}
		return applicationMsg{stream: stream, outcome: &outcome, event: stream.FinalEvent()}
	}
}

func (m *Model) startApplicationGoal(prompt string, images []llm.ImageContent) tea.Cmd {
	stream, err := m.service.Start(context.Background(), prompt, images)
	if err != nil {
		m.running = false
		m.finishGoal(agentio.RunOutcome{Status: agentio.InfrastructureError, Reason: "run_start_failed", Cause: err})
		return nil
	}
	m.activeStream = stream
	m.identity = stream.Identity()
	m.cancel = func() { stream.Cancel() }
	m.refreshViewport()
	return tea.Batch(nextApplicationEvent(stream), m.startExtensionPolling())
}

func (m *Model) handleApplicationMessage(msg applicationMsg) tea.Cmd {
	if msg.stream != m.activeStream || !m.running {
		return nil
	}
	if msg.outcome != nil {
		if msg.event.Details.UsageKnown {
			m.renderApplicationEvent(app.Event{Kind: "session_usage", Model: msg.event.Model, Details: msg.event.Details})
		}
		m.running = false
		m.goalChecking = false
		m.activeStream = nil
		m.cancel = nil
		m.dismissApproval()
		m.flushStream()
		m.lastTurnDuration = time.Since(m.turnStart)
		m.goalIteration = msg.outcome.Iterations
		m.finishGoal(*msg.outcome)
		if m.quitAfterRun {
			return tea.Quit
		}
		if msg.outcome.Status == agentio.Completed {
			return m.startQueuedFollowup(false)
		}
		return nil
	}
	if !m.goalCancelled {
		m.renderApplicationEvent(msg.event)
		for _, event := range msg.following {
			m.renderApplicationEvent(event)
		}
	}
	m.refreshViewport()
	return nextApplicationEvent(msg.stream)
}

func (m *Model) renderApplicationEvent(e app.Event) {
	m.lastModelInfo = e.Model
	d := e.Details
	switch e.Kind {
	case "approval_required":
		m.dismissApproval()
		m.pendingApprovalID = e.ApprovalID
		preview := e.Text
		if e.Truncated {
			preview += "\n[approval display truncated]"
		}
		m.pending = &agentio.ApprovalRequest{Identity: m.identity, Tool: d.ToolName, Input: json.RawMessage(d.ToolInput), Preview: preview}
		m.recordApproval("pending")
	case "approval_resolved":
		if m.pendingApprovalID == e.ApprovalID {
			if m.outputView != nil && m.outputView.approval {
				m.closeOutputView()
			}
			m.pending = nil
			m.pendingApprovalID = ""
		}
	case "warning":
		m.appendSourceBlock(TranscriptBlock{Kind: "error", Text: e.Text})
	case "state":
		m.goalChecking = e.State == app.CheckingCompletion
		if e.State == app.Running {
			m.goalIteration = e.Iteration
		}
	case "text":
		m.streamBuf.WriteString(sanitizeForTerminal(e.Text))
	case "tool_call":
		m.flushStream()
		m.toolCallsThisTurn++
		name := sanitizeForTerminal(d.ToolName)
		if !d.ToolPresent {
			name = "?"
		}
		m.appendSourceBlock(TranscriptBlock{Kind: "tool_call", Text: name, Detail: d.ToolInput, Truncated: d.Truncated})
	case "tool_result":
		block := ToolOutput{Output: d.Output, Error: d.ToolError, Truncated: d.Truncated, SessionID: m.identity.SessionID, ToolID: d.ToolID}
		m.toolOutputs = append(m.toolOutputs, block)
		m.appendSourceBlock(TranscriptBlock{Kind: "tool_result", Text: d.Output, Detail: d.ToolError, Truncated: d.Truncated})
	case "session_usage":
		m.sessionUsage = llm.Usage{InputTokens: d.InputTokens, OutputTokens: d.OutputTokens, CacheCreationInputTokens: d.CacheCreationInputTokens, CacheReadInputTokens: d.CacheReadInputTokens}
		m.usageRequests, m.usageUnknown = d.UsageRequests, d.UsageUnknown
		m.usagePriorUnknown = d.UsagePriorUnknown
	case "context_usage":
		m.lastRequestUsage = nil
		if d.UsageKnown {
			m.lastRequestUsage = &llm.Usage{InputTokens: d.InputTokens}
		}
	case "usage":
		m.lastUsage = nil
		if d.UsageKnown {
			m.lastUsage = &llm.Usage{InputTokens: d.InputTokens, OutputTokens: d.OutputTokens, CacheCreationInputTokens: d.CacheCreationInputTokens, CacheReadInputTokens: d.CacheReadInputTokens}
			m.sessionUsage = addUsage(m.sessionUsage, m.lastUsage)
		}
	case "turn_end":
		m.flushStream()
		m.lastTurnDuration = time.Since(m.turnStart)
	case "continuation":
		m.goalIteration = e.Iteration
		m.toolCallsThisTurn = 0
		m.turnStart = time.Now()
		line := fmt.Sprintf("↻ continuing (goal iteration %d): %s", e.Iteration, sanitizeForTerminal(e.Text))
		if e.Truncated {
			line += " [prompt display truncated]"
		}
		m.appendSourceBlock(TranscriptBlock{Kind: "status", Text: line})
	case "compaction_start":
		m.appendSourceBlock(TranscriptBlock{Kind: "compaction", State: "running"})
	case "compaction_done", "compaction_skipped":
		state := "automatic"
		text := d.Summary
		if d.Skipped != "" {
			state = "skipped"
			text = d.Skipped
		}
		m.appendSourceBlock(TranscriptBlock{Kind: "compaction", State: state, Text: text, Count: d.TurnsCompacted, TokensBefore: d.TokensBefore, TokensAfter: d.TokensAfter, Truncated: d.Truncated})
	}
}

// CloseApplication joins a goal after Program.Run has returned, including
// terminal disconnects and program errors. Do not call concurrently with Update.
func (m *Model) CloseApplication() {
	m.closePermissions()
	if m.controller != nil && m.controller.OptionalMCP != nil {
		m.controller.OptionalMCP.Close()
	}
	m.closeProcesses()
	m.closeMarkdownLayout()
	m.closeOutputView()
	m.closeOutputLoads()
	if m.compactDone != nil {
		m.compactCancel()
		<-m.compactDone
	}
	if m.sessionDone != nil {
		m.sessionCancel()
		<-m.sessionDone
	}
	if m.profileDone != nil {
		m.profileCancel()
		<-m.profileDone
	}
	if m.activeStream != nil {
		m.activeStream.Close()
		m.activeStream.Wait()
	}
}

// SetContextLimit applies the same explicit override supplied to the runtime.
func (m *Model) SetContextLimit(limit int) {
	if limit > 0 {
		m.contextWindow = limit
	}
}

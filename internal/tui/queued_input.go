package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/hand/internal/app"
	"strings"
	"time"
)

func isQueueCommand(text string) bool {
	fields := strings.Fields(text)
	return len(fields) > 0 && (fields[0] == "/steer" || fields[0] == "/followup" || fields[0] == "/queue")
}

func (m *Model) queueMessage(text string) {
	m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
	m.refreshViewport()
}

func (m *Model) runQueueCommand(name, args string) tea.Cmd {
	if m.service == nil {
		m.queueMessage("Input queues require the application service.")
		return nil
	}
	if name == "/followup" || name == "/steer" {
		queue := app.FollowupQueue
		if name == "/steer" {
			queue = app.SteeringQueue
		}
		input, err := m.service.EnqueueInput(queue, args)
		if err != nil {
			m.queueMessage(err.Error())
			return nil
		}
		m.queueMessage("Queued " + string(queue) + " " + input.ID + ": " + input.Text)
		if !m.running && queue == app.FollowupQueue {
			return m.startQueuedFollowup(false)
		}
		return nil
	}
	action, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)
	switch action {
	case "":
		inputs := m.service.QueuedInputs()
		if len(inputs) == 0 {
			m.queueMessage("No queued input.")
		}
		for _, input := range inputs {
			state := "pending"
			if input.Claimed {
				state = "delivery unresolved"
			}
			m.queueMessage(fmt.Sprintf("%s [%s; %s] %s", input.ID, input.Queue, state, input.Text))
		}
	case "edit":
		id, text, _ := strings.Cut(rest, " ")
		if err := m.service.EditQueuedInput(id, strings.TrimSpace(text)); err != nil {
			m.queueMessage(err.Error())
		} else {
			m.queueMessage("Updated queued input " + id)
		}
	case "remove":
		if err := m.service.RemoveQueuedInput(rest); err != nil {
			m.queueMessage(err.Error())
		} else {
			m.queueMessage("Removed queued input " + rest)
		}
	case "run":
		if rest != "" {
			m.queueMessage("Usage: /queue run")
			return nil
		}
		return m.startQueuedFollowup(true)
	default:
		m.queueMessage("Usage: /queue [edit <ID> <text> | remove <ID> | run]")
	}
	return nil
}

func (m *Model) startQueuedFollowup(reportEmpty bool) tea.Cmd {
	if m.service == nil || m.running || m.compacting || m.sessionChanging || m.profileChanging {
		if reportEmpty {
			m.queueMessage("Active work must settle before a follow-up can start.")
		}
		return nil
	}
	found := false
	for _, input := range m.service.QueuedInputs() {
		if input.Queue == app.FollowupQueue {
			found = true
			break
		}
	}
	if !found {
		if reportEmpty {
			m.queueMessage("No queued follow-up.")
		}
		return nil
	}
	stream, input, err := m.service.StartFollowup(context.Background())
	if err != nil {
		m.queueMessage("Follow-up remains queued: " + err.Error())
		return nil
	}
	m.abandonGoal("queued_followup")
	m.goalIteration = 1
	m.goalCancelled = false
	m.quitAfterRun = false
	m.goalActive = true
	m.lastOutcome = nil
	m.running = true
	m.turnStart = time.Now()
	m.toolCallsThisTurn = 0
	m.appendNotice("> "+sanitizeForTerminal(input.Text), "userLineStyle")
	m.activeStream = stream
	m.identity = stream.Identity()
	m.cancel = func() { stream.Cancel() }
	m.refreshViewport()
	return tea.Batch(nextApplicationEvent(stream), m.startExtensionPolling())
}

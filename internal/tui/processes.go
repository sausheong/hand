package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type processTask struct {
	cancel context.CancelFunc
	done   chan struct{}
	text   string
	err    error
}
type processCommandDone struct{ task *processTask }

// This entrypoint handles explicit terminal-user commands only. Model tool
// requests must pass their own permission admission before reaching the registry.
func (m *Model) runProcessCommand(input string) tea.Cmd {
	report := func(text string) tea.Cmd {
		m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
		m.refreshViewport()
		return nil
	}
	if m.controller == nil || m.controller.Processes == nil {
		return report("Background processes are unavailable")
	}
	p := m.controller.Processes
	action, rest, _ := strings.Cut(input, " ")
	rest = strings.TrimSpace(rest)
	switch action {
	case "list", "":
		var lines []string
		for _, info := range p.List() {
			lines = append(lines, fmt.Sprintf("%s running=%t %s", info.ID, info.Running, info.Command))
		}
		if len(lines) == 0 {
			return report("No background processes")
		}
		return report(strings.Join(lines, "\n"))
	case "read":
		s, err := p.Read(rest)
		if err != nil {
			return report(err.Error())
		}
		return report(fmt.Sprintf("%s running=%t exit=%d\nstdout (%d bytes, truncated=%t):\n%s\nstderr (%d bytes, truncated=%t):\n%s\n%s", s.ID, s.Running, s.ExitCode, s.StdoutBytes, s.StdoutTruncated, s.Stdout, s.StderrBytes, s.StderrTruncated, s.Stderr, s.Error))
	case "cancel", "forget":
		var err error
		if action == "cancel" {
			err = p.Cancel(rest)
		} else {
			err = p.Forget(rest)
		}
		if err != nil {
			return report(err.Error())
		}
		return report(action + " requested for " + rest)
	case "start", "send", "wait":
	default:
		return report("Usage: /process start <shell command> | list | read/cancel/wait/forget <ID> | send <ID> <line>")
	}
	if m.processTask != nil {
		return report("A process operation is pending; read, list and cancel remain available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &processTask{cancel: cancel, done: make(chan struct{})}
	m.processTask = task
	go func() {
		defer close(task.done)
		defer cancel()
		switch action {
		case "start":
			info, err := p.Start(ctx, rest)
			task.err = err
			task.text = "Started background process " + info.ID
		case "send":
			id, line, ok := strings.Cut(rest, " ")
			if !ok {
				task.err = fmt.Errorf("usage: /process send <ID> <line>")
				return
			}
			task.err = p.Send(ctx, id, []byte(line+"\n"))
			task.text = "Input sent to " + id
		case "wait":
			s, err := p.Wait(ctx, rest)
			task.err = err
			task.text = fmt.Sprintf("Process %s exited with status %d: %s", s.ID, s.ExitCode, s.Error)
		}
	}()
	return func() tea.Msg { <-task.done; return processCommandDone{task} }
}
func (m *Model) finishProcessCommand(msg processCommandDone) tea.Cmd {
	if m.processTask != msg.task {
		return nil
	}
	<-msg.task.done
	m.processTask = nil
	text := msg.task.text
	if msg.task.err != nil {
		text = msg.task.err.Error()
	}
	m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
	m.refreshViewport()
	return nil
}
func (m *Model) closeProcesses() {
	if m.processTask != nil {
		m.processTask.cancel()
	}
	if m.controller != nil && m.controller.Processes != nil {
		m.controller.Processes.Close()
	}
	if m.processTask != nil {
		<-m.processTask.done
		m.processTask = nil
	}
}

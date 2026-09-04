package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

// Runner is the subset of *runtime.Runtime the TUI needs, so tests can
// supply a fake instead of a real provider-backed Runtime.
type Runner interface {
	Run(ctx context.Context, userMsg string, images []llm.ImageContent) (<-chan runtime.AgentEvent, error)
}

// Model is the agcode Bubble Tea program. It uses pointer-receiver
// Init/Update/View methods (rather than the value-receiver style most
// Bubble Tea examples use) so a *tea.Program reference can be injected
// after construction via BindProgram — breaking the construction cycle
// between the Program and the approval hook that needs to Send into it
// (see internal/agentio.Sender and cmd/agcode/main.go).
type Model struct {
	rt      Runner
	program *tea.Program

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	transcript []string
	streamBuf  strings.Builder

	running bool
	cancel  context.CancelFunc
}

// NewModel builds an agcode TUI model driving rt. Call BindProgram with
// the *tea.Program constructed from this model before calling Run on
// that program.
func NewModel(rt Runner) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.Focus()

	vp := viewport.New(80, 20)
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	return &Model{
		rt:       rt,
		textarea: ta,
		viewport: vp,
		spinner:  sp,
	}
}

// BindProgram gives the model a reference to its own running Program,
// used to launch the per-turn StreamEvents goroutine.
func (m *Model) BindProgram(p *tea.Program) {
	m.program = p
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case runtime.AgentEvent:
		m.handleAgentEvent(msg)
		m.refreshViewport()
		return m, nil

	case runEndedMsg:
		m.running = false
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.running && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		return m, tea.Quit

	case "enter":
		if m.running {
			return m, nil
		}
		text := strings.TrimSpace(m.textarea.Value())
		if text == "" {
			return m, nil
		}
		return m, m.startRun(text)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// startRun begins one agent turn and returns nil (not a tea.Cmd that
// produces a Msg): StreamEvents runs in its own goroutine for the
// lifetime of the turn, since a tea.Cmd can only ever produce a single
// terminal Msg and this stream is unbounded until EventDone /
// EventError / channel-close.
func (m *Model) startRun(text string) tea.Cmd {
	m.transcript = append(m.transcript, "> "+text)
	m.textarea.Reset()
	m.running = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	events, err := m.rt.Run(ctx, text, nil)
	if err != nil {
		m.running = false
		m.transcript = append(m.transcript, "error: "+err.Error())
		cancel()
		m.refreshViewport()
		return nil
	}

	go StreamEvents(m.program, events)
	m.refreshViewport()
	return nil
}

func (m *Model) handleAgentEvent(ev runtime.AgentEvent) {
	switch ev.Type {
	case runtime.EventTextDelta:
		m.streamBuf.WriteString(ev.Text)

	case runtime.EventToolCallStart:
		m.flushStream()
		name := "?"
		if ev.ToolCall != nil {
			name = ev.ToolCall.Name
		}
		m.transcript = append(m.transcript, fmt.Sprintf("[tool: %s]", name))

	case runtime.EventToolResult:
		if ev.Result != nil && ev.Result.Error != "" {
			m.transcript = append(m.transcript, "  ✗ "+ev.Result.Error)
		} else {
			m.transcript = append(m.transcript, "  ✓")
		}

	case runtime.EventError:
		m.flushStream()
		errText := "unknown error"
		if ev.Error != nil {
			errText = ev.Error.Error()
		}
		m.transcript = append(m.transcript, "error: "+errText)

	case runtime.EventDone:
		m.flushStream()
		m.running = false
	}
}

func (m *Model) flushStream() {
	if m.streamBuf.Len() == 0 {
		return
	}
	m.transcript = append(m.transcript, m.streamBuf.String())
	m.streamBuf.Reset()
}

func (m *Model) refreshViewport() {
	content := strings.Join(m.transcript, "\n")
	if m.streamBuf.Len() > 0 {
		content += "\n" + m.streamBuf.String()
	}
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *Model) resize(width, height int) {
	m.viewport.Width = width
	const inputHeight, statusHeight = 3, 1
	viewportHeight := height - inputHeight - statusHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.Height = viewportHeight
	m.textarea.SetWidth(width)
	m.refreshViewport()
}

func (m *Model) statusLine() string {
	if m.running {
		return m.spinner.View() + " working..."
	}
	return "ready"
}

func (m *Model) View() string {
	return m.viewport.View() + "\n" + m.statusLine() + "\n" + m.textarea.View()
}

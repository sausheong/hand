package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tokens"
)

// Runner is the subset of *runtime.Runtime the TUI needs, so tests can
// supply a fake instead of a real provider-backed Runtime.
type Runner interface {
	Run(ctx context.Context, userMsg string, images []llm.ImageContent) (<-chan runtime.AgentEvent, error)
}

// Model is the Hand Bubble Tea program. It uses pointer-receiver
// Init/Update/View methods (rather than the value-receiver style most
// Bubble Tea examples use) so a *tea.Program reference can be injected
// after construction via BindProgram — breaking the construction cycle
// between the Program and the approval hook that needs to Send into it
// (see internal/agentio.Sender and cmd/hand/main.go).
type Model struct {
	rt         Runner
	workspace  string
	program    *tea.Program
	controller *Controller // optional; nil in tests that only exercise Runner

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	transcript []string
	streamBuf  strings.Builder

	running bool
	pending *agentio.ApprovalRequest
	cancel  context.CancelFunc

	// lastUsage holds the token counts from the most recent turn's
	// EventDone, nil until the first turn completes with usage reported
	// (a provider may not report usage at all). Surfaced via /usage and
	// the always-on status line's context/turn-token figures.
	lastUsage *llm.Usage

	// sessionUsage accumulates lastUsage's fields across every completed
	// turn since this process started (not persisted — a fresh hand
	// process, even resuming the same saved session, starts back at
	// zero, matching how the rest of this status line resets).
	sessionUsage llm.Usage

	// model is the active "provider/model" string, kept in sync with
	// SetBanner and /model so contextWindow can be recomputed on switch.
	model string
	// contextWindow is model's max input tokens (tokens.ContextWindowFor),
	// 0 if never set (tests that skip SetBanner/setModel).
	contextWindow int

	// turnStart marks when the in-flight (or, once finished, most
	// recent) turn's Run() began — read live while running for the
	// status line's elapsed-time display.
	turnStart time.Time
	// lastTurnDuration is frozen at the most recently completed turn's
	// wall-clock time, set once on runEndedMsg (the one signal harness
	// guarantees fires exactly once per turn on every exit path — see
	// runEndedMsg's doc comment in events.go). Zero until a turn ends.
	lastTurnDuration time.Duration

	// suggestIndex is the highlighted row in the slash-command
	// auto-complete dropdown (see commandSuggestions/renderSuggestions in
	// commands.go), moved by the up/down keys while the dropdown is shown.
	suggestIndex int

	// termWidth/termHeight cache the last WindowSizeMsg. The approval
	// panel's height varies with the pending request's Preview (a diff
	// can be several lines; a bash command is one), so the viewport's
	// height can't be a fixed constant computed only on resize — it's
	// recomputed in refreshViewport, using these cached dimensions,
	// every time the transcript or approval state changes.
	termWidth, termHeight int
}

// NewModel builds a Hand TUI model driving rt, with workspace used to
// resolve image paths referenced in chat messages (see
// agentio.ExtractImagePaths). Call BindProgram with the *tea.Program
// constructed from this model before calling Run on that program.
func NewModel(rt Runner, workspace string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.SetWidth(78) // 80 minus the 1-column border on each side (see View/resize)
	ta.SetHeight(3)
	ta.Focus()

	vp := viewport.New(80, 20)
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle))

	return &Model{
		rt:         rt,
		workspace:  workspace,
		textarea:   ta,
		viewport:   vp,
		spinner:    sp,
		termWidth:  80,
		termHeight: 24,
	}
}

// BindProgram gives the model a reference to its own running Program,
// used to launch the per-turn StreamEvents goroutine.
func (m *Model) BindProgram(p *tea.Program) {
	m.program = p
}

// SetBanner prepends the startup banner (version/model/workspace) to the
// transcript. Call once, right after NewModel and before LoadHistory, so
// the banner sits above any resumed conversation.
func (m *Model) SetBanner(version, model, workspace string) {
	m.setModel(model)
	banner := []string{
		userLineStyle.Render("Hand") + statusIdleStyle.Render(" "+version),
		statusIdleStyle.Render(model),
		statusIdleStyle.Render(workspace),
		"",
	}
	m.transcript = append(banner, m.transcript...)
	m.refreshViewport()
}

// setModel updates the active model name and recomputes its context
// window (tokens.ContextWindowFor), so the status line's "ctx" figure
// tracks whichever model is actually active. Called at startup
// (SetBanner) and after a successful /model switch.
func (m *Model) setModel(model string) {
	m.model = model
	m.contextWindow = tokens.ContextWindowFor(model, 0)
}

// SetController wires up slash commands that need state beyond the
// Runner interface (/model, /new, /compact). Without it those commands
// report themselves unavailable; /exit, /help, and /clear work regardless.
func (m *Model) SetController(c *Controller) {
	m.controller = c
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
		m.lastTurnDuration = time.Since(m.turnStart)
		m.refreshViewport()
		return m, nil

	case agentio.ApprovalRequest:
		req := msg
		m.pending = &req
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		switch msg.String() {
		case "y", "Y":
			m.pending.Respond <- agentio.DecisionOnce
			m.transcript = append(m.transcript, approvedStyle.Render(fmt.Sprintf("  approved: %s", m.pending.Tool)))
			m.pending = nil
			m.refreshViewport()
		case "a", "A":
			m.pending.Respond <- agentio.DecisionAlways
			m.transcript = append(m.transcript, approvedStyle.Render(fmt.Sprintf("  always allowed: %s", m.pending.Tool)))
			m.pending = nil
			m.refreshViewport()
		default:
			m.pending.Respond <- agentio.DecisionDeny
			m.transcript = append(m.transcript, deniedStyle.Render(fmt.Sprintf("  denied: %s", m.pending.Tool)))
			m.pending = nil
			m.refreshViewport()
		}
		return m, nil
	}

	// While the slash-command auto-complete dropdown is showing, up/down
	// move the highlight, tab completes the highlighted name into the
	// input (leaving room to type arguments), enter runs it directly, and
	// esc dismisses it — all instead of their normal textarea behavior.
	if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
		switch msg.String() {
		case "up", "ctrl+p":
			m.suggestIndex--
			if m.suggestIndex < 0 {
				m.suggestIndex = len(suggestions) - 1
			}
			return m, nil
		case "down", "ctrl+n":
			m.suggestIndex = (m.suggestIndex + 1) % len(suggestions)
			return m, nil
		case "tab":
			m.completeSuggestion(suggestions)
			return m, nil
		case "enter":
			if m.suggestIndex < 0 || m.suggestIndex >= len(suggestions) {
				m.suggestIndex = 0
			}
			text := suggestions[m.suggestIndex].name
			m.textarea.Reset()
			m.suggestIndex = 0
			return m, m.handleCommand(text)
		case "esc":
			m.textarea.Reset()
			m.suggestIndex = 0
			return m, nil
		}
	}

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
		if isCommand(text) {
			m.textarea.Reset()
			return m, m.handleCommand(text)
		}
		return m, m.startRun(text)
	}

	m.suggestIndex = 0
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
	cleanText, images := agentio.ExtractImagePaths(m.workspace, text)
	m.transcript = append(m.transcript, userLineStyle.Render("> "+cleanText))
	m.textarea.Reset()
	m.running = true
	m.turnStart = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// The original text (with the real path, not the placeholder) is
	// sent to the model alongside images — the model may benefit from
	// the literal filename/path context next to the image bytes.
	events, err := m.rt.Run(ctx, text, images)
	if err != nil {
		m.running = false
		m.transcript = append(m.transcript, errorLineStyle.Render("error: "+err.Error()))
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
		m.streamBuf.WriteString(sanitizeForTerminal(ev.Text))

	case runtime.EventToolCallStart:
		m.flushStream()
		name := "?"
		var input json.RawMessage
		if ev.ToolCall != nil {
			name = ev.ToolCall.Name
			input = ev.ToolCall.Input
		}
		line := fmt.Sprintf("[tool: %s]", name)
		if detail := summarizeToolCall(name, input); detail != "" {
			line = fmt.Sprintf("[tool: %s] %s", name, detail)
		}
		m.transcript = append(m.transcript, toolCallStyle.Render(line))

	case runtime.EventToolResult:
		switch {
		case ev.Result != nil && ev.Result.Error != "":
			m.transcript = append(m.transcript, toolErrStyle.Render("  ✗ "+ev.Result.Error))
		case ev.Result != nil:
			line := toolOKStyle.Render("  ✓")
			if snippet := summarizeToolResult(ev.Result.Output); snippet != "" {
				line += "\n" + toolCallStyle.Render(snippet)
			}
			m.transcript = append(m.transcript, line)
		default:
			m.transcript = append(m.transcript, toolOKStyle.Render("  ✓"))
		}

	case runtime.EventError:
		m.flushStream()
		errText := "unknown error"
		if ev.Error != nil {
			errText = ev.Error.Error()
		}
		m.transcript = append(m.transcript, errorLineStyle.Render("error: "+errText))

	case runtime.EventDone:
		m.flushStream()
		m.running = false
		m.lastUsage = ev.Usage
		m.sessionUsage = addUsage(m.sessionUsage, ev.Usage)
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
	m.viewport.SetContent(wrapToWidth(content, m.termWidth))

	// inputHeight is the textarea's own rows; borderRows accounts for the
	// rounded border View() draws around it (1 row top + 1 bottom).
	// bottomHeight is the status line when idle, or the whole approval
	// panel (preview + question) when a request is pending — recomputed
	// here, not just on resize, since the panel's height depends on the
	// current Preview, not just the terminal size.
	const inputHeight, borderRows = 3, 2
	viewportHeight := m.termHeight - inputHeight - borderRows - m.bottomHeight()
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.Width = m.termWidth
	m.viewport.Height = viewportHeight
	m.viewport.GotoBottom()
}

// wrapToWidth word-wraps s (which may contain lipgloss/ANSI styling) to
// width columns, so long lines break instead of overflowing the
// viewport's fixed-height scroll region. bubbles/viewport counts
// content by newline, not display row, so unwrapped long lines would
// desync scrolling from what's actually visible.
func wrapToWidth(s string, width int) string {
	if width < 1 {
		return s
	}
	return lipgloss.NewStyle().Width(width).Render(s)
}

func (m *Model) resize(width, height int) {
	m.termWidth = width
	m.termHeight = height
	m.textarea.SetWidth(width - 2) // border consumes 1 column each side
	m.refreshViewport()
}

// bottomHeight returns how many lines the status/approval area below the
// viewport currently occupies.
func (m *Model) bottomHeight() int {
	if m.pending != nil {
		if m.pending.Preview == "" {
			return 1 // just the question line
		}
		wrapped := wrapToWidth(renderPreview(m.pending.Preview), m.termWidth)
		return strings.Count(wrapped, "\n") + 1 /* last preview line */ + 1 /* question line */
	}
	if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
		return len(suggestions)
	}
	return strings.Count(wrapToWidth(m.statusLine(), m.termWidth), "\n") + 1
}

// statusLine is the always-visible footer: run state (with a live
// elapsed timer while a turn is in flight) plus the context/token gauge
// from usageLine. Unlike /usage (a one-shot transcript entry), this is
// redrawn on every Update — including the spinner's own tick — so the
// elapsed time visibly counts up during a turn.
func (m *Model) statusLine() string {
	left := statusIdleStyle.Render("ready")
	if m.running {
		left = m.spinner.View() + statusIdleStyle.Render(fmt.Sprintf(" working... %s", formatDuration(time.Since(m.turnStart))))
	}
	return left + statusIdleStyle.Render("   "+m.usageLine())
}

// usageLine renders the context-window gauge plus running token/turn
// totals — context (how much of the model's window the last turn's
// final request used), the last completed turn's own token cost and
// duration, and the session's cumulative token cost (this hand process
// only; see sessionUsage).
func (m *Model) usageLine() string {
	turnTok := 0
	if m.lastUsage != nil {
		turnTok = totalTokens(*m.lastUsage)
	}
	parts := []string{
		"ctx " + m.contextSummary(),
		"turn " + formatTokenCount(turnTok) + " tok",
		"session " + formatTokenCount(totalTokens(m.sessionUsage)) + " tok",
	}
	if m.lastTurnDuration > 0 {
		parts = append(parts, "last turn "+formatDuration(m.lastTurnDuration))
	}
	return strings.Join(parts, "  ·  ")
}

// contextSummary renders the most recently reported context occupancy
// as "used/window (pct%)", or just "used" when contextWindow is unknown
// (setModel was never called, e.g. in a test that skips SetBanner).
func (m *Model) contextSummary() string {
	used := contextTokens(m.lastUsage)
	if m.contextWindow <= 0 {
		return formatTokenCount(used)
	}
	pct := float64(used) / float64(m.contextWindow) * 100
	return fmt.Sprintf("%s/%s (%.0f%%)", formatTokenCount(used), formatTokenCount(m.contextWindow), pct)
}

// approvalPanel renders the pending request's preview (colored by line
// prefix) followed by the yes/always/no question.
func (m *Model) approvalPanel() string {
	question := statusAlertStyle.Render(fmt.Sprintf("Allow %s? [y]es / [a]lways / [n]o", m.pending.Tool))
	if m.pending.Preview == "" {
		return question
	}
	return wrapToWidth(renderPreview(m.pending.Preview), m.termWidth) + "\n" + question
}

// renderPreview colors an agentio-built preview line by line: "+ " lines
// (added) in the success color, "- " lines (removed) in the error color,
// "$ " (a bash command) in the user-input color, everything else
// (diff context lines) dim.
func renderPreview(preview string) string {
	lines := strings.Split(preview, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+ "):
			lines[i] = toolOKStyle.Render(line)
		case strings.HasPrefix(line, "- "):
			lines[i] = toolErrStyle.Render(line)
		case strings.HasPrefix(line, "$ "):
			lines[i] = userLineStyle.Render(line)
		default:
			lines[i] = toolCallStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) View() string {
	bottom := wrapToWidth(m.statusLine(), m.termWidth)
	switch {
	case m.pending != nil:
		bottom = m.approvalPanel()
	default:
		if suggestions := m.commandSuggestions(); len(suggestions) > 0 {
			bottom = m.renderSuggestions(suggestions)
		}
	}
	return m.viewport.View() + "\n" + bottom + "\n" + inputBorderStyle.Render(m.textarea.View())
}

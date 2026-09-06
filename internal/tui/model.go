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
	"github.com/sausheong/hand/internal/config"
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

	// markdownStyle is the glamour style name (config.ValidMarkdownStyles)
	// passed to renderMarkdown. Defaults to config.DefaultMarkdownStyle so
	// tests that skip SetMarkdownStyle still render safely; main.go sets
	// it from --markdown-style/config.json via SetMarkdownStyle.
	markdownStyle string

	// skillsIndex is the "## Skills" block the agent's system prompt was
	// built with (agentio.BuildSkillProvider().FormatIndex()), captured
	// once at startup for the /skills command. Not refreshed mid-session —
	// a skill the agent creates via skill_manage won't appear here (or in
	// the system prompt) until the next run, a harness-level limitation
	// (see runtime.SkillProvider's doc comment: FormatIndex is called
	// once at BuildRuntime time).
	skillsIndex string

	// turnStart marks when the in-flight (or, once finished, most
	// recent) turn's Run() began — read live while running for the
	// status line's elapsed-time display.
	turnStart time.Time
	// toolCallsThisTurn counts EventToolCallStart events since turnStart,
	// reset at the start of each new turn. Surfaced in the "working..."
	// status while running: a turn can spend a long stretch making tool
	// calls with no assistant text in between, during which the
	// token/context figures don't move at all (usage is only reported
	// once, at EventDone) — this is real, concrete evidence that
	// something is actually happening, not a frozen/hung UI.
	toolCallsThisTurn int
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
		rt:            rt,
		workspace:     workspace,
		textarea:      ta,
		viewport:      vp,
		spinner:       sp,
		termWidth:     80,
		termHeight:    24,
		markdownStyle: config.DefaultMarkdownStyle,
	}
}

// SetMarkdownStyle sets the glamour style renderMarkdown uses for
// assistant output (see config.ValidMarkdownStyles). Call before the
// first turn if overriding the config.DefaultMarkdownStyle set by
// NewModel — main.go does this from --markdown-style/config.json.
// renderMarkdown falls back to the default itself for anything not on
// the allow-list, so an unvalidated value here is safe, just possibly
// not what the caller intended.
func (m *Model) SetMarkdownStyle(style string) {
	m.markdownStyle = style
}

// SetSkillsIndex sets the text /skills prints — main.go passes
// agentio.BuildSkillProvider's merged provider's FormatIndex() result.
func (m *Model) SetSkillsIndex(index string) {
	m.skillsIndex = index
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

	case tea.MouseMsg:
		// Only the transcript scrolls on mouse wheel — viewport.Update
		// ignores anything but wheel-up/down on its own (MouseWheelEnabled
		// defaults true from viewport.New), so this is safe to forward
		// unconditionally, pending-approval or not.
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		// Scrolling must not fall through to the default case below
		// (deny) — a long diff or bash preview is exactly when a user
		// most wants to scroll back through it before deciding, and
		// Update's tea.MouseMsg case already documents this same intent
		// for the mouse wheel ("safe to forward unconditionally,
		// pending-approval or not"); these are its keyboard equivalent.
		switch msg.String() {
		case "pgup", "ctrl+u":
			m.viewport.HalfPageUp()
			return m, nil
		case "pgdown", "ctrl+d":
			m.viewport.HalfPageDown()
			return m, nil
		}
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

	// Scroll the transcript, independent of the textarea (which owns
	// plain up/down for cursor movement — see the dropdown branch above
	// and the fallthrough to m.textarea.Update below). These keys are
	// never typed as ordinary message text, so they're safe to claim
	// unconditionally. refreshViewport only re-snaps to the bottom when
	// the viewport was already there before new content arrived (see
	// its own comment), so scrolling up here isn't immediately undone
	// by the next streamed event.
	case "pgup":
		m.viewport.HalfPageUp()
		return m, nil
	case "pgdown":
		m.viewport.HalfPageDown()
		return m, nil
	case "ctrl+u":
		m.viewport.HalfPageUp()
		return m, nil
	case "ctrl+d":
		m.viewport.HalfPageDown()
		return m, nil

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
	m.toolCallsThisTurn = 0

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
		m.toolCallsThisTurn++
		name := "?"
		var input json.RawMessage
		if ev.ToolCall != nil {
			// An MCP server's own registered tool name — unlike hand's
			// fixed built-in tool names (bash, read_file, ...), this
			// string comes from whatever the (possibly untrusted) server
			// declared, so it needs the same sanitization untrusted tool
			// output already gets.
			name = sanitizeForTerminal(ev.ToolCall.Name)
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
			// ev.Result.Error can carry untrusted content (bash stderr,
			// a web_fetch failure echoing page content, an MCP server's
			// own error text) just as much as a successful Output does —
			// summarizeToolResult sanitizes that path; this one needs it too.
			m.transcript = append(m.transcript, toolErrStyle.Render("  ✗ "+sanitizeForTerminal(ev.Result.Error)))
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

// flushStream moves the in-progress assistant text block into the
// transcript, rendered as Markdown (see renderMarkdown) now that the
// block is complete. Live-streaming text (shown raw via refreshViewport
// reading m.streamBuf directly) stays plain until it flushes — see
// refreshViewport's comment for why re-rendering the growing buffer
// live was tried and reverted.
func (m *Model) flushStream() {
	if m.streamBuf.Len() == 0 {
		return
	}
	m.transcript = append(m.transcript, renderMarkdown(m.streamBuf.String(), m.termWidth, m.markdownStyle))
	m.streamBuf.Reset()
}

func (m *Model) refreshViewport() {
	// Only auto-scroll to the new bottom if the viewport was already
	// there before this update — otherwise every streamed delta or tool
	// event (refreshViewport runs on each one) would yank the view back
	// down the instant the user scrolls up to reread earlier output.
	// Checked before SetContent changes what "bottom" even means.
	stickToBottom := m.viewport.AtBottom()

	content := strings.Join(m.transcript, "\n")
	if m.streamBuf.Len() > 0 {
		// Deliberately NOT rendered through glamour here. This used to
		// re-render the whole growing buffer as Markdown on every single
		// delta so formatting appeared live instead of only once the
		// block flushed — but Update (and everything else on Bubble
		// Tea's single event loop: spinner ticks, key presses, Ctrl+C)
		// blocks for the full duration of glamour's Render call, and
		// that call's cost grows with buffer size. For a long, dense
		// response (headers, lists, an open code fence) called on every
		// one of what can be hundreds of deltas, the cumulative
		// synchronous render time compounds into many seconds of a
		// completely unresponsive UI — indistinguishable from a genuine
		// hang, and Ctrl+C can't get through it either since it's all on
		// the one goroutine. flushStream still renders the complete,
		// final block through glamour — bounded to a handful of calls
		// per turn on text that has stopped growing, not one call per
		// delta on text that keeps growing.
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
	if stickToBottom {
		m.viewport.GotoBottom()
	}
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
// elapsed time and (while streaming) the token estimate visibly move
// during a turn instead of sitting frozen at the previous turn's
// numbers. Segments are styled individually (statusIdleStyle each,
// contextSummary sometimes statusAlertStyle) and joined with a plain
// separator, rather than wrapping the whole line in one outer Render —
// lipgloss's reset-on-render would otherwise cancel a differently-styled
// inner segment's color for everything after it.
func (m *Model) statusLine() string {
	left := statusIdleStyle.Render("ready")
	if m.running {
		toolInfo := ""
		if m.toolCallsThisTurn > 0 {
			// Concrete, non-estimated evidence of progress — the token
			// figures in usageLine can sit at "0" for a long stretch of
			// real work when a turn is mostly back-to-back tool calls
			// with no assistant text in between (usage is only reported
			// once, at EventDone), which otherwise reads as a hung UI.
			toolInfo = fmt.Sprintf(" · %d tool call", m.toolCallsThisTurn)
			if m.toolCallsThisTurn != 1 {
				toolInfo += "s"
			}
		}
		left = m.spinner.View() + statusIdleStyle.Render(fmt.Sprintf(" working... %s%s", formatDuration(time.Since(m.turnStart)), toolInfo))
	}
	return left + "   " + m.usageLine()
}

// usageLine renders the context-window gauge plus running token/turn
// totals: context (how much of the model's window the last completed
// turn's final request used), this turn's token cost, and the session's
// cumulative token cost (this hand process only; see sessionUsage).
//
// While a turn is running, "turn" shows a live, clearly-labeled estimate
// of the response so far (chars/4, the same rough heuristic
// harness's own tokens.Estimate uses) instead of the previous turn's
// now-stale total — actual usage numbers only arrive once per turn, on
// EventDone, so a live count during streaming can only ever be an
// estimate, not the real thing.
func (m *Model) usageLine() string {
	var turnSeg string
	switch {
	case m.running:
		// .Len(), not len(.String()): usageLine runs on every render
		// (every spinner tick while streaming), and .String() would
		// copy the whole growing buffer just to measure it.
		turnSeg = fmt.Sprintf("turn ~%s tok", formatTokenCount(m.streamBuf.Len()/4))
	case m.lastUsage != nil:
		turnSeg = fmt.Sprintf("turn %s tok", formatTokenCount(totalTokens(*m.lastUsage)))
	default:
		turnSeg = "turn 0 tok"
	}

	sep := statusIdleStyle.Render("  ·  ")
	parts := []string{
		m.contextSummary(),
		statusIdleStyle.Render(turnSeg),
		statusIdleStyle.Render(fmt.Sprintf("session %s tok", formatTokenCount(totalTokens(m.sessionUsage)))),
	}
	if m.lastTurnDuration > 0 {
		parts = append(parts, statusIdleStyle.Render("last turn "+formatDuration(m.lastTurnDuration)))
	}
	return strings.Join(parts, sep)
}

// contextAlertThreshold is the fraction of the context window at which
// contextSummary switches from statusIdleStyle to statusAlertStyle — a
// visible nudge, ahead of harness's own automatic preventive compaction
// (or a manual /compact), that the window is running out.
const contextAlertThreshold = 0.85

// contextSummary renders the most recently reported context occupancy
// as "ctx used/window (pct%)", styled with the alert color once usage
// crosses contextAlertThreshold, or just "ctx used" when contextWindow
// is unknown (setModel was never called, e.g. a test that skips
// SetBanner, or a model tokens.ContextWindowFor has no data for).
func (m *Model) contextSummary() string {
	used := contextTokens(m.lastUsage)
	if m.contextWindow <= 0 {
		return statusIdleStyle.Render("ctx " + formatTokenCount(used))
	}
	frac := float64(used) / float64(m.contextWindow)
	text := fmt.Sprintf("ctx %s/%s (%.0f%%)", formatTokenCount(used), formatTokenCount(m.contextWindow), frac*100)
	if frac >= contextAlertThreshold {
		return statusAlertStyle.Render(text)
	}
	return statusIdleStyle.Render(text)
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

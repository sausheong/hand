package tui

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tokens"
)

// Controller is an internal construction alias; ownership lives in app.
type Controller = app.Controller

// Model is the Hand Bubble Tea program. It uses pointer-receiver
// Init/Update/View methods (rather than the value-receiver style most
// Bubble Tea examples use) so a *tea.Program reference can be injected
// after construction via BindProgram — breaking the construction cycle
// between the Program and the approval hook that needs to Send into it
// (see internal/agentio.Sender and cmd/hand/main.go).
type Model struct {
	permissionTask      *permissionTask
	markdownLayout      *markdownLayout
	layoutEpoch         uint64
	outputLoads         map[*outputLoad]struct{}
	sourceBlocks        map[int]sourceLayout
	toolOutputs         []ToolOutput
	outputView          *outputViewer
	attachmentPolicy    *agentio.AttachmentPolicy
	editorActive        bool
	sessionChanging     bool
	sessionGeneration   uint64
	extensionQuestion   *extensions.PendingQuestion
	extensionGeneration uint64
	extensionDraft      string
	sessionCancel       context.CancelFunc
	sessionDone         chan struct{}
	quitAfterSession    bool
	lastModelInfo       app.ModelInfo
	profileDone         chan struct{}
	profileGeneration   uint64
	profileChanging     bool
	profilePicker       *profilePicker
	profileCancel       context.CancelFunc
	quitAfterProfile    bool
	pendingApprovalID   string
	service             *app.Service
	activeStream        *app.Stream
	goalActive          bool
	lastOutcome         *agentio.RunOutcome

	identity            agentio.RunIdentity
	quitAfterRun        bool
	workspace           string
	program             *tea.Program
	processTask         *processTask
	evidenceIDs         []string
	verificationReviews []app.VerificationProfileView
	summarizerReview    *app.SummarizerView
	priceReview         *app.PriceReview
	recoveryPage        *app.CheckpointRecoveryPage
	restoreReview       *app.CheckpointRestorePreview
	controller          *Controller // optional for presentation-only tests

	viewport transcriptViewport
	textarea textarea.Model
	spinner  spinner.Model

	transcript []string
	streamBuf  strings.Builder

	compacting        bool
	compactGeneration uint64
	compactCancel     context.CancelFunc
	compactDone       chan struct{}
	compactCancelled  bool
	quitAfterCompact  bool
	goalGeneration    uint64
	goalChecking      bool
	running           bool
	pending           *agentio.ApprovalRequest
	cancel            context.CancelFunc

	// lastUsage holds the token counts from the most recent turn's
	// EventDone, nil until the first turn completes with usage reported
	// (a provider may not report usage at all). Surfaced via /usage and
	// the always-on status line's context/turn-token figures.
	lastUsage        *llm.Usage
	lastRequestUsage *llm.Usage

	// sessionUsage accumulates lastUsage's fields across every completed
	// turn since this process started (not persisted — a fresh hand
	// process, even resuming the same saved session, starts back at
	// zero, matching how the rest of this status line resets).
	sessionUsage                llm.Usage
	usageRequests, usageUnknown int
	usagePriorUnknown           bool

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

	// goalIteration is the latest iteration reported by the application.
	goalIteration int
	// goalCancelled suppresses late presentation after user cancellation.
	goalCancelled bool

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
	// wall-clock duration reported when the application turn ends.
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

// newPresentationModel initializes terminal presentation state. Production
// construction must attach an application service before returning the model.
func newPresentationModel(workspace string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Ctrl+G: external editor)"
	ta.ShowLineNumbers = false
	ta.SetWidth(78) // 80 minus the 1-column border on each side (see View/resize)
	ta.SetHeight(3)
	ta.Focus()

	vp := transcriptViewport{Width: 80, Height: 20}
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle))

	return &Model{
		identity:      agentio.RunIdentity{SessionID: "tui-" + rand.Text()},
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
// NewApplicationModel — main.go does this from --markdown-style/config.json.
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
// retained for terminal construction compatibility.
func (m *Model) BindProgram(p *tea.Program) {
	m.program = p
}

// SetBanner prepends the startup banner (version/model/workspace) to the
// transcript. Call once, right after NewApplicationModel and before LoadHistory, so
// the banner sits above any resumed conversation.
func (m *Model) SetBanner(version, model, workspace string) {
	m.setModel(model)
	m.prependSourceBlocks([]TranscriptBlock{
		{Kind: "notice", Text: "Hand " + version, Tone: "userLineStyle"},
		{Kind: "notice", Text: model, Tone: "statusIdleStyle"},
		{Kind: "notice", Text: workspace, Tone: "statusIdleStyle"},
		{Kind: "notice", Text: "", Tone: "statusIdleStyle"},
	})
	m.refreshViewport()
}

// setModel updates the active model name and recomputes its context
// window (tokens.ContextWindowFor), so the status line's "ctx" figure
// tracks whichever model is actually active. Called at startup
// (SetBanner) and after a successful /model switch.
func (m *Model) setModel(model string) {
	m.lastRequestUsage = nil
	m.model = model
	m.contextWindow = tokens.ContextWindowFor(model, 0)
}

// SetController wires up slash commands that need state beyond the
// presentation state (/model, /new, /compact). Without it those commands
// report themselves unavailable; /exit, /help, and /clear work regardless.
func (m *Model) SetController(c *Controller) {
	m.controller = c
	if c != nil && c.SessionID() != "" {
		if m.identity.SessionID != c.SessionID() {
			m.lastRequestUsage = nil
		}
		m.identity.SessionID = c.SessionID()
		if usage, err := c.SessionUsage(); err == nil {
			m.sessionUsage, m.usageRequests, m.usageUnknown = usage.Total, usage.Requests, usage.Unknown
			m.usagePriorUnknown = usage.PriorUsageUnknown
		} else {
			// Totals belong to a session. An unreadable replacement must not
			// retain the previous session's cached accounting.
			m.sessionUsage = llm.Usage{}
			m.usageRequests, m.usageUnknown = 0, 0
			m.usagePriorUnknown = true
			m.appendNotice("session usage unavailable: "+sanitizeForTerminal(err.Error()), "errorLineStyle")
		}
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case permissionCommandDone:
		return m, m.finishPermissionCommand(msg)
	case processCommandDone:
		return m, m.finishProcessCommand(msg)
	case referenceCompletionMsg:
		m.applyReferenceCompletion(msg)
		return m, nil
	case editorResult:
		m.finishExternalEditor(msg)
		return m, nil
	case outputLoaded:
		if msg.load != nil {
			<-msg.load.done
			delete(m.outputLoads, msg.load)
		}
		if m.outputView != msg.viewer {
			return m, nil
		}
		if msg.err != nil {
			m.outputView.status = "Full output unavailable: " + sanitizeForTerminal(msg.err.Error())
			return m, nil
		}
		m.outputView.block = msg.block
		m.resizeOutput()
		m.outputView.status = "Loaded complete session result"
		if msg.block.Artifact != "" {
			m.outputView.status = "Verified " + msg.block.Artifact + " capture"
			if msg.block.Truncated {
				m.outputView.status += " (capture truncated)"
			}
		}
		return m, nil
	case outputCopyResult:
		if m.outputView == msg.viewer {
			if msg.err != nil {
				m.outputView.status = "Copy failed: " + sanitizeForTerminal(msg.err.Error())
			} else {
				m.outputView.status = "Copy request sent; terminal must support OSC52"
			}
		}
		return m, nil
	case markdownLayoutDone:
		m.finishMarkdownLayout(msg.task)
		return m, m.waitMarkdownLayout()
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		m.resizeOutput()
		return m, m.waitMarkdownLayout()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, tea.Batch(cmd, m.waitMarkdownLayout())

	case extensionQuestionTick:
		return m, m.pollExtensionQuestion(msg)
	case sessionChangedMsg:
		if msg.generation == m.sessionGeneration {
			m.dismissExtensionQuestion()
			m.extensionGeneration++
		}
		return m, m.finishSessionChange(msg)
	case profileChangedMsg:
		if !m.profileChanging || msg.generation != m.profileGeneration {
			return m, nil
		}
		m.profileDone = nil
		m.profileChanging = false
		m.profileCancel = nil
		if msg.err != nil {
			m.appendNotice("profile switch failed: "+sanitizeForTerminal(msg.err.Error()), "errorLineStyle")
		} else {
			m.identity.Generation++
			m.lastRequestUsage = nil
			m.setModel(m.controller.CurrentModel())
			m.SetContextLimit(m.controller.ContextLimit())
			m.appendNotice("profile switched to "+sanitizeForTerminal(msg.name)+": "+sanitizeForTerminal(m.model), "approvedStyle")
		}
		m.refreshViewport()
		if m.quitAfterProfile {
			return m, tea.Quit
		}
		return m, nil
	case applicationMsg:
		return m, m.handleApplicationMessage(msg)

	case compactResultMsg:
		if !m.compacting || msg.generation != m.compactGeneration || msg.identity != m.identity {
			return m, nil
		}
		if m.compactDone != nil {
			<-m.compactDone
			m.compactDone = nil
		}
		m.compacting = false
		m.lastRequestUsage = nil
		m.compactCancel = nil
		if msg.usageKnown {
			m.sessionUsage, m.usageRequests, m.usageUnknown = msg.usage.Total, msg.usage.Requests, msg.usage.Unknown
			m.usagePriorUnknown = msg.usage.PriorUsageUnknown
		}
		if msg.usageErr != nil {
			// Cached usage remains a reported subtotal, not a complete total.
			m.usagePriorUnknown = true
			m.appendNotice("compaction usage unavailable: "+sanitizeForTerminal(msg.usageErr.Error()), "errorLineStyle")
		}
		if m.compactCancelled {
			m.appendNotice("compaction cancelled", "toolCallStyle")
		} else {
			m.showCompactResult(msg.result, msg.err)
		}
		m.refreshViewport()
		if m.quitAfterCompact {
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyMsg:
		if m.outputView != nil {
			return m.outputKey(msg)
		}
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.profilePicker != nil {
		return m.handleProfilePickerKey(msg)
	}
	if m.editorActive {
		return m, nil
	}
	if msg.String() == "ctrl+g" {
		return m, m.openExternalEditor()
	}
	if m.sessionChanging && msg.String() == "ctrl+c" {
		m.sessionCancel()
		return m, nil
	}
	if m.extensionQuestion != nil && (m.sessionChanging || m.running || m.compacting) {
		switch msg.String() {
		case "enter":
			m.answerExtension(m.textarea.Value(), false)
			return m, nil
		case "esc":
			m.answerExtension("", true)
			return m, nil
		}
	}
	if m.sessionChanging && msg.String() == "enter" {
		text := strings.TrimSpace(m.textarea.Value())
		if text == "/quit" || text == "/exit" {
			m.textarea.Reset()
			return m, m.handleCommand(text)
		}
		return m, nil
	}
	if m.profileChanging && msg.String() == "ctrl+c" {
		m.profileCancel()
		return m, nil
	}
	if m.profileChanging && msg.String() == "enter" {
		text := strings.TrimSpace(m.textarea.Value())
		if text == "/quit" || text == "/exit" {
			m.textarea.Reset()
			return m, m.handleCommand(text)
		}
		return m, nil
	}
	if m.running && msg.String() == "ctrl+c" {
		m.goalCancelled = true
		m.dismissApproval()
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil
	}

	if m.compacting && msg.String() == "ctrl+c" {
		m.compactCancelled = true
		m.compactCancel()
		return m, nil
	}
	if m.compacting && msg.String() == "enter" {
		// Quit must reach the existing cancel-and-join command path even
		// while ordinary submissions are held during compaction.
		text := strings.TrimSpace(m.textarea.Value())
		if text == "/quit" || text == "/exit" {
			m.textarea.Reset()
			return m, m.handleCommand(text)
		}
		return m, nil
	}

	if m.pending != nil && (msg.String() == "/" || strings.HasPrefix(m.textarea.Value(), "/")) {
		// Slash input must not accidentally answer an approval with y/a keys.
		if msg.String() == "enter" {
			text := strings.TrimSpace(m.textarea.Value())
			if isQueueCommand(text) {
				m.textarea.Reset()
				return m, m.handleCommand(text)
			}
			return m, nil
		}
		if msg.String() == "esc" {
			m.textarea.Reset()
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	if m.pending != nil {
		// Scrolling must not fall through to the default case below
		// (deny) — a long diff or bash preview is exactly when a user
		// most wants to scroll back through it before deciding.
		switch msg.String() {
		case "v":
			m.showApprovalPreview()
			return m, nil
		case "tab":
			return m, nil
		case "pgup", "ctrl+u":
			m.viewport.HalfPageUp()
			return m, nil
		case "pgdown", "ctrl+d":
			m.viewport.HalfPageDown()
			return m, nil
		}
		switch msg.String() {
		case "y", "Y":
			if m.respondPendingApproval(agentio.DecisionOnce) {
				m.recordApproval("approved")
			}
			m.pending = nil
			m.refreshViewport()
		case "a", "A":
			if m.respondPendingApproval(agentio.DecisionAlways) {
				m.recordApproval("always allowed")
			}
			m.pending = nil
			m.refreshViewport()
		default:
			if m.respondPendingApproval(agentio.DecisionDeny) {
				m.recordApproval("denied")
			}
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
		if m.goalChecking {
			m.goalCancelled = true
			m.abandonGoal("context_cancelled")
			return m, nil
		}
		if m.running && m.cancel != nil {
			m.goalCancelled = true
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

	case "tab":
		return m, m.completeReference()
	case "enter":
		if m.running {
			text := strings.TrimSpace(m.textarea.Value())
			if isQueueCommand(text) {
				m.textarea.Reset()
				return m, m.handleCommand(text)
			}
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

// startRun begins one user-submitted agent turn. Resets the goal-loop
// chain (goalIteration/goalCancelled) since this is a fresh,
// human-initiated turn, not an automatic continuation of a previous one.
func (m *Model) startRun(text string) tea.Cmd {
	if m.compacting || m.running || m.profileChanging || m.sessionChanging {
		return nil
	}
	input, err := agentio.ParsePromptInput(agentio.WithAttachmentPolicy(context.Background(), m.attachmentPolicy), m.workspace, text)
	if err != nil {
		m.queueMessage("Attachment error: " + err.Error() + "; input preserved.")
		return nil
	}
	m.abandonGoal("superseded_by_input")
	m.goalIteration = 1
	m.goalCancelled = false
	// The original text (with the real path, not the placeholder) is
	// sent to the model alongside images — the model may benefit from
	// the literal filename/path context next to the image bytes.
	return m.runTurn(userLineStyle.Render("> "+input.Display), input.Prompt, input.Images)
}

// runTurn starts a user request through the application service.
func (m *Model) runTurn(displayLine, prompt string, images []llm.ImageContent) tea.Cmd {
	if m.running || m.compacting {
		return nil
	}
	m.identity.RunID++
	m.identity.Generation++
	m.quitAfterRun = false
	m.goalActive = true
	m.lastOutcome = nil

	if text := sanitizeForTerminal(displayLine); strings.HasPrefix(text, "> ") {
		m.appendSourceBlock(TranscriptBlock{Kind: "user", Text: strings.TrimPrefix(text, "> ")})
	} else {
		m.appendSourceBlock(TranscriptBlock{Kind: "status", Text: text})
	}
	m.textarea.Reset()
	m.running = true
	m.turnStart = time.Now()
	m.toolCallsThisTurn = 0
	return m.startApplicationGoal(prompt, images)
}

// cancelGoalCheck invalidates even an already queued result. Only Update's
// goroutine changes this state; commands capture their generation and context.
func (m *Model) cancelGoalCheck() {
	m.goalGeneration++
	m.goalChecking = false
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
	m.appendSourceBlock(TranscriptBlock{Kind: "assistant", Text: m.streamBuf.String()})
	m.streamBuf.Reset()
}

func (m *Model) refreshViewport() {
	m.refreshSourceBlocks()
	// Only auto-scroll to the new bottom if the viewport was already
	// there before this update — otherwise every streamed delta or tool
	// event (refreshViewport runs on each one) would yank the view back
	// down the instant the user scrolls up to reread earlier output.
	// Checked before SetContent changes what "bottom" even means.
	stickToBottom := m.viewport.AtBottom()

	// Cached layouts retain completed history while only the growing stream
	// block is rewrapped. The viewport renders visible rows on demand.
	m.viewport.Width = m.termWidth
	m.viewport.SetBlocks(m.transcript, m.streamBuf.String())

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
		return strings.Count(m.approvalPanel(), "\n") + 1
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
	if m.compacting {
		label := " compacting..."
		if m.compactCancelled {
			label = " cancelling compaction..."
		}
		left = m.spinner.View() + statusIdleStyle.Render(label)
	}
	if m.goalChecking {
		left = m.spinner.View() + statusIdleStyle.Render(" checking goal...")
	}
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

// usageLine renders the active "provider/model" string, the
// context-window gauge, and running token/turn totals: context (how
// much of the model's window the last completed turn's final request
// used), this turn's token cost, and the session's cumulative token
// cost (this hand process only; see sessionUsage). The model leads
// because it's the one thing /model can change mid-session — after a
// switch, the startup banner scrolls out of view but this line stays
// put, so it's the reliable place to check which model is actually
// live.
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

	sessionSeg := fmt.Sprintf("session %s tok", formatTokenCount(totalTokens(m.sessionUsage)))
	if m.usagePriorUnknown || m.usageUnknown > 0 {
		sessionSeg = fmt.Sprintf("session ≥%s tok (incomplete)", formatTokenCount(totalTokens(m.sessionUsage)))
	}
	sep := statusIdleStyle.Render("  ·  ")
	parts := []string{
		statusIdleStyle.Render(m.model),
		m.contextSummary(),
		statusIdleStyle.Render(turnSeg),
		statusIdleStyle.Render(sessionSeg),
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
	if m.lastRequestUsage == nil {
		return statusIdleStyle.Render("ctx unknown")
	}
	used := contextTokens(m.lastRequestUsage)
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
	width := max(1, m.termWidth)
	toolName := strings.ReplaceAll(sanitizeForTerminal(m.pending.Tool), "\n", " ")
	toolName = ansi.Truncate(toolName, max(1, width-44), "…")
	question := fmt.Sprintf("Allow %s? [y]es / [a]lways / [n]o [v]iew", toolName)
	question = strings.Split(wrapToWidth(question, width), "\n")[0]
	budget := max(2, min(8, m.termHeight/3))
	preview := sanitizeForTerminal(m.pending.Preview)
	lines := strings.Split(wrapToWidth(preview, width), "\n")
	if preview == "" {
		return statusAlertStyle.Render(question)
	}
	if len(lines) > budget-1 {
		lines = append(lines[:max(0, budget-2)], "… [v] opens full preview")
	}
	return renderPreview(strings.Join(lines, "\n")) + "\n" + statusAlertStyle.Render(question)
}

// renderPreview colors an agentio-built preview line by line: "+ " lines
// (added) in the success color, "- " lines (removed) in the error color,
// "$ " (a bash command) in the user-input color, everything else
// (diff context lines) dim.
func renderPreview(preview string) string {
	lines := strings.Split(sanitizeForTerminal(preview), "\n")
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
	if m.profilePicker != nil {
		return m.profilePickerView()
	}
	if m.outputView != nil {
		return m.outputView.view()
	}
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

// dismissApproval never blocks Update, including when cancellation races a
// decision already queued for the hook. The hook also rechecks its context.
func (m *Model) respondPendingApproval(decision agentio.Decision) bool {
	if m.pending == nil || m.pendingApprovalID == "" || m.service == nil {
		return false
	}
	if err := m.service.RespondApproval(m.pending.Identity, m.pendingApprovalID, decision); err != nil {
		if !m.goalCancelled {
			m.appendNotice("approval expired: "+sanitizeForTerminal(err.Error()), "errorLineStyle")
		}
		return false
	}
	m.pendingApprovalID = ""
	return true
}

func (m *Model) dismissApproval() {
	if m.outputView != nil && m.outputView.approval {
		m.closeOutputView()
	}
	if m.pending != nil {
		m.respondPendingApproval(agentio.DecisionDeny)
		m.pendingApprovalID = ""
		m.pending = nil
	}
}

// finishGoal publishes one result for a user request, including all automatic
// continuations. Duplicate closure/check messages cannot publish it again.
func (m *Model) finishGoal(outcome agentio.RunOutcome) {
	m.dismissExtensionQuestion()
	m.extensionGeneration++
	if !m.goalActive {
		return
	}
	m.goalActive = false
	outcome.Iterations = m.goalIteration
	m.lastOutcome = &outcome
	label := map[agentio.RunStatus]string{agentio.Completed: "Completed", agentio.VerificationFailed: "Verification failed", agentio.BudgetExhausted: "Limit reached", agentio.Cancelled: "Cancelled", agentio.InfrastructureError: "Execution failed"}[outcome.Status]
	detail := outcome.Reason
	if outcome.Status == agentio.Completed {
		detail = "no mandatory verification"
		if outcome.Verified {
			detail = "configured checks passed"
		}
	}
	line := fmt.Sprintf("[result] %s: %s", label, detail)
	if outcome.Cause != nil {
		line += ": " + outcome.Cause.Error()
	}
	m.appendSourceBlock(TranscriptBlock{Kind: "status", Text: line})
	m.refreshViewport()
}

func (m *Model) abandonGoal(reason string) {
	m.cancelGoalCheck()
	m.finishGoal(agentio.RunOutcome{Status: agentio.Cancelled, Reason: reason})
}

// SetAttachmentPolicy installs invocation grants for explicit user input only.
func (m *Model) SetAttachmentPolicy(policy *agentio.AttachmentPolicy) { m.attachmentPolicy = policy }

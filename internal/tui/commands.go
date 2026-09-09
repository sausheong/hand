package tui

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/compaction"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// commandDef backs both /help's listing and the auto-complete dropdown
// shown while typing a command name, so the two can't drift apart.
type commandDef struct {
	name string
	desc string
}

var commandDefs = []commandDef{
	{"/run-budget", "inspect [ID], select ID, tokens ID TOTAL, cost ID CURRENCY TOTAL strict|advisory, time ID SECONDS"},
	{"/time-budget", "inspect or renew session wall-clock allowance: [SECONDS]"},
	{"/prices-review", "review replacement route tariffs from JSON: PATH"},
	{"/prices-confirm", "install the reviewed tariff table: DIGEST"},
	{"/cost", "inspect or set monetary ceiling: [CURRENCY TOTAL strict|advisory]"},
	{"/prices", "inspect installed route tariffs and provenance"},
	{"/budget", "inspect session token ceiling and committed usage"},
	{"/budget-tokens", "set or resume absolute session token ceiling: TOTAL"},
	{"/summarizer-follow", "review returning summarisation to the main model"},
	{"/summarizer", "review summariser: PROFILE [OUTPUT_TOKENS TIMEOUT_SECONDS]"},
	{"/summarizer-confirm", "select reviewed summariser: DIGEST"},
	{"/state", "inspect or update structured objectives, decisions, pending work and evidence references"},
	{"/pins", "list session objectives and constraints"},
	{"/pin", "set session pin: ID objective|constraint TEXT"},
	{"/unpin", "remove a session pin: ID"},
	{"/context", "inspect estimated context contributions while idle"},
	{"/reload", "refresh the skill index while idle"},
	{"/extension", "run a reviewed extension: NAME COMMAND [arguments]"},
	{"/verify-list", "list saved verification IDs: [offset]"},
	{"/verify-delete", "delete listed evidence record: ID confirm"},
	{"/verify", "review named verification commands: [profile]"},
	{"/verify-confirm", "run reviewed verification: PROFILE DIGEST"},
	{"/verify-check", "reassess saved verification: PROFILE EVIDENCE-ID"},
	{"/boundary", "show effective execution isolation"},
	{"/recoveries", "inspect restore recovery: [offset]"},
	{"/recovery-resolve", "confirm recovery resolution: acknowledge|cancel RECOVERY-ID"},
	{"/restore-confirm", "apply displayed restore preview: CURRENT-DIGEST"},
	{"/restore-cancel", "discard pending restore preview"},
	{"/restore-preview", "preview one checkpoint restore: RUN-ID PATH"},
	{"/changes", "inspect checkpoint file changes: [run-ID] [offset]"},
	{"/permissions", "inspect scoped grants: [offset]; revoke <ID>; legacy; acknowledge <fingerprint>"},
	{"/mcp", "show optional connections; /mcp retry <server>"},
	{"/process", "background shell: start <command>, list, read/send/cancel/wait/forget <ID>"},
	{"/output", "view output: /output [result number] [stdout|stderr]"},
	{"/steer", "queue a correction at the next safe tool boundary"},
	{"/followup", "queue a message after the active goal settles"},
	{"/queue", "list queued input; /queue edit <ID> <text>, remove <ID>, or run"},
	{"/help", "show this message"},
	{"/model", "show the active model, or /model <name> to switch"},
	{"/profile", "choose a profile, or /profile <name> to switch"},
	{"/new", "create a new session while keeping previous history"},
	{"/resume", "list sessions, or /resume <session ID>"},
	{"/name", "name the current session: /name <text>"},
	{"/export", "export full session JSONL to a new path"},
	{"/fork", "copy the selected history into a new session"},
	{"/tree", "show session branches, or /tree <entry ID> to select"},
	{"/clear", "clear the on-screen transcript (keeps the saved session)"},
	{"/compact", "compact context with optional focus instructions"},
	{"/usage", "show token usage: this turn, session total, and context window"},
	{"/skills", "list available skills (personal + project)"},
	{"/exit", "quit hand"},
}

func buildHelpText() string {
	var b strings.Builder
	b.WriteString("Ctrl+G: edit current input in $VISUAL or $EDITOR (default vi).\nCommands:\n")
	for i, cd := range commandDefs {
		fmt.Fprintf(&b, "  %-10s %s", cd.name, cd.desc)
		if i < len(commandDefs)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

var helpText = buildHelpText()

// isCommand reports whether text should be dispatched as a slash command
// rather than sent to the agent as a user message.
func isCommand(text string) bool {
	return strings.HasPrefix(text, "/")
}

// matchingCommands returns the known commands whose name starts with
// prefix, for the auto-complete dropdown. It only matches while the user
// is still typing the command word itself — once whitespace appears
// (moving into arguments, e.g. "/model anthropic/..."), there's nothing
// left to complete.
func matchingCommands(prefix string) []commandDef {
	if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, " \t") {
		return nil
	}
	var out []commandDef
	for _, cd := range commandDefs {
		if strings.HasPrefix(cd.name, prefix) {
			out = append(out, cd)
		}
	}
	return out
}

// commandSuggestions returns the auto-complete matches for the text
// currently in the input box, or nil when there's nothing to suggest
// (empty input, not a command, mid-argument, or a turn/approval is
// already in progress and the input isn't being used for a command).
func (m *Model) commandSuggestions() []commandDef {
	if m.pending != nil || m.running {
		return nil
	}
	return matchingCommands(m.textarea.Value())
}

// renderSuggestions draws the auto-complete dropdown, highlighting the
// currently selected entry (see suggestIndex, moved by the up/down keys
// in handleKey).
func (m *Model) renderSuggestions(suggestions []commandDef) string {
	if m.suggestIndex < 0 || m.suggestIndex >= len(suggestions) {
		m.suggestIndex = 0
	}
	lines := make([]string, len(suggestions))
	for i, cd := range suggestions {
		line := fmt.Sprintf("%-10s %s", cd.name, cd.desc)
		if i == m.suggestIndex {
			lines[i] = userLineStyle.Render("› " + line)
		} else {
			lines[i] = toolCallStyle.Render("  " + line)
		}
	}
	return strings.Join(lines, "\n")
}

// completeSuggestion fills the input with the selected suggestion's full
// command name (Tab), leaving the cursor ready for arguments.
func (m *Model) completeSuggestion(suggestions []commandDef) {
	if m.suggestIndex < 0 || m.suggestIndex >= len(suggestions) {
		m.suggestIndex = 0
	}
	m.textarea.SetValue(suggestions[m.suggestIndex].name + " ")
	m.textarea.CursorEnd()
	m.suggestIndex = 0
}

// handleCommand parses and runs a slash command, appending its result to
// the transcript. It never starts an agent turn.
func (m *Model) handleCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	name, args := fields[0], fields[1:]
	if m.sessionChanging {
		if name == "/exit" || name == "/quit" {
			m.quitAfterSession = true
			m.sessionCancel()
		}
		return nil
	}
	if m.profileChanging {
		if name == "/exit" || name == "/quit" {
			m.quitAfterProfile = true
			m.profileCancel()
		}
		return nil
	}

	if m.running && (name == "/state" || name == "/run-budget" || name == "/time-budget" || name == "/prices-review" || name == "/prices-confirm" || name == "/cost" || name == "/prices" || name == "/budget" || name == "/budget-tokens" || name == "/summarizer-follow" || name == "/summarizer" || name == "/summarizer-confirm" || name == "/pins" || name == "/pin" || name == "/unpin" || name == "/context" || name == "/reload" || name == "/verify-list" || name == "/verify-delete" || name == "/verify" || name == "/verify-confirm" || name == "/verify-check" || name == "/recoveries" || name == "/recovery-resolve" || name == "/restore-confirm" || name == "/restore-preview" || name == "/changes" || name == "/new" || name == "/resume" || name == "/name" || name == "/tree" || name == "/fork" || name == "/export" || name == "/model" || name == "/profile" || name == "/compact" || name == "/exit" || name == "/quit") {
		if name == "/exit" || name == "/quit" {
			m.quitAfterRun = true
			m.goalCancelled = true
			m.dismissApproval()
			if m.cancel != nil {
				m.cancel()
			}
		} else {
			m.appendNotice("run still active; Ctrl+C cancels", "toolCallStyle")
			m.refreshViewport()
		}
		return nil
	}
	if m.compacting {
		if name == "/exit" || name == "/quit" {
			m.quitAfterCompact = true
			m.compactCancelled = true
			m.compactCancel()
		} else {
			m.appendNotice("compaction in progress; Ctrl+C cancels", "toolCallStyle")
			m.refreshViewport()
		}
		return nil
	}
	switch name {
	case "/extension":
		return m.runExtension(args)
	case "/boundary":
		boundary := "unrestricted host"
		if m.controller != nil && m.controller.ExecutionBoundary != "" {
			boundary = m.controller.ExecutionBoundary
		}
		m.appendNotice(sanitizeForTerminal(boundary), "toolCallStyle")
		m.refreshViewport()
		return nil
	case "/run-budget":
		return m.runRunBudget(args)
	case "/time-budget":
		return m.runTimeBudget(args)
	case "/prices-review":
		return m.runPriceReview(args)
	case "/prices-confirm":
		return m.runPriceConfirm(args)
	case "/cost", "/prices":
		return m.runCostBudget(name, args)
	case "/budget", "/budget-tokens":
		return m.runTokenBudget(name, args)
	case "/summarizer-follow":
		if len(args) != 0 {
			m.appendNotice("Usage: /summarizer-follow", "errorLineStyle")
			m.refreshViewport()
			return nil
		}
		return m.runSummarizer(nil, true)
	case "/summarizer":
		return m.runSummarizer(args)
	case "/summarizer-confirm":
		return m.runSummarizerConfirm(args)
	case "/state":
		return m.runContextState(args)
	case "/pins", "/pin", "/unpin":
		return m.runPins(name, args)
	case "/context":
		return m.runContext(args)
	case "/reload":
		return m.runReload(args)
	case "/verify-list":
		return m.runVerificationList(args)
	case "/verify-delete":
		return m.runVerificationDelete(args)
	case "/verify":
		return m.runVerificationReview(args)
	case "/verify-confirm":
		return m.runVerificationConfirm(args)
	case "/verify-check":
		return m.runVerificationCheck(args)
	case "/recoveries":
		return m.runRecoveries(args)
	case "/recovery-resolve":
		return m.runRecoveryResolve(args)
	case "/restore-confirm":
		return m.runRestoreConfirm(strings.TrimSpace(text[len(name):]))
	case "/restore-cancel":
		m.restoreReview = nil
		m.appendNotice("Restore preview discarded.", "toolCallStyle")
		m.refreshViewport()
		return nil
	case "/restore-preview":
		return m.runRestorePreview(strings.TrimSpace(text[len(name):]))
	case "/changes":
		return m.runChangesCommand(args)
	case "/permissions":
		return m.runPermissionCommand(args)
	case "/mcp":
		return m.runMCPCommand(args)
	case "/process":
		return m.runProcessCommand(strings.TrimSpace(text[len(name):]))
	case "/output":
		return m.showOutput(strings.Join(args, " "))
	case "/steer", "/followup", "/queue":
		return m.runQueueCommand(name, strings.TrimSpace(text[len(name):]))
	case "/export":
		return m.runExportCommand(strings.TrimSpace(text[len(name):]))
	case "/fork":
		if m.controller == nil || len(args) != 0 {
			m.appendNotice("usage: /fork (requires workspace sessions)", "errorLineStyle")
			m.refreshViewport()
			return nil
		}
		return m.startSessionChange("fork", m.controller.ForkSession)
	case "/tree":
		return m.runTreeCommand(args)
	case "/resume":
		return m.runResumeCommand(args)
	case "/name":
		return m.runNameCommand(strings.TrimSpace(text[len(name):]))
	case "/profile":
		cmd := m.runProfileCommand(args)
		m.refreshViewport()
		return cmd
	case "/exit", "/quit":
		m.abandonGoal("superseded_by_command")
		return tea.Quit

	case "/help":
		m.appendNotice(helpText, "toolCallStyle")

	case "/clear":
		m.clearTranscript()
		m.toolOutputs = nil
		m.closeOutputView()
		m.streamBuf.Reset()

	case "/model":
		m.runModelCommand(args)

	case "/new":
		return m.runNewCommand()

	case "/compact":
		return m.runCompactCommand(strings.TrimSpace(text[len(name):]))

	case "/usage":
		m.runUsageCommand()

	case "/skills":
		m.runSkillsCommand()

	default:
		m.appendNotice("unknown command: "+name+" (try /help)", "errorLineStyle")
	}

	m.refreshViewport()
	return nil
}

func (m *Model) runModelCommand(args []string) {
	if m.controller == nil {
		m.appendNotice("/model is not available in this build", "errorLineStyle")
		return
	}
	if len(args) == 0 {
		m.appendNotice("model: "+m.controller.CurrentModel(), "toolCallStyle")
		return
	}
	m.abandonGoal("superseded_by_command")
	target := args[0]
	oldProvider, _, _ := strings.Cut(m.controller.CurrentModel(), "/")
	if err := m.controller.SwitchModel(target); err != nil {
		m.appendNotice("model switch failed: "+err.Error(), "errorLineStyle")
		return
	}
	m.identity.Generation++
	m.setModel(m.controller.CurrentModel())
	m.SetContextLimit(m.controller.ContextLimit())
	newProvider, _, _ := strings.Cut(m.model, "/")
	msg := "model switched to " + m.model
	if newProvider != oldProvider {
		// The first segment of a /model argument always selects hand's own
		// provider, even when it happens to match the vendor name inside
		// an aggregator's own catalog id (e.g. openrouter lists models as
		// "anthropic/claude-...", "google/gemini-..."). Typing that vendor
		// name without re-prefixing the aggregator ("openrouter/anthropic/
		// claude-...") silently leaves the aggregator entirely and hits
		// that vendor's real API instead — worth calling out immediately,
		// since the failure otherwise only surfaces later as a confusing
		// API error from a provider the user didn't think they switched to.
		msg += fmt.Sprintf(" — now using the %q provider directly (was %q)", newProvider, oldProvider)
	}
	m.appendNotice(msg, "approvedStyle")
}

func (m *Model) runNewCommand() tea.Cmd {
	if m.controller == nil {
		m.appendNotice("/new is not available in this build", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	return m.startSessionChange("new", m.controller.NewSessionContext)
}

func (m *Model) runCompactCommand(focusArgs ...string) tea.Cmd {
	focus := ""
	if len(focusArgs) > 0 {
		focus = focusArgs[0]
	}
	if m.controller == nil {
		m.appendNotice("/compact is not available in this build", "errorLineStyle")
		m.refreshViewport()
		return nil
	}
	if m.running || m.compacting {
		return nil
	}
	m.abandonGoal("superseded_by_command")
	m.compactGeneration++
	generation := m.compactGeneration
	identity := m.identity
	ctx, cancel := context.WithCancel(agentio.WithRunIdentity(context.Background(), identity))
	m.compactCancel = cancel
	m.compactCancelled = false
	m.quitAfterCompact = false
	m.compacting = true
	// Capture dependencies before leaving Update. While this worker is active,
	// commands and new turns cannot mutate the runtime/session it operates on.
	controller := m.controller
	m.refreshViewport()
	// Register the worker now, but preserve Bubble Tea's command dispatch
	// boundary. Shutdown can cancel/join even if the command is never invoked.
	dispatch := make(chan struct{})
	done := make(chan struct{})
	m.compactDone = done
	results := make(chan compactResultMsg, 1)
	go func() {
		defer close(done)
		defer cancel()
		select {
		case <-dispatch:
		case <-ctx.Done():
		}
		if err := ctx.Err(); err != nil {
			results <- compactResultMsg{identity: identity, generation: generation, err: err}
			return
		}
		result, err := controller.CompactWithFocus(ctx, focus)
		usage, usageErr := controller.SessionUsage()
		results <- compactResultMsg{identity: identity, generation: generation, result: result, err: err, usage: usage, usageKnown: usageErr == nil, usageErr: usageErr}
	}()
	var start sync.Once
	return func() tea.Msg { start.Do(func() { close(dispatch) }); return <-results }

}

func (m *Model) showCompactResult(result compaction.Result, err error) {
	if err != nil {
		m.appendSourceBlock(TranscriptBlock{Kind: "compaction", State: "failed", Text: err.Error()})
		return
	}
	if !result.Compacted {
		reason := result.Skipped
		if reason == "" {
			reason = "unknown"
		}
		m.appendSourceBlock(TranscriptBlock{Kind: "compaction", State: "skipped", Text: reason})
		return
	}
	m.appendSourceBlock(TranscriptBlock{Kind: "compaction", State: "completed", Count: result.TurnsCompacted, Text: result.Summary})
}

func (m *Model) runUsageCommand() {
	if m.usagePriorUnknown {
		m.appendNotice("Earlier session usage is unknown. Reported totals cover recorded attempts only.", "toolCallStyle")
	}
	if m.usageRequests > 0 {
		m.appendNotice(fmt.Sprintf("saved session usage: %s reported tokens across %d attempts; %d attempts have unknown usage", formatTokenCount(totalTokens(m.sessionUsage)), m.usageRequests, m.usageUnknown), "toolCallStyle")
	}
	if info := m.lastModelInfo; info.RequestedModel != "" {
		serving := "unknown"
		if info.ServingModelKnown {
			serving = info.ServingModel
		}
		text := fmt.Sprintf("last run requested: %s (profile: %s)\nserving model: %s\nlast run context: %d; source: %s", info.RequestedModel, info.Profile, serving, info.ContextLimit, info.ContextSource)
		if info.Truncated {
			text += " [metadata display truncated]"
		}
		m.appendNotice(sanitizeForTerminal(text), "toolCallStyle")
	}
	if m.lastUsage == nil {
		m.appendNotice("no usage recorded yet", "toolCallStyle")
		return
	}
	u := m.lastUsage
	lines := []string{
		fmt.Sprintf("input: %d  output: %d  cache write: %d  cache read: %d",
			u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens),
		fmt.Sprintf("last turn: %s tok in %s", formatTokenCount(totalTokens(*u)), formatDuration(m.lastTurnDuration)),
		fmt.Sprintf("reported session total: %s tok", formatTokenCount(totalTokens(m.sessionUsage))),
		m.contextSummary(),
	}
	m.appendNotice(strings.Join(lines, "\n"), "toolCallStyle")
}

func (m *Model) runSkillsCommand() {
	if m.skillsIndex == "" {
		m.appendNotice("no skills found in ~/.hand/skills or this workspace's .hand/skills", "toolCallStyle")
		return
	}
	m.appendNotice(strings.TrimRight(m.skillsIndex, "\n"), "toolCallStyle")
}

type profileChangedMsg struct {
	generation uint64
	name       string
	err        error
}

func (m *Model) runProfileCommand(args []string) tea.Cmd {
	if m.controller == nil {
		m.appendNotice("profile switching unavailable", "errorLineStyle")
		return nil
	}
	if len(args) == 0 {
		m.openProfilePicker()
		return nil
	}
	if len(args) != 1 {
		m.appendNotice("usage: /profile <name>", "errorLineStyle")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.profileCancel = cancel
	m.profileChanging = true
	m.quitAfterProfile = false

	name := args[0]
	m.profileGeneration++
	generation := m.profileGeneration
	done := make(chan struct{})
	m.profileDone = done
	result := make(chan profileChangedMsg, 1)
	controller := m.controller
	go func() {
		defer close(done)
		defer cancel()
		result <- profileChangedMsg{generation: generation, name: name, err: controller.SwitchProfileContext(ctx, name)}
	}()
	return func() tea.Msg { return <-result }
}

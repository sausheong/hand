package tui

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestModel_StreamsAssistantTextIntoTranscript(t *testing.T) {
	events := make(chan app.BackendEvent, 4)
	m := applicationModel(t, app.New(applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) { return events, nil }}, app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("hello there")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- app.BackendEvent{Text: "Hi"}
	events <- app.BackendEvent{Text: " back"}
	events <- app.BackendEvent{Done: true}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Hi back") && contains(bts, "ready")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ToolCallAndResultRender(t *testing.T) {
	events := make(chan app.BackendEvent, 4)
	m := applicationModel(t, app.New(applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) { return events, nil }}, app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run a build")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- app.BackendEvent{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "read_file"}}
	events <- app.BackendEvent{Kind: "tool_result", Details: app.Details{ResultPresent: true, Output: "ok"}}
	events <- app.BackendEvent{Done: true}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "[tool: read_file]") && contains(bts, "✓")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ToolCallShowsInputAndResultDetail(t *testing.T) {
	events := make(chan app.BackendEvent, 4)
	m := applicationModel(t, app.New(applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) { return events, nil }}, app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("what does model.go do")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- app.BackendEvent{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "read_file", ToolInput: `{"path":"internal/tui/model.go"}`}}
	events <- app.BackendEvent{Kind: "tool_result", Details: app.Details{ResultPresent: true, Output: "package tui\n"}}
	events <- app.BackendEvent{Done: true}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "[tool: read_file] internal/tui/model.go") && contains(bts, "package tui")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

type imageCaptureBackend struct {
	calls  int
	images []llm.ImageContent
	prompt string
}

func (b *imageCaptureBackend) Run(_ context.Context, prompt string, images []llm.ImageContent) (<-chan app.BackendEvent, error) {
	b.calls++
	b.prompt, b.images = prompt, images
	events := make(chan app.BackendEvent, 1)
	events <- app.BackendEvent{Done: true}
	close(events)
	return events, nil
}
func (*imageCaptureBackend) StopReason() string { return "" }

// The two image-extraction tests below call startRun directly rather
// than driving a real tea.Program: a temp-dir path (macOS's default
// TMPDIR is a long /var/folders/... prefix) routinely exceeds the
// 80-column test terminal width, so asserting on the *rendered* output
// would be asserting on however lipgloss happens to wrap a long
// unbroken token — asserting on m.transcript directly (and on what was
// actually passed to Run) tests the real behavior without that
// incidental fragility.

func TestModel_ImagePathInWorkspaceAttachedAndPlaceholderShown(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(imgPath, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := &imageCaptureBackend{}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1, InputTypes: []string{"text", "image"}}), dir)
	driveApplication(t, m, m.startRun("look at "+imgPath))
	images := backend.images
	if len(images) != 1 {
		t.Fatalf("Run was called with %d images, want 1", len(images))
	}
	if string(images[0].Data) != "fake png bytes" || backend.prompt != "look at "+imgPath || backend.calls != 1 {
		t.Fatal("attachment or backend prompt changed", backend.prompt)
	}
	if images[0].MimeType != "image/png" {
		t.Fatalf("MimeType = %q, want image/png", images[0].MimeType)
	}

	transcript := strings.Join(m.transcript, "\n")
	if !strings.Contains(transcript, "[image: shot.png]") {
		t.Fatalf("transcript = %q, want it to contain the image placeholder", transcript)
	}
	if strings.Contains(transcript, imgPath) {
		t.Fatalf("transcript = %q, want the raw path replaced", transcript)
	}
}

func TestModel_ImagePathOutsideWorkspaceNotAttached(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	imgPath := filepath.Join(outside, "shot.png")
	if err := os.WriteFile(imgPath, []byte("fake png bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := &imageCaptureBackend{}
	m := applicationModel(t, app.New(backend, app.Options{MaxIterations: 1, InputTypes: []string{"text", "image"}}), workspace)
	driveApplication(t, m, m.startRun("look at "+imgPath))
	if images := backend.images; len(images) != 0 {
		t.Fatalf("Run was called with %d images, want nil (path outside workspace)", len(images))
	}

	if backend.calls != 0 || backend.prompt != "" || m.running {
		t.Fatal("outside attachment reached backend", backend.calls, backend.prompt)
	}
	transcript := strings.Join(m.transcript, "\n")
	if !strings.Contains(transcript, imgPath) {
		t.Fatalf("transcript = %q, want the raw path left unchanged", transcript)
	}
}

// The usage tests below call renderApplicationEvent/handleCommand directly
// rather than driving a real tea.Program, for the same reason the
// slash-command dropdown tests in commands_test.go do: this is pure
// state-handling logic with no goroutines involved, so a direct
// synchronous call is both correct and simpler than teatest's async run
// loop.

// Regression: an MCP server's own tool name (unlike hand's fixed
// built-in tool names) is untrusted input, same as tool output — it
// must go through sanitizeForTerminal before rendering, not just the
// summarized detail field next to it.
func TestHandleAgentEvent_SanitizesToolNameOnToolCallStart(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	malicious := "mcp__evil__\x1b]0;pwned\x07tool"

	m.renderApplicationEvent(app.Event{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: malicious}})

	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %v, want exactly one entry", m.transcript)
	}
	if strings.ContainsRune(m.transcript[0], '\x1b') || strings.ContainsRune(m.transcript[0], '\x07') {
		t.Fatalf("transcript[0] = %q, still contains raw ANSI/control bytes from the tool name", m.transcript[0])
	}
}

// Regression: a tool's Error text (bash stderr, a web_fetch failure
// echoing page content, an MCP server's own error string) is untrusted
// external content exactly like a successful Output is — summarizeToolResult
// sanitizes Output, but the Error branch was rendering raw.
func TestHandleAgentEvent_SanitizesToolResultError(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	malicious := "command failed\x1b]52;c;ZXZpbA==\x07"

	m.renderApplicationEvent(app.Event{Kind: "tool_result", Details: app.Details{ResultPresent: true, ToolError: malicious}})

	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %v, want exactly one entry", m.transcript)
	}
	if strings.ContainsRune(m.transcript[0], '\x1b') || strings.ContainsRune(m.transcript[0], '\x07') {
		t.Fatalf("transcript[0] = %q, still contains raw ANSI/control bytes from the tool error", m.transcript[0])
	}
	if !strings.Contains(m.transcript[0], "command failed") {
		t.Fatalf("transcript[0] = %q, lost the legitimate error text", m.transcript[0])
	}
}

func TestHandleAgentEvent_EventDoneCapturesUsage(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	usage := &llm.Usage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 5, CacheReadInputTokens: 3}

	m.renderApplicationEvent(app.Event{Kind: "usage", Details: app.Details{UsageKnown: true, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CacheCreationInputTokens: usage.CacheCreationInputTokens, CacheReadInputTokens: usage.CacheReadInputTokens}})

	if m.lastUsage == nil || *m.lastUsage != *usage {
		t.Fatalf("lastUsage = %v, want %v", m.lastUsage, usage)
	}
}

func TestHandleAgentEvent_EventDoneWithNoUsageStaysNil(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())

	m.renderApplicationEvent(app.Event{Kind: "usage"})

	if m.lastUsage != nil {
		t.Fatalf("lastUsage = %v, want nil", m.lastUsage)
	}
}

func TestRunUsageCommand_NoUsageYet(t *testing.T) {
	m := NewModel(nil, t.TempDir())

	m.handleCommand("/usage")

	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "no usage recorded yet") {
		t.Fatalf("expected \"no usage recorded yet\" in transcript, got %v", m.transcript)
	}
}

func TestRunUsageCommand_WithUsage(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.lastUsage = &llm.Usage{InputTokens: 100, OutputTokens: 20, CacheCreationInputTokens: 5, CacheReadInputTokens: 3}

	m.handleCommand("/usage")

	want := "input: 100  output: 20  cache write: 5  cache read: 3"
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], want) {
		t.Fatalf("expected %q in transcript, got %v", want, m.transcript)
	}
}

// Regression: the status line must always show a context/token gauge
// (not just when running, and not just via /usage) — driven directly
// via renderApplicationEvent, same rationale as the tests above.

func TestStatusLine_ShowsContextGaugeWhenIdleWithNoTurnsYet(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.setModel("anthropic/claude-sonnet-5")

	got := m.statusLine()
	if !strings.Contains(got, "ready") {
		t.Fatalf("statusLine() = %q, want it to contain \"ready\" while idle", got)
	}
	if !strings.Contains(got, "ctx unknown") {
		t.Fatalf("statusLine() = %q, want context explicitly unknown before a request reports usage", got)
	}
}

func TestStatusLine_ShowsLiveElapsedTimeWhileRunning(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.running = true
	m.turnStart = time.Now().Add(-3 * time.Second)

	got := m.statusLine()
	if !strings.Contains(got, "working...") {
		t.Fatalf("statusLine() = %q, want \"working...\" while running", got)
	}
	if !strings.Contains(got, "3.0s") {
		t.Fatalf("statusLine() = %q, want it to show the elapsed time (~3.0s)", got)
	}
}

// Regression: a turn that's mostly back-to-back tool calls with no
// assistant text in between leaves every token/context figure frozen at
// 0 for a real stretch of genuine work (usage is only reported once, at
// EventDone) — which reads as a hung UI. The tool-call count is
// concrete, immediately-available evidence that something is happening.
func TestStatusLine_ShowsToolCallCountWhileRunning(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.running = true
	m.renderApplicationEvent(app.Event{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "bash"}})
	m.renderApplicationEvent(app.Event{Kind: "tool_call", Details: app.Details{ToolPresent: true, ToolName: "bash"}})

	got := m.statusLine()
	if !strings.Contains(got, "2 tool calls") {
		t.Fatalf("statusLine() = %q, want it to mention \"2 tool calls\"", got)
	}
}

func TestStatusLine_OmitsToolCallCountBeforeAnyToolCall(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.running = true

	if got := m.statusLine(); strings.Contains(got, "tool call") {
		t.Fatalf("statusLine() = %q, want no tool-call mention before any tool has run this turn", got)
	}
}

func TestStartRun_ResetsToolCallCount(t *testing.T) {
	events := make(chan app.BackendEvent, 4)
	m := applicationModel(t, app.New(applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) { return events, nil }}, app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()
	m.toolCallsThisTurn = 5

	cmd := m.startRun("go again")
	events <- app.BackendEvent{Done: true}
	close(events)
	driveApplication(t, m, cmd)

	if m.toolCallsThisTurn != 0 {
		t.Fatalf("toolCallsThisTurn = %d after startRun, want reset to 0", m.toolCallsThisTurn)
	}
}

// Regression: PageUp/PageDown/ctrl+u/ctrl+d must scroll the transcript
// viewport directly, independent of the textarea (which owns plain
// up/down for cursor movement) — before this, nothing forwarded these
// keys to the viewport at all, so there was no way to scroll back
// through earlier output.
func TestHandleKey_ScrollsViewportIndependentlyOfTextarea(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	m.transcript = lines
	m.refreshViewport()

	if !m.viewport.AtBottom() {
		t.Fatal("expected the viewport to start at the bottom (stick-to-bottom default)")
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.viewport.AtBottom() {
		t.Fatal("expected pgup to scroll away from the bottom")
	}
	offsetAfterPgUp := m.viewport.YOffset

	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.viewport.YOffset <= offsetAfterPgUp {
		t.Fatalf("expected ctrl+d to scroll further down (YOffset %d), got %d", offsetAfterPgUp, m.viewport.YOffset)
	}
}

// Regression: pgdown and ctrl+u look like mirror images of pgdown/ctrl+u
// above in the implementation (handleKey's scroll switch), but nothing
// asserted their direction specifically — a copy-paste slip (e.g.
// ctrl+u wired to HalfPageDown) would pass every other scroll test.
func TestHandleKey_PgDownAndCtrlUScrollCorrectDirections(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	m.transcript = lines
	m.refreshViewport()

	m.viewport.GotoTop()
	topOffset := m.viewport.YOffset

	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.viewport.YOffset <= topOffset {
		t.Fatalf("expected pgdown to scroll down from the top (YOffset %d), got %d", topOffset, m.viewport.YOffset)
	}
	afterPgDown := m.viewport.YOffset

	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.viewport.YOffset >= afterPgDown {
		t.Fatalf("expected ctrl+u to scroll back up (YOffset %d), got %d", afterPgDown, m.viewport.YOffset)
	}
}

// Regression: a keyboard scroll while an approval is pending used to
// fall through to the pending-response switch's default case and
// silently deny the tool call — exactly the moment a user most wants to
// scroll back through a long diff or command preview before deciding.
func TestHandleKey_ScrollDuringPendingApprovalDoesNotRespond(t *testing.T) {
	m, cmd, returned, release := pendingServiceApproval(t)
	m.resize(80, 24)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	m.transcript = lines
	m.refreshViewport()

	for _, key := range []tea.KeyMsg{{Type: tea.KeyPgUp}, {Type: tea.KeyPgDown}, {Type: tea.KeyCtrlU}, {Type: tea.KeyCtrlD}} {
		m.handleKey(key)
	}

	if m.pending == nil {
		t.Fatal("scrolling while an approval is pending must not resolve it")
	}
	select {
	case <-returned:
		t.Fatal("scrolling resolved the live approval")
	default:
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	release()
	driveApplication(t, m, cmd)
}

// Regression: refreshViewport used to call GotoBottom() unconditionally
// on every update, so the instant any new event arrived (a streamed
// delta, a tool result) it yanked the view back down even if the user
// had just scrolled up to reread something.
func TestRefreshViewport_PreservesManualScrollPosition(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	m.transcript = lines
	m.refreshViewport()

	m.viewport.GotoTop()
	if m.viewport.AtBottom() {
		t.Fatal("expected GotoTop to leave the viewport away from the bottom")
	}

	m.transcript = append(m.transcript, "a new line arrived")
	m.refreshViewport()

	if m.viewport.AtBottom() {
		t.Fatal("refreshViewport must not re-snap to the bottom when the user had scrolled away from it")
	}
}

func TestRefreshViewport_StillFollowsBottomWhenAlreadyThere(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)
	m.transcript = []string{"line 1"}
	m.refreshViewport()

	m.transcript = append(m.transcript, "line 2")
	m.refreshViewport()

	if !m.viewport.AtBottom() {
		t.Fatal("refreshViewport should still auto-follow the bottom when the user hasn't scrolled away from it")
	}
}

func TestHandleAgentEvent_EventDoneAccumulatesSessionUsage(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())

	m.renderApplicationEvent(app.Event{Kind: "usage", Details: app.Details{UsageKnown: true, InputTokens: 100, OutputTokens: 10}})
	m.renderApplicationEvent(app.Event{Kind: "usage", Details: app.Details{UsageKnown: true, InputTokens: 50, OutputTokens: 5}})

	want := llm.Usage{InputTokens: 150, OutputTokens: 15}
	if m.sessionUsage != want {
		t.Fatalf("sessionUsage = %+v, want %+v (cumulative across both turns)", m.sessionUsage, want)
	}
	// lastUsage reflects only the most recent turn, not the running total.
	if m.lastUsage == nil || m.lastUsage.InputTokens != 50 {
		t.Fatalf("lastUsage = %+v, want the second turn's usage only", m.lastUsage)
	}
}

func TestModel_RunEndedMsgFreezesLastTurnDuration(t *testing.T) {
	events := make(chan app.BackendEvent, 4)
	m := applicationModel(t, app.New(applicationBackend{run: func(context.Context, string) (<-chan app.BackendEvent, error) { return events, nil }}, app.Options{MaxIterations: 1}), t.TempDir())
	defer m.CloseApplication()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("hello")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- app.BackendEvent{Kind: "usage", Done: true, Details: app.Details{UsageKnown: true, InputTokens: 10, OutputTokens: 2}}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "last turn")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestUsageLine_ShowsLiveTurnEstimateWhileRunning(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.running = true
	m.streamBuf.WriteString(strings.Repeat("a", 400)) // 400/4 = 100 estimated tokens

	got := m.usageLine()
	if !strings.Contains(got, "turn ~100 tok") {
		t.Fatalf("usageLine() = %q, want a live \"turn ~100 tok\" estimate while streaming", got)
	}
}

func TestUsageLine_ShowsCompletedTurnTotalWhenIdle(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.lastUsage = &llm.Usage{InputTokens: 100, OutputTokens: 20}

	got := m.usageLine()
	if !strings.Contains(got, "turn 120 tok") {
		t.Fatalf("usageLine() = %q, want the completed turn's real total (120 tok), not an estimate", got)
	}
}

// Regression: after a live /model switch, the startup banner naming the
// original model scrolls out of the visible transcript, leaving no
// on-screen indication of which model is actually active — exactly the
// kind of confusion this session's earlier /model self-identification
// bugs came from. The status line stays put, so it's where this needs
// to live.
func TestUsageLine_ShowsActiveModel(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.setModel("openrouter/anthropic/claude-sonnet-5")

	got := m.usageLine()
	if !strings.Contains(got, "openrouter/anthropic/claude-sonnet-5") {
		t.Fatalf("usageLine() = %q, want it to show the active model", got)
	}
}

// Regression: the reported ctx figure must be flagged, not just quietly
// stated, once it crosses contextAlertThreshold — this is the user's
// signal that harness's automatic preventive compaction (or a manual
// /compact) is worth watching for.
func TestContextSummary_UsesAlertStyleAboveThreshold(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.setModel("anthropic/claude-sonnet-5") // 200k window
	m.lastRequestUsage = &llm.Usage{InputTokens: 190_000}

	got := m.contextSummary()
	want := statusAlertStyle.Render(fmt.Sprintf("ctx %s/%s (%.0f%%)", formatTokenCount(190_000), formatTokenCount(200_000), 95.0))
	if got != want {
		t.Fatalf("contextSummary() = %q, want alert-styled %q", got, want)
	}
}

func TestContextSummary_UsesIdleStyleBelowThreshold(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.setModel("anthropic/claude-sonnet-5") // 200k window
	m.lastRequestUsage = &llm.Usage{InputTokens: 10_000}

	got := m.contextSummary()
	want := statusIdleStyle.Render(fmt.Sprintf("ctx %s/%s (%.0f%%)", formatTokenCount(10_000), formatTokenCount(200_000), 5.0))
	if got != want {
		t.Fatalf("contextSummary() = %q, want idle-styled %q", got, want)
	}
}

// Regression: live-streaming text must stay plain (not run through
// glamour) until it flushes. This used to render live, but Bubble Tea
// processes one message at a time on a single goroutine — a full
// glamour re-render of the whole growing buffer on every delta blocked
// that goroutine for the render's full duration, which scales with
// buffer size, so a long dense response's cumulative render time
// compounded into a UI that was completely unresponsive (not even
// Ctrl+C worked) for a real stretch of wall-clock time. See
// refreshViewport's own comment for the full account.
func TestModel_LiveStreamStaysPlainUntilFlush(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.renderApplicationEvent(app.Event{Kind: "text", Text: "some **bold** text"})
	m.refreshViewport()

	content := m.viewport.View()
	if !strings.Contains(content, "some **bold** text") {
		t.Fatalf("viewport content = %q, want the raw streaming text shown verbatim (unrendered) while still streaming", content)
	}

	m.flushStream()
	flushed := m.transcript[len(m.transcript)-1]
	if !strings.Contains(flushed, "bold") {
		t.Fatalf("flushed transcript entry = %q, want it to contain the rendered word %q", flushed, "bold")
	}
}

// Regression guard for the hang above: refreshViewport runs on every
// single streamed delta, so its cost must stay roughly linear in the
// CURRENT buffer size — never compounding across the stream the way a
// per-delta Markdown re-render of the whole (ever-growing) buffer did,
// where the cumulative cost over hundreds of deltas grew closer to
// quadratic. This grows streamBuf incrementally across 400 calls
// (standing in for 400 deltas of one long response, ending around 20KB)
// and checks the running total, not just one call in isolation.
func TestRefreshViewport_StaysFastAcrossALongGrowingStream(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.resize(80, 24)

	start := time.Now()
	for i := 0; i < 400; i++ {
		m.streamBuf.WriteString(strings.Repeat("w", 50))
		m.refreshViewport()
	}
	elapsed := time.Since(start)
	// The full qualification suite runs this under the race detector with
	// repository-wide atomic coverage. Leave enough headroom for shared CI
	// runners while retaining a low upper bound for a 400-delta response.
	if elapsed > 5*time.Second {
		t.Fatalf("400 refreshViewport calls across a growing ~20KB stream took %v, want under 5s under race and coverage instrumentation — something whose cost compounds with delta count (e.g. a Markdown render) was reintroduced into this hot path", elapsed)
	}
}

func TestRunModelCommand_RefreshesContextWindowOnSwitch(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.setModel("anthropic/claude-sonnet-5")
	// Same provider ("anthropic") on both sides so this doesn't need a
	// BuildProvider — SwitchModel only rebuilds the LLM client when the
	// provider itself changes.
	m.SetController(&Controller{
		Rt: &runtime.Runtime{Provider: "anthropic", Model: "claude-sonnet-5"},
	})

	m.handleCommand("/model anthropic/claude-sonnet-4-6")

	if m.model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("model = %q, want %q", m.model, "anthropic/claude-sonnet-4-6")
	}
	if m.contextWindow != 1_000_000 {
		t.Fatalf("contextWindow = %d, want 1000000 (claude-sonnet-4-6's 1M window, vs claude-sonnet-5's 200k)", m.contextWindow)
	}
}

func TestNewModel_DefaultsMarkdownStyleToConfigDefault(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	if m.markdownStyle != config.DefaultMarkdownStyle {
		t.Fatalf("markdownStyle = %q, want the config default %q", m.markdownStyle, config.DefaultMarkdownStyle)
	}
}

func TestSetMarkdownStyle_AffectsFlushedRendering(t *testing.T) {
	m := applicationModel(t, app.New(nil, app.Options{}), t.TempDir())
	m.SetMarkdownStyle("light")

	m.renderApplicationEvent(app.Event{Kind: "text", Text: "some **bold** text"})
	m.flushStream()

	want := renderMarkdown("some **bold** text", m.termWidth, "light")
	if len(m.transcript) != 1 || m.transcript[0] != want {
		t.Fatalf("transcript = %v, want a single entry rendered with the \"light\" style: %q", m.transcript, want)
	}
}

func contains(haystack []byte, needle string) bool {
	return len(needle) == 0 || indexOf(string(haystack), needle) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

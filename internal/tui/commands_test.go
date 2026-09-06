package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

// fakeLLMProvider is a distinguishable llm.LLMProvider stand-in for
// SwitchModel tests that need to tell two provider values apart by
// identity. The bare struct{ llm.LLMProvider }{} literal used elsewhere
// in this file can't do that: two zero-valued instances of the same
// anonymous type both hold a nil embedded interface and compare equal
// to each other regardless of which one a piece of code actually chose.
type fakeLLMProvider struct {
	llm.LLMProvider
	name string
}

func TestIsCommand(t *testing.T) {
	cases := map[string]bool{"/exit": true, "/": true, "hello": false, "": false}
	for in, want := range cases {
		if got := isCommand(in); got != want {
			t.Errorf("isCommand(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHandleCommand_Help(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.handleCommand("/help")
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "/exit") {
		t.Fatalf("expected help text in transcript, got %v", m.transcript)
	}
}

func TestHandleCommand_Exit(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	cmd := m.handleCommand("/exit")
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for /exit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestHandleCommand_Clear(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.transcript = []string{"some line"}
	m.handleCommand("/clear")
	if len(m.transcript) != 0 {
		t.Fatalf("expected empty transcript after /clear, got %v", m.transcript)
	}
}

func TestHandleCommand_Unknown(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.handleCommand("/nope")
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "unknown command") {
		t.Fatalf("expected unknown-command message, got %v", m.transcript)
	}
}

func TestHandleCommand_WithoutControllerReportUnavailable(t *testing.T) {
	for _, cmdText := range []string{"/model", "/new", "/compact"} {
		m := NewModel(&fakeRunner{}, t.TempDir())
		m.handleCommand(cmdText)
		if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "not available") {
			t.Fatalf("%s: expected unavailable message, got %v", cmdText, m.transcript)
		}
	}
}

func TestController_CurrentModel(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{Provider: "anthropic", Model: "claude-sonnet-5"}}
	if got, want := c.CurrentModel(), "anthropic/claude-sonnet-5"; got != want {
		t.Fatalf("CurrentModel() = %q, want %q", got, want)
	}
}

func TestController_SwitchModel_SameProviderJustUpdatesModel(t *testing.T) {
	rt := &runtime.Runtime{Provider: "anthropic", Model: "claude-haiku-4-5"}
	c := &Controller{Rt: rt, BuildProvider: func(string, string) (llm.LLMProvider, error) {
		t.Fatal("BuildProvider should not be called for a same-provider switch")
		return nil, nil
	}}
	if err := c.SwitchModel("anthropic/claude-sonnet-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if rt.Model != "claude-sonnet-5" {
		t.Fatalf("Model = %q, want claude-sonnet-5", rt.Model)
	}
}

func TestController_SwitchModel_DifferentProviderRebuildsLLM(t *testing.T) {
	rt := &runtime.Runtime{Provider: "anthropic", Model: "claude-sonnet-5"}
	var built string
	fakeLLM := struct{ llm.LLMProvider }{}
	c := &Controller{Rt: rt, BuildProvider: func(providerName, baseURL string) (llm.LLMProvider, error) {
		built = providerName
		return fakeLLM, nil
	}}
	if err := c.SwitchModel("openai/gpt-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if built != "openai" {
		t.Fatalf("BuildProvider called with %q, want openai", built)
	}
	if rt.Provider != "openai" || rt.Model != "gpt-5" {
		t.Fatalf("Provider/Model = %q/%q, want openai/gpt-5", rt.Provider, rt.Model)
	}
}

// Regression: SwitchModel used to update Rt.LLM/Provider/Model only,
// leaving Rt.Compaction.Summarizer and Rt.FallbackModel silently pinned
// to whatever provider hand started with — so compaction kept
// summarizing through a now-abandoned client, and a retryable error
// would misfire FallbackModel's stale bare model id against the new one.
func TestController_SwitchModel_DifferentProviderUpdatesCompactionAndClearsFallback(t *testing.T) {
	// Named with a distinguishing field, not the bare struct{
	// llm.LLMProvider }{} literal used elsewhere in this file: two
	// zero-valued instances of that anonymous type are indistinguishable
	// (both hold a nil embedded interface), so a naive identity check
	// against them can't actually tell "correctly updated" from "bug:
	// left at the old value" apart.
	anthropicLLM := fakeLLMProvider{name: "anthropic-llm"}
	rt := &runtime.Runtime{
		Provider:      "anthropic",
		Model:         "claude-sonnet-5",
		FallbackModel: "claude-haiku-4-5",
		Compaction: &compaction.Manager{
			Summarizer: &compaction.Summarizer{Provider: anthropicLLM, Model: "claude-sonnet-5"},
		},
	}
	openaiLLM := fakeLLMProvider{name: "openai-llm"}
	c := &Controller{Rt: rt, BuildProvider: func(string, string) (llm.LLMProvider, error) {
		return openaiLLM, nil
	}}

	if err := c.SwitchModel("openai/gpt-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}

	if rt.FallbackModel != "" {
		t.Errorf("FallbackModel = %q, want cleared after a cross-provider switch", rt.FallbackModel)
	}
	if rt.Compaction.Summarizer.Provider != openaiLLM {
		t.Error("Compaction.Summarizer.Provider was not updated to the new provider")
	}
	if rt.Compaction.Summarizer.Model != "gpt-5" {
		t.Errorf("Compaction.Summarizer.Model = %q, want gpt-5", rt.Compaction.Summarizer.Model)
	}
}

func TestController_SwitchModel_SameProviderUpdatesCompactionModelKeepsFallback(t *testing.T) {
	anthropicLLM := fakeLLMProvider{name: "anthropic-llm"}
	rt := &runtime.Runtime{
		Provider:      "anthropic",
		Model:         "claude-haiku-4-5",
		FallbackModel: "claude-haiku-4-5",
		Compaction: &compaction.Manager{
			Summarizer: &compaction.Summarizer{Provider: anthropicLLM, Model: "claude-haiku-4-5"},
		},
	}
	c := &Controller{Rt: rt, BuildProvider: func(string, string) (llm.LLMProvider, error) {
		t.Fatal("BuildProvider should not be called for a same-provider switch")
		return nil, nil
	}}

	if err := c.SwitchModel("anthropic/claude-sonnet-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}

	if rt.FallbackModel != "claude-haiku-4-5" {
		t.Errorf("FallbackModel = %q, want it left alone on a same-provider switch", rt.FallbackModel)
	}
	if rt.Compaction.Summarizer.Provider != anthropicLLM {
		t.Error("Compaction.Summarizer.Provider should not change on a same-provider switch")
	}
	if rt.Compaction.Summarizer.Model != "claude-sonnet-5" {
		t.Errorf("Compaction.Summarizer.Model = %q, want claude-sonnet-5 (kept in sync with the active run's model)", rt.Compaction.Summarizer.Model)
	}
}

func TestController_SwitchModel_NilCompactionIsSafe(t *testing.T) {
	rt := &runtime.Runtime{Provider: "anthropic", Model: "claude-sonnet-5"}
	c := &Controller{Rt: rt}
	if err := c.SwitchModel("anthropic/claude-haiku-4-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if rt.Model != "claude-haiku-4-5" {
		t.Fatalf("Model = %q, want claude-haiku-4-5", rt.Model)
	}
}

func TestController_SwitchModel_RejectsMalformedModel(t *testing.T) {
	c := &Controller{Rt: &runtime.Runtime{Provider: "anthropic", Model: "claude-sonnet-5"}}
	if err := c.SwitchModel("not-a-valid-model"); err == nil {
		t.Fatal("expected an error for a malformed provider/model string")
	}
}

// fakeSkillProvider is a minimal runtime.SkillProvider stand-in so tests
// can assert FormatIndex's result actually reaches the rebuilt static
// prompt, rather than a hardcoded default.
type fakeSkillProvider struct{ index string }

func (f fakeSkillProvider) FormatIndex() string       { return f.index }
func (f fakeSkillProvider) Get(string) (string, bool) { return "", false }

// Regression: SwitchModel used to leave Rt.StaticSystemPrompt untouched,
// so the "you are running as model X" line baked in at startup kept
// naming the OLD model forever — a model asked "which model are you"
// after switching would answer with whatever hand started up as, not
// what it was actually switched to.
func TestController_SwitchModel_RebuildsStaticSystemPromptWhenConfigured(t *testing.T) {
	rt := &runtime.Runtime{
		Provider:  "openrouter",
		Model:     "meta/muse-spark-1.3",
		AgentID:   "hand",
		AgentName: "Hand",
		Workspace: "/tmp/workspace",
		Tools:     tool.NewRegistry(),
		Skills:    fakeSkillProvider{index: "\n\nSkills Index:\n- example"},
	}
	var gotModel string
	c := &Controller{
		Rt: rt,
		RebuildSystemPrompt: func(providerModel string) string {
			gotModel = providerModel
			return "identity for " + providerModel
		},
	}

	if err := c.SwitchModel("openrouter/anthropic/claude-sonnet-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}

	if gotModel != "openrouter/anthropic/claude-sonnet-5" {
		t.Fatalf("RebuildSystemPrompt called with %q, want the new provider/model", gotModel)
	}
	if !strings.Contains(rt.StaticSystemPrompt, "identity for openrouter/anthropic/claude-sonnet-5") {
		t.Fatalf("StaticSystemPrompt = %q, want the rebuilt identity text", rt.StaticSystemPrompt)
	}
	if !strings.Contains(rt.StaticSystemPrompt, "Skills Index") {
		t.Fatalf("StaticSystemPrompt = %q, want the live skills index preserved across the switch", rt.StaticSystemPrompt)
	}
}

func TestController_SwitchModel_NilRebuildSystemPromptLeavesStaticPromptAlone(t *testing.T) {
	rt := &runtime.Runtime{
		Provider:           "anthropic",
		Model:              "claude-haiku-4-5",
		StaticSystemPrompt: "original identity",
	}
	c := &Controller{Rt: rt}
	if err := c.SwitchModel("anthropic/claude-sonnet-5"); err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if rt.StaticSystemPrompt != "original identity" {
		t.Fatalf("StaticSystemPrompt = %q, want unchanged when RebuildSystemPrompt is nil", rt.StaticSystemPrompt)
	}
}

func TestController_NewSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Load("hand", "workspace-key")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rt := &runtime.Runtime{AgentID: "hand", Session: sess}
	c := &Controller{Rt: rt, Store: store, SessionKey: "workspace-key"}

	rt.Session.Append(session.UserMessageEntry("hello"))

	if err := c.NewSession(); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if len(rt.Session.History()) != 0 {
		t.Fatalf("expected a fresh empty session, got %d history entries", len(rt.Session.History()))
	}
}

func TestController_Compact_NoManagerSkipsCleanly(t *testing.T) {
	rt := &runtime.Runtime{AgentID: "hand"}
	c := &Controller{Rt: rt}
	result, err := c.Compact(context.Background())
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if result.Compacted {
		t.Fatal("expected Compacted=false with no compaction manager configured")
	}
}

func TestMatchingCommands(t *testing.T) {
	if got := matchingCommands("hello"); got != nil {
		t.Fatalf("non-command prefix should not match, got %v", got)
	}
	if got := matchingCommands("/model extra"); got != nil {
		t.Fatalf("prefix with a space (into arguments) should not match, got %v", got)
	}
	all := matchingCommands("/")
	if len(all) != len(commandDefs) {
		t.Fatalf("\"/\" should match every command, got %d of %d", len(all), len(commandDefs))
	}
	only := matchingCommands("/mo")
	if len(only) != 1 || only[0].name != "/model" {
		t.Fatalf("\"/mo\" should match only /model, got %v", only)
	}
}

// The dropdown tests below drive Model.Update directly rather than going
// through a real tea.Program (as the streaming tests elsewhere in this
// package do): the Program's run loop applies key messages on its own
// goroutine, so reading Model fields right after tm.Send races it. Slash
// commands are pure key-handling logic with no goroutines involved, so a
// direct, synchronous Update call is both correct and simpler here.

func TestModel_SlashTriggersDropdown(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.textarea.SetValue("/mo")

	suggestions := m.commandSuggestions()
	if len(suggestions) != 1 || suggestions[0].name != "/model" {
		t.Fatalf("expected only /model to match, got %v", suggestions)
	}
}

func TestModel_DropdownTabCompletes(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.textarea.SetValue("/mo")

	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if got := strings.TrimSpace(m.textarea.Value()); got != "/model" {
		t.Fatalf("expected input to be completed to /model, got %q", got)
	}
}

func TestModel_DropdownEnterRunsHighlighted(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.textarea.SetValue("/hel")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.transcript) == 0 || !strings.Contains(m.transcript[0], "Commands:") {
		t.Fatalf("expected /help output in transcript, got %v", m.transcript)
	}
}

func TestModel_DropdownEscDismisses(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.textarea.SetValue("/mo")

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if got := m.textarea.Value(); got != "" {
		t.Fatalf("expected esc to clear the input, got %q", got)
	}
}

func TestModel_DropdownArrowKeysMoveSelection(t *testing.T) {
	m := NewModel(&fakeRunner{}, t.TempDir())
	m.textarea.SetValue("/")

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	got := strings.TrimSpace(m.textarea.Value())
	want := commandDefs[1].name
	if got != want {
		t.Fatalf("after one down-arrow, tab should complete to %q, got %q", want, got)
	}
}

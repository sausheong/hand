# hand Phase 1 (Core Loop + TUI Shell) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a working `hand` binary: an interactive terminal coding
agent, built on `harness`, with a Bubble Tea TUI, four tools wired in
(file read/write/edit, bash, web fetch/search, todo), and a
non-persisted yes/no approval gate on every write/edit/bash call.

**Architecture:** A single Go binary composes one harness `runtime.Runtime`
per process and drives it from a Bubble Tea program. `internal/config`
owns the user-level config file; `internal/agentio` owns the tool
registry, the `AgentSpec`, and the approval bridge (a `BeforeToolUse`
hook that blocks on a channel until the TUI answers); `internal/tui`
owns the Bubble Tea `Model` and the goroutine that forwards harness's
`AgentEvent` stream into the program. `cmd/hand` is wiring only.

**Tech Stack:** Go 1.25.1, `github.com/sausheong/harness` (local
replace), `github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss`
(TUI), stdlib `flag`/`encoding/json` for config and CLI.

**Spec:** [docs/superpowers/specs/2026-09-04-hand-phase1-design.md](../specs/2026-09-04-hand-phase1-design.md)

## Global Constraints

- Go 1.25.1 (matches harness's floor — `harness` requires 1.25.1+).
- Module path: `github.com/sausheong/hand`.
- `harness` is consumed via `replace github.com/sausheong/harness =>
  /Users/sausheong/projects/harness` (matches the convention already
  used by the sibling `sidecar` project in this workspace) — do not
  attempt to fetch it from a module proxy.
- Tool names gated behind approval: exactly `write_file`, `edit_file`,
  `bash`. All other tools (`read_file`, `web_fetch`, `web_search`,
  `todo_write`) are never gated, per the spec.
- No approval decision is ever persisted in Phase 1 — every gated call
  prompts again, even for an identical command in the same turn.
- No slash commands, plan mode, subagents, memory/skills, MCP, browser
  tool, session persistence, or one-shot (`-p`) mode — all explicitly
  out of scope for this phase (see spec's Non-goals).

---

### Task 1: Project scaffolding + config package

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config{Model string}`, `config.DefaultModel string`,
  `config.DefaultPath() (string, error)`, `config.Load(path string)
  (Config, error)`, `config.Save(path string, cfg Config) error`,
  `config.ResolveModel(flagValue string, cfg Config) string`.

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd /Users/sausheong/projects/hand
go mod init github.com/sausheong/hand
```

Then edit the generated `go.mod` to read exactly:

```
module github.com/sausheong/hand

go 1.25.1

require github.com/sausheong/harness v0.3.6

replace github.com/sausheong/harness => /Users/sausheong/projects/harness
```

- [ ] **Step 2: Write the failing tests for `internal/config`**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/config"
)

func TestLoad_CreatesDefaultOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Model != config.DefaultModel {
		t.Fatalf("Model = %q, want default %q", cfg.Model, config.DefaultModel)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Load to create %s, but ReadFile failed: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatal("config file was created but is empty")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.Config{Model: "openai/gpt-5"}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestResolveModel_FlagOverridesConfig(t *testing.T) {
	cases := []struct {
		name      string
		flagValue string
		cfg       config.Config
		want      string
	}{
		{"flag empty uses config", "", config.Config{Model: "anthropic/claude-sonnet-5"}, "anthropic/claude-sonnet-5"},
		{"flag set overrides config", "qwen/qwen-max", config.Config{Model: "anthropic/claude-sonnet-5"}, "qwen/qwen-max"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveModel(tc.flagValue, tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveModel(%q, %+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/config/... -v`
Expected: FAIL — `package config: no Go files` or undefined symbols
(`config.Load`, `config.Config`, etc. don't exist yet).

- [ ] **Step 4: Implement `internal/config`**

Create `internal/config/config.go`:

```go
// Package config reads and writes hand's user-level configuration
// file at ~/.hand/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultModel is used when no config file exists yet.
const DefaultModel = "anthropic/claude-sonnet-5"

// Config is the on-disk shape of ~/.hand/config.json. API keys are
// never stored here — each provider reads its key from its own
// standard environment variable.
type Config struct {
	Model string `json:"model"`
}

// DefaultPath returns ~/.hand/config.json for the current user.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hand", "config.json"), nil
}

// Load reads the config at path. If the file does not exist, Load
// creates it (and any missing parent directories) with DefaultModel
// and returns that default.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Config{Model: DefaultModel}
		if saveErr := Save(path, cfg); saveErr != nil {
			return Config{}, fmt.Errorf("write default config: %w", saveErr)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path as indented JSON, creating any missing
// parent directories.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// ResolveModel returns flagValue if non-empty, otherwise cfg.Model.
// Does not persist anything — a --model flag overrides the loaded
// config for this invocation only.
func ResolveModel(flagValue string, cfg Config) string {
	if flagValue != "" {
		return flagValue
	}
	return cfg.Model
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS (all three tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/config
git commit -m "Add project scaffolding and user-level config package"
```

---

### Task 2: Approval bridge (`internal/agentio`)

**Files:**
- Create: `internal/agentio/approval.go`
- Test: `internal/agentio/approval_test.go`

**Interfaces:**
- Consumes: nothing from prior tasks.
- Produces: `agentio.Sender` (interface: `Send(msg any)`),
  `agentio.ApprovalRequest{Tool string, Input json.RawMessage, Respond
  chan bool}`, `agentio.NewApprovalHook(sender Sender) func(ctx
  context.Context, name string, input json.RawMessage)
  (runtime.HookDecision, error)`. This returned func is assigned to
  `runtime.LifecycleHooks.BeforeToolUse` by Task 4.

- [ ] **Step 1: Write the failing tests**

Create `internal/agentio/approval_test.go`:

```go
package agentio_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []any
}

func (f *fakeSender) Send(msg any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
}

func (f *fakeSender) first() (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return nil, false
	}
	return f.sent[0], true
}

func waitForRequest(t *testing.T, sender *fakeSender) agentio.ApprovalRequest {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if msg, ok := sender.first(); ok {
			req, ok := msg.(agentio.ApprovalRequest)
			if !ok {
				t.Fatalf("sent message has type %T, want agentio.ApprovalRequest", msg)
			}
			return req
		}
		select {
		case <-deadline:
			t.Fatal("hook never sent an approval request")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestApprovalHook_UngatedToolsAllowedImmediately(t *testing.T) {
	for _, name := range []string{"read_file", "web_fetch", "web_search", "todo_write"} {
		t.Run(name, func(t *testing.T) {
			sender := &fakeSender{}
			hook := agentio.NewApprovalHook(sender)

			decision, err := hook(context.Background(), name, json.RawMessage(`{}`))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !decision.Allow {
				t.Fatalf("expected Allow=true for ungated tool %q, got %+v", name, decision)
			}
			if _, ok := sender.first(); ok {
				t.Fatalf("ungated tool %q should never prompt", name)
			}
		})
	}
}

func TestApprovalHook_GatedToolBlocksThenRespects(t *testing.T) {
	cases := []struct {
		tool   string
		answer bool
	}{
		{"write_file", true},
		{"write_file", false},
		{"edit_file", true},
		{"bash", false},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			sender := &fakeSender{}
			hook := agentio.NewApprovalHook(sender)

			resultCh := make(chan struct {
				decision runtime.HookDecision
				err      error
			}, 1)
			go func() {
				decision, err := hook(context.Background(), tc.tool, json.RawMessage(`{"x":1}`))
				resultCh <- struct {
					decision runtime.HookDecision
					err      error
				}{decision, err}
			}()

			req := waitForRequest(t, sender)
			if req.Tool != tc.tool {
				t.Fatalf("ApprovalRequest.Tool = %q, want %q", req.Tool, tc.tool)
			}

			select {
			case <-resultCh:
				t.Fatal("hook returned before the approval was answered")
			case <-time.After(20 * time.Millisecond):
			}

			req.Respond <- tc.answer

			select {
			case res := <-resultCh:
				if res.err != nil {
					t.Fatalf("unexpected error: %v", res.err)
				}
				if res.decision.Allow != tc.answer {
					t.Fatalf("Allow = %v, want %v", res.decision.Allow, tc.answer)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("hook did not return after approval was answered")
			}
		})
	}
}

func TestApprovalHook_ContextCancelDenies(t *testing.T) {
	sender := &fakeSender{}
	hook := agentio.NewApprovalHook(sender)

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	var allow bool
	go func() {
		decision, err := hook(ctx, "bash", json.RawMessage(`{}`))
		allow = decision.Allow
		resultCh <- err
	}()

	waitForRequest(t, sender)
	cancel()

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allow {
			t.Fatal("expected Allow=false after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hook did not return after context cancellation")
	}
}
```

Add `"github.com/sausheong/harness/runtime"` to the test file's import
block (alongside `context`, `encoding/json`, `sync`, `testing`, and
`time`) — the file above already asserts against `runtime.HookDecision`
directly.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/agentio/... -v`
Expected: FAIL — `agentio.NewApprovalHook` and `agentio.ApprovalRequest`
undefined.

- [ ] **Step 3: Implement `internal/agentio/approval.go`**

```go
// Package agentio wires harness's tool registry, agent spec, and
// approval gating for hand.
package agentio

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sausheong/harness/runtime"
)

// Sender delivers a message to the TUI program. Declared with a plain
// `any` parameter (rather than bubbletea's tea.Msg) so this package has
// no dependency on the TUI framework; internal/tui provides the
// concrete adapter around *tea.Program.
type Sender interface {
	Send(msg any)
}

// ApprovalRequest is sent to the TUI when a gated tool call needs a
// yes/no decision before it may proceed. Respond must receive exactly
// one value — the BeforeToolUse hook blocks reading it until it does.
type ApprovalRequest struct {
	Tool    string
	Input   json.RawMessage
	Respond chan bool
}

// gatedTools names the tools that require approval before executing.
// read_file, web_fetch, web_search, and todo_write are never gated.
var gatedTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
}

// NewApprovalHook returns a runtime.LifecycleHooks.BeforeToolUse
// closure. For a gated tool it sends an ApprovalRequest via sender and
// blocks until the request's Respond channel receives an answer (or
// the call's context is cancelled, which denies). Every other tool is
// allowed immediately with no prompt.
func NewApprovalHook(sender Sender) func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		if !gatedTools[name] {
			return runtime.HookDecision{Allow: true}, nil
		}

		req := ApprovalRequest{Tool: name, Input: input, Respond: make(chan bool, 1)}
		sender.Send(req)

		select {
		case allow := <-req.Respond:
			if allow {
				return runtime.HookDecision{Allow: true}, nil
			}
			return runtime.HookDecision{Allow: false, Reason: fmt.Sprintf("user denied %s", name)}, nil
		case <-ctx.Done():
			return runtime.HookDecision{Allow: false, Reason: "approval cancelled: " + ctx.Err().Error()}, nil
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/agentio/... -v`
Expected: PASS (all cases in all three test functions).

- [ ] **Step 5: Commit**

```bash
git add internal/agentio/approval.go internal/agentio/approval_test.go
git commit -m "Add approval bridge for gated tool calls"
```

---

### Task 3: Tool registry (`internal/agentio`)

**Files:**
- Create: `internal/agentio/registry.go`
- Test: `internal/agentio/registry_test.go`

**Interfaces:**
- Consumes: nothing from prior tasks.
- Produces: `agentio.BuildRegistry(workDir string) *tool.Registry`.

- [ ] **Step 1: Add the tool dependencies and write the failing test**

Run:
```bash
go get github.com/sausheong/harness/tools/file
go get github.com/sausheong/harness/tools/bash
go get github.com/sausheong/harness/tools/web
go get github.com/sausheong/harness/tools/todo
```
(These resolve via the `replace` directive in `go.mod`, no network
fetch of harness itself.)

Create `internal/agentio/registry_test.go`:

```go
package agentio_test

import (
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

func TestBuildRegistry_RegistersExpectedTools(t *testing.T) {
	reg := agentio.BuildRegistry(t.TempDir())

	got := map[string]bool{}
	for _, def := range reg.ToolDefs() {
		got[def.Name] = true
	}

	want := []string{
		"read_file", "write_file", "edit_file",
		"bash", "web_fetch", "web_search", "todo_write",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("registry missing tool %q", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("registry has %d tools, want exactly %d (%v)", len(got), len(want), want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agentio/... -run TestBuildRegistry -v`
Expected: FAIL — `agentio.BuildRegistry` undefined.

- [ ] **Step 3: Implement `internal/agentio/registry.go`**

```go
package agentio

import (
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/file"
	"github.com/sausheong/harness/tools/todo"
	"github.com/sausheong/harness/tools/web"
)

// BuildRegistry returns the tool.Registry for hand's four Phase 1
// tool packages, all scoped to workDir. bash.BashTool's ExecPolicy is
// left nil (full) — the approval bridge in approval.go is the safety
// net for bash in Phase 1, not the exec policy.
func BuildRegistry(workDir string) *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(&file.ReadFileTool{WorkDir: workDir})
	reg.Register(&file.WriteFileTool{WorkDir: workDir})
	reg.Register(&file.EditFileTool{WorkDir: workDir})
	reg.Register(&bash.BashTool{WorkDir: workDir})
	reg.Register(&web.WebFetchTool{})
	reg.Register(&web.WebSearchTool{})
	reg.Register(&todo.TodoWriteTool{WorkDir: workDir})
	return reg
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agentio/... -run TestBuildRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/agentio/registry.go internal/agentio/registry_test.go
git commit -m "Add tool registry wiring for file, bash, web, and todo tools"
```

---

### Task 4: AgentSpec builder (`internal/agentio`)

**Files:**
- Create: `internal/agentio/spec.go`
- Test: `internal/agentio/spec_test.go`

**Interfaces:**
- Consumes: nothing from prior tasks directly (takes a hook function of
  the same shape `NewApprovalHook` from Task 2 returns, but does not
  import `agentio`'s own `NewApprovalHook` — the caller in Task 8 wires
  the two together).
- Produces: `agentio.SystemPrompt string`, `agentio.BuildAgentSpec(model,
  workspace string, hook func(ctx context.Context, name string, input
  json.RawMessage) (runtime.HookDecision, error)) runtime.AgentSpec`.

- [ ] **Step 1: Write the failing test**

Create `internal/agentio/spec_test.go`:

```go
package agentio_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/runtime"
)

func TestBuildAgentSpec_SetsExpectedFields(t *testing.T) {
	hook := func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: true}, nil
	}

	spec := agentio.BuildAgentSpec("anthropic/claude-sonnet-5", "/tmp/work", hook)

	if spec.Model != "anthropic/claude-sonnet-5" {
		t.Errorf("Model = %q, want %q", spec.Model, "anthropic/claude-sonnet-5")
	}
	if spec.Workspace != "/tmp/work" {
		t.Errorf("Workspace = %q, want %q", spec.Workspace, "/tmp/work")
	}
	if spec.SystemPrompt == "" {
		t.Error("SystemPrompt is empty")
	}
	if spec.MaxTurns <= 0 {
		t.Errorf("MaxTurns = %d, want > 0", spec.MaxTurns)
	}
	if spec.Loop.Hooks.BeforeToolUse == nil {
		t.Error("Loop.Hooks.BeforeToolUse is nil, want the provided hook")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agentio/... -run TestBuildAgentSpec -v`
Expected: FAIL — `agentio.BuildAgentSpec` undefined.

- [ ] **Step 3: Implement `internal/agentio/spec.go`**

```go
package agentio

import (
	"context"
	"encoding/json"

	"github.com/sausheong/harness/runtime"
)

// SystemPrompt is hand's fixed identity prompt for Phase 1 (no
// per-project customization yet).
const SystemPrompt = `You are hand, a terminal-based coding assistant. You can read, write, and edit files in the current workspace, run shell commands, and fetch or search the web. Track multi-step work with the todo tool. Be direct and concise: prefer making the requested change over describing what you would do.`

// MaxTurns caps the tool-use loop for a single hand run.
const MaxTurns = 50

// BuildAgentSpec builds the single AgentSpec hand uses for the whole
// process. hook is wired as Loop.Hooks.BeforeToolUse — callers pass the
// closure returned by NewApprovalHook.
func BuildAgentSpec(model, workspace string, hook func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)) runtime.AgentSpec {
	return runtime.AgentSpec{
		ID:           "hand",
		Name:         "hand",
		Model:        model,
		Workspace:    workspace,
		SystemPrompt: SystemPrompt,
		MaxTurns:     MaxTurns,
		Loop: runtime.LoopConfig{
			Hooks: runtime.LifecycleHooks{
				BeforeToolUse: hook,
			},
		},
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agentio/... -run TestBuildAgentSpec -v`
Expected: PASS.

- [ ] **Step 5: Run the full `internal/agentio` suite**

Run: `go test ./internal/agentio/... -v`
Expected: PASS (Tasks 2, 3, and 4's tests all pass together).

- [ ] **Step 6: Commit**

```bash
git add internal/agentio/spec.go internal/agentio/spec_test.go
git commit -m "Add AgentSpec builder wiring system prompt and approval hook"
```

---

### Task 5: TUI event bridge (`internal/tui`)

**Files:**
- Create: `internal/tui/events.go`
- Test: `internal/tui/events_test.go`

**Interfaces:**
- Consumes: `runtime.AgentEvent` (from `github.com/sausheong/harness/runtime`).
- Produces: `tui.runEndedMsg{}` (unexported, but its existence and
  semantics — "sent exactly once, after the events channel closes" —
  are relied on by Task 6/7), `tui.StreamEvents(p programSender, events
  <-chan runtime.AgentEvent)` (unexported `programSender` interface;
  `*tea.Program` satisfies it, used directly by Task 6/7's `Model`).

- [ ] **Step 1: Add the Bubble Tea dependency and write the failing test**

Run:
```bash
go get github.com/charmbracelet/bubbletea
```

Create `internal/tui/events_test.go`:

```go
package tui

import (
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

type fakeProgramSender struct {
	mu   sync.Mutex
	sent []tea.Msg
}

func (f *fakeProgramSender) Send(msg tea.Msg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
}

func (f *fakeProgramSender) snapshot() []tea.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tea.Msg, len(f.sent))
	copy(out, f.sent)
	return out
}

func TestStreamEvents_ForwardsEventsThenRunEnded(t *testing.T) {
	events := make(chan runtime.AgentEvent, 2)
	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "hi"}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	sender := &fakeProgramSender{}
	done := make(chan struct{})
	go func() {
		StreamEvents(sender, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StreamEvents did not return after the channel closed")
	}

	got := sender.snapshot()
	if len(got) != 3 {
		t.Fatalf("sent %d messages, want 3 (2 events + runEndedMsg): %+v", len(got), got)
	}
	if ev, ok := got[0].(runtime.AgentEvent); !ok || ev.Text != "hi" {
		t.Errorf("first message = %+v, want AgentEvent{Text: \"hi\"}", got[0])
	}
	if ev, ok := got[1].(runtime.AgentEvent); !ok || ev.Type != runtime.EventDone {
		t.Errorf("second message = %+v, want AgentEvent{Type: EventDone}", got[1])
	}
	if _, ok := got[2].(runEndedMsg); !ok {
		t.Errorf("third message = %+v (%T), want runEndedMsg", got[2], got[2])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v`
Expected: FAIL — `StreamEvents` and `runEndedMsg` undefined.

- [ ] **Step 3: Implement `internal/tui/events.go`**

```go
// Package tui is hand's Bubble Tea terminal UI.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sausheong/harness/runtime"
)

// runEndedMsg marks that an AgentEvent channel has closed. harness's
// Run goroutine can exit several error paths without ever emitting a
// terminal EventDone (see runtime.go's early r.emit(EventError); return
// paths), so relying on EventDone alone to know a turn ended would
// leave the UI stuck in the "running" state on those paths. StreamEvents
// sends this exactly once, after ranging over events completes.
type runEndedMsg struct{}

// programSender is satisfied by *tea.Program. Declared as an interface
// (rather than taking *tea.Program directly) so StreamEvents is
// testable without a real terminal.
type programSender interface {
	Send(msg tea.Msg)
}

// StreamEvents forwards every AgentEvent from events into p as a
// tea.Msg, in order, then sends a single runEndedMsg once events
// closes. Intended to run in its own goroutine for the lifetime of one
// agent turn — an ordinary tea.Cmd cannot represent an unbounded stream
// like this, since a Cmd only ever produces one terminal Msg.
func StreamEvents(p programSender, events <-chan runtime.AgentEvent) {
	for ev := range events {
		p.Send(ev)
	}
	p.Send(runEndedMsg{})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tui/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/tui/events.go internal/tui/events_test.go
git commit -m "Add AgentEvent-to-tea.Msg streaming bridge"
```

---

### Task 6: TUI model — transcript, streaming, submit, quit (`internal/tui`)

**Files:**
- Create: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: `runtime.AgentEvent`, `runEndedMsg`/`programSender` (Task 5,
  same package).
- Produces: `tui.Runner` (interface: `Run(ctx context.Context, userMsg
  string, images []llm.ImageContent) (<-chan runtime.AgentEvent,
  error)` — satisfied by `*runtime.Runtime`), `tui.NewModel(rt Runner)
  *Model`, `(*Model) BindProgram(p *tea.Program)`, and the `tea.Model`
  interface (`Init`/`Update`/`View`) on `*Model`. Approval-prompt
  handling (the `agentio.ApprovalRequest` case) is added in Task 7 —
  this task's `Update` does not reference `agentio` at all yet.

- [ ] **Step 1: Add the remaining Bubble Tea dependencies and write the failing test**

Run:
```bash
go get github.com/charmbracelet/bubbles
```

Create `internal/tui/model_test.go`:

```go
package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	teatest "github.com/charmbracelet/x/exp/teatest"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

type fakeRunner struct {
	events chan runtime.AgentEvent
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, userMsg string, images []llm.ImageContent) (<-chan runtime.AgentEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

func TestModel_StreamsAssistantTextIntoTranscript(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("hello there")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: "Hi"}
	events <- runtime.AgentEvent{Type: runtime.EventTextDelta, Text: " back"}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Hi back") && contains(bts, "ready")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ToolCallAndResultRender(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run a build")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	events <- runtime.AgentEvent{Type: runtime.EventToolCallStart, ToolCall: &llm.ToolCall{Name: "read_file"}}
	events <- runtime.AgentEvent{Type: runtime.EventToolResult, Result: &toolResultOK}
	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "[tool: read_file]") && contains(bts, "✓")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
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
```

Add this package-level test fixture to the same file (above the two
test functions), since `tool.ToolResult` cannot be composite-literaled
inline inside the `events <-` line above without a named variable:

```go
var toolResultOK = tool.ToolResult{Output: "ok"}
```

Add `"github.com/sausheong/harness/tool"` to the imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/... -v`
Expected: FAIL — `NewModel`, `Model`, `BindProgram` undefined (and the
`github.com/charmbracelet/x/exp/teatest` import fails to resolve until
Step 3 adds it as a dependency).

Before proceeding, run:
```bash
go get github.com/charmbracelet/x/exp/teatest
```

- [ ] **Step 3: Implement `internal/tui/model.go`**

```go
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

// Model is the hand Bubble Tea program. It uses pointer-receiver
// Init/Update/View methods (rather than the value-receiver style most
// Bubble Tea examples use) so a *tea.Program reference can be injected
// after construction via BindProgram — breaking the construction cycle
// between the Program and the approval hook that needs to Send into it
// (see internal/agentio.Sender and cmd/hand/main.go).
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

// NewModel builds an hand TUI model driving rt. Call BindProgram with
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS (`TestStreamEvents_ForwardsEventsThenRunEnded` from Task
5, plus this task's two new tests).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/tui/model.go internal/tui/model_test.go
git commit -m "Add TUI model with streaming transcript and submit/quit handling"
```

---

### Task 7: Approval prompt integration (`internal/tui`)

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: `agentio.ApprovalRequest` (Task 2).
- Produces: no new exported symbols — extends `Model.Update` and
  `Model.View` from Task 6 to handle `agentio.ApprovalRequest`.

- [ ] **Step 1: Write the failing test**

Add to `internal/tui/model_test.go` (new imports: add
`"github.com/sausheong/hand/internal/agentio"` and
`"encoding/json"` to the existing import block):

```go
func TestModel_ApprovalPromptBlocksAndRespondsYes(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("write a file")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan bool, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "write_file",
		Input:   json.RawMessage(`{"path":"x.txt"}`),
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Allow write_file?")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	select {
	case allow := <-respond:
		if !allow {
			t.Fatal("expected Respond to receive true after pressing y")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Respond never received a value after pressing y")
	}

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "approved: write_file")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestModel_ApprovalPromptRespondsNoOnAnyOtherKey(t *testing.T) {
	events := make(chan runtime.AgentEvent, 4)
	runner := &fakeRunner{events: events}
	m := NewModel(runner)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.BindProgram(tm.GetProgram())

	tm.Type("run rm -rf")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	respond := make(chan bool, 1)
	tm.Send(agentio.ApprovalRequest{
		Tool:    "bash",
		Input:   json.RawMessage(`{"command":"rm -rf /"}`),
		Respond: respond,
	})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return contains(bts, "Allow bash?")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case allow := <-respond:
		if allow {
			t.Fatal("expected Respond to receive false after pressing enter on a pending prompt")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Respond never received a value")
	}

	events <- runtime.AgentEvent{Type: runtime.EventDone}
	close(events)
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/... -run TestModel_ApprovalPrompt -v`
Expected: FAIL — pressing `y`/`Enter` on a pending request does nothing
yet (`Model` has no `pending` state), so `respond` never receives a
value and both new tests time out.

- [ ] **Step 3: Extend `internal/tui/model.go`**

Add the import `"github.com/sausheong/hand/internal/agentio"` to
`model.go`'s import block.

Add a `pending` field to the `Model` struct, right after `running bool`:

```go
	running bool
	pending *agentio.ApprovalRequest
	cancel  context.CancelFunc
```

Add a case to `Update`'s type switch, alongside the existing `case
runtime.AgentEvent:` case:

```go
	case agentio.ApprovalRequest:
		req := msg
		m.pending = &req
		m.refreshViewport()
		return m, nil
```

Change `handleKey` to check for a pending approval first, before its
existing `switch msg.String()`:

```go
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending != nil {
		switch msg.String() {
		case "y", "Y":
			m.pending.Respond <- true
			m.transcript = append(m.transcript, fmt.Sprintf("  approved: %s", m.pending.Tool))
			m.pending = nil
			m.refreshViewport()
		default:
			m.pending.Respond <- false
			m.transcript = append(m.transcript, fmt.Sprintf("  denied: %s", m.pending.Tool))
			m.pending = nil
			m.refreshViewport()
		}
		return m, nil
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
		return m, m.startRun(text)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}
```

Change `statusLine` to show the pending prompt first:

```go
func (m *Model) statusLine() string {
	if m.pending != nil {
		return fmt.Sprintf("Allow %s? [y/N]", m.pending.Tool)
	}
	if m.running {
		return m.spinner.View() + " working..."
	}
	return "ready"
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS — every test in `internal/tui` (Tasks 5, 6, and 7).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "Add approval prompt handling to the TUI model"
```

---

### Task 8: `cmd/hand` wiring

**Files:**
- Create: `cmd/hand/main.go`

**Interfaces:**
- Consumes: `config.DefaultPath`, `config.Load`, `config.ResolveModel`
  (Task 1); `agentio.NewApprovalHook`, `agentio.BuildRegistry`,
  `agentio.BuildAgentSpec` (Tasks 2-4); `tui.NewModel`,
  `(*tui.Model).BindProgram` (Tasks 6-7); harness's
  `llm.ParseProviderModel`, `providers/{anthropic,openai,gemini,qwen}`,
  `runtime.BuildRuntime`, `session.NewSession`.
- Produces: the `hand` binary's `main()`. No new testable package —
  this task is verified by `go build` and a manual smoke run, matching
  how harness's own `examples/*/main.go` files are verified (none of
  them have `_test.go` files).

- [ ] **Step 1: Add the remaining provider dependencies**

Run:
```bash
go get github.com/sausheong/harness/providers/anthropic
go get github.com/sausheong/harness/providers/openai
go get github.com/sausheong/harness/providers/gemini
go get github.com/sausheong/harness/providers/qwen
go get github.com/sausheong/harness/session
```

- [ ] **Step 2: Implement `cmd/hand/main.go`**

```go
// Command hand is an interactive terminal coding agent built on
// harness. Run it from the directory you want it to work in.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/tui"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
	"github.com/sausheong/harness/providers/gemini"
	"github.com/sausheong/harness/providers/openai"
	"github.com/sausheong/harness/providers/qwen"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

// programSender adapts a *tea.Program to agentio.Sender, whose method
// signature uses `any` (not bubbletea's tea.Msg) so the agentio package
// itself has no dependency on the TUI framework. Program is set after
// tea.NewProgram runs, once the *tea.Program value exists — see main().
type programSender struct {
	Program *tea.Program
}

func (s *programSender) Send(msg any) {
	s.Program.Send(msg)
}

func buildProvider(providerName string) (llm.LLMProvider, error) {
	switch providerName {
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
		}
		return anthropic.NewAnthropicProvider(key, ""), nil

	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set")
		}
		return openai.NewOpenAIProvider(key, ""), nil

	case "gemini":
		key := os.Getenv("GEMINI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is not set")
		}
		p, err := gemini.NewGeminiProvider(context.Background(), key)
		if err != nil {
			return nil, fmt.Errorf("build gemini provider: %w", err)
		}
		return p, nil

	case "qwen":
		key := os.Getenv("DASHSCOPE_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("DASHSCOPE_API_KEY is not set")
		}
		return qwen.NewQwenProvider(key, ""), nil

	default:
		return nil, fmt.Errorf("unknown provider %q (want anthropic, openai, gemini, or qwen)", providerName)
	}
}

func run() error {
	modelFlag := flag.String("model", "", "provider/model to use, e.g. anthropic/claude-sonnet-5 (overrides ~/.hand/config.json for this run)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	model := config.ResolveModel(*modelFlag, cfg)

	providerName, _ := llm.ParseProviderModel(model)
	if providerName == "" {
		return fmt.Errorf("model %q must be in \"provider/model\" form, e.g. anthropic/claude-sonnet-5", model)
	}
	provider, err := buildProvider(providerName)
	if err != nil {
		return fmt.Errorf("build provider %q: %w", providerName, err)
	}

	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	sender := &programSender{}
	hook := agentio.NewApprovalHook(sender)
	spec := agentio.BuildAgentSpec(model, workspace, hook)
	reg := agentio.BuildRegistry(workspace)
	sess := session.NewSession(spec.ID, "main")

	rt, err := runtime.BuildRuntime(
		runtime.RuntimeDeps{},
		runtime.RuntimeInputs{
			Provider: provider,
			Tools:    reg,
			Session:  sess,
		},
		spec,
	)
	if err != nil {
		return fmt.Errorf("build runtime: %w", err)
	}
	defer rt.Close()

	m := tui.NewModel(rt)
	program := tea.NewProgram(m, tea.WithAltScreen())
	m.BindProgram(program)
	sender.Program = program

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hand:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Build the whole module**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS across `internal/config`, `internal/agentio`, and
`internal/tui`.

- [ ] **Step 5: Manual smoke test**

Run (from a disposable scratch directory, with a real API key):
```bash
mkdir -p /tmp/hand-smoke && cd /tmp/hand-smoke
ANTHROPIC_API_KEY=sk-ant-... go run github.com/sausheong/hand/cmd/hand
```
Expected: the TUI launches, shows a `ready` status line and an empty
input box. Typing a message that asks the agent to create a file (e.g.
"create a file called hello.txt with the text hi") and pressing Enter
streams assistant text, then shows an `Allow write_file? [y/N]` prompt;
pressing `y` creates the file in `/tmp/hand-smoke`, pressing `n`
denies it. `Ctrl-C` while idle quits the program.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/hand/main.go
git commit -m "Wire config, agentio, and tui into the hand binary"
```

## Self-Review Notes

- **Spec coverage:** every Phase 1 spec section has a task —
  architecture/approval bridge → Tasks 2, 7; components (config,
  agentio, tui, events) → Tasks 1, 2, 3, 4, 5, 6; error handling
  (startup errors, mid-run errors, denied approvals) → Task 8's `run()`
  error returns, Task 6/7's `EventError`/deny-path rendering; testing →
  every task's own test step plus Task 8's build/test/smoke steps.
- **Type consistency checked:** `agentio.ApprovalRequest` (Task 2) is
  the exact type referenced in Task 7's `Model.Update` case and in
  `main.go`'s wiring (indirectly, via the hook closure's return type
  `runtime.HookDecision`, also used consistently from Task 2 through
  Task 4's `BuildAgentSpec` signature). `agentio.Sender`'s `Send(msg
  any)` and `tui.programSender`'s `Send(msg tea.Msg)` are deliberately
  different interfaces for a documented reason (Task 8's
  `programSender` comment, Task 5's `programSender` comment) — this is
  not an inconsistency to fix.
- **No placeholders:** every step has complete, concrete code; nothing
  reads "TODO" or "similar to Task N".

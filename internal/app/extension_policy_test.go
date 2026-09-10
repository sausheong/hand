package app

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/llm/llmtest"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type policyCallProvider struct {
	llmtest.Base
	calls    int
	admitted <-chan struct{}
}

func (p *policyCallProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	first := p.calls == 1
	out := make(chan llm.ChatEvent, 2)
	go func() {
		defer close(out)
		if first {
			out <- llm.ChatEvent{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{ID: "write-1", Name: "write_file", Input: json.RawMessage(`{"path":"protected.txt"}`)}}
			if p.admitted != nil {
				select {
				case <-p.admitted:
				case <-ctx.Done():
					return
				}
			}
		}
		out <- llm.ChatEvent{Type: llm.EventDone}
	}()
	return out, nil
}

type signalledPermission struct {
	tool.PermissionChecker
	once     sync.Once
	admitted chan struct{}
}

func (p *signalledPermission) Check(ctx context.Context, agent, name string, input json.RawMessage) tool.Decision {
	decision := p.PermissionChecker.Check(ctx, agent, name, input)
	p.once.Do(func() { close(p.admitted) })
	return decision
}

type policyExecution struct{ calls int }

func (p *policyExecution) Execute(context.Context, string, json.RawMessage) (tool.ToolResult, error) {
	p.calls++
	return tool.ToolResult{Output: "executed"}, nil
}
func (*policyExecution) ToolDefs() []llm.ToolDef { return nil }

type hostDenial struct{}

func (hostDenial) Check(context.Context, string, string, json.RawMessage) tool.Decision {
	return tool.Decision{Behavior: tool.DecisionDeny, Reason: "host denial"}
}
func (hostDenial) FilterToolDefs([]llm.ToolDef, string) []llm.ToolDef { return nil }
func TestRuntimeExtensionPolicyVetoAndHostPrecedence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "policy")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-tool-policy")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, out)
	}
	for _, mode := range []string{"runturn", "serial", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			sess := session.NewSession("hand", "policy")
			defer sess.Close()
			executor := &policyExecution{}
			provider := &policyCallProvider{}
			rt := &runtime.Runtime{Session: sess, LLM: provider, Tools: executor, MaxTurns: 2}
			rt.AgentLoop.StreamingTools = mode == "streaming"
			defer rt.Close()
			controller := &Controller{Rt: rt}
			workspace := t.TempDir()
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "tool-policy", Executable: binary, Workspace: workspace, Capabilities: []string{"policy.check"}, Mandatory: true})
			if err != nil {
				t.Fatal(err)
			}
			host, err := controller.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "private"), Identities: map[string]string{"tool-policy": "examples/policy"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			wrapper := rt.Permission.(*extensionPermission)
			if mode == "streaming" {
				signal := make(chan struct{})
				provider.admitted = signal
				rt.Permission = &signalledPermission{PermissionChecker: wrapper, admitted: signal}
			}
			if mode == "runturn" {
				result, err := rt.RunTurn(ctx, "write", nil, nil)
				if err != nil || result.Err != nil {
					t.Fatal(result, err)
				}
			} else {
				events, err := rt.Run(ctx, "write", nil)
				if err != nil {
					t.Fatal(err)
				}
				for event := range events {
					if event.Error != nil {
						t.Fatal(event.Error)
					}
				}
			}
			if executor.calls != 0 {
				t.Fatal("policy-denied tool executed")
			}
			found := false
			for _, entry := range sess.View() {
				if entry.Type == session.EntryTypeToolResult {
					var data session.ToolResultData
					if err = json.Unmarshal(entry.Data, &data); err != nil {
						t.Fatal(err)
					}
					if data.ToolCallID == "write-1" && data.Error != "" {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("denied tool result not recorded")
			}
			allowed := rt.Permission.Check(ctx, "hand", "write_file", json.RawMessage(`{"path":"other.txt"}`))
			if allowed.Behavior != tool.DecisionAllow {
				t.Fatal(allowed)
			}
			if err = host.Close(); err != nil {
				t.Fatal(err)
			}
			if decision := rt.Permission.Check(ctx, "hand", "write_file", json.RawMessage(`{"path":"other.txt"}`)); decision.Behavior != tool.DecisionDeny {
				t.Fatal("closed mandatory policy failed open")
			}
			wrapper.previous = hostDenial{}
			if decision := wrapper.Check(ctx, "hand", "write_file", json.RawMessage(`{}`)); decision.Reason != "host denial" {
				t.Fatal("extension checked after host denial", decision)
			}
		})
	}
}
func (*policyExecution) Names() []string              { return []string{"write_file"} }
func (*policyExecution) Get(string) (tool.Tool, bool) { return policyDescriptor{}, true }

// A concurrency-safe fixture forces the streaming kickoff path. Its execution
// would be observable through policyExecution and must remain denied.
type policyDescriptor struct{}

func (policyDescriptor) Name() string                { return "write_file" }
func (policyDescriptor) Description() string         { return "policy fixture" }
func (policyDescriptor) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (policyDescriptor) Execute(context.Context, json.RawMessage) (tool.ToolResult, error) {
	panic("fixture descriptor must not execute")
}
func (policyDescriptor) IsConcurrencySafe(json.RawMessage) bool { return true }

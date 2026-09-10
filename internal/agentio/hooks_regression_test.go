package agentio_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
)

func TestHookLargePayloadUsesJSONStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.json")
	hooks := []config.HookConfig{{Event: "UserPromptSubmit", Command: "sh", Args: []string{"-c", `cat > "$1"`, "hook", path}}}
	lc := agentio.BuildLifecycleHooks(hooks, t.TempDir(), nil, nil)
	prompt := strings.Repeat("payload", 200000)
	if _, _, err := lc.OnUserPromptSubmit(context.Background(), prompt, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version int    `json:"version"`
		Prompt  string `json:"prompt"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Prompt != prompt {
		t.Fatal("hook payload lost data or version")
	}
}

func TestMandatoryPreToolFailureDeniesByDefault(t *testing.T) {
	hooks := []config.HookConfig{{Event: "PreToolUse", Command: "sh", Args: []string{"-c", "exit 1"}}}
	lc := agentio.BuildLifecycleHooks(hooks, t.TempDir(), nil, nil)
	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allow {
		t.Fatal("validator execution failure allowed tool")
	}
}

func TestMandatoryStopFailuresAreErrors(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		args          []string
		timeout       int
	}{
		{"exit", "sh", []string{"-c", "exit 1"}, 0},
		{"spawn", "/nonexistent/hand-hook", nil, 0},
		{"timeout", "sleep", []string{"5"}, 1},
		{"output", "sh", []string{"-c", "head -c 100000 /dev/zero"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := agentio.EvaluateStopHooks(context.Background(), []config.HookConfig{{Event: "Stop", Command: tc.command, Args: tc.args, Timeout: tc.timeout}}, t.TempDir(), "completed", 1)
			if o.Err == nil || o.Continue {
				t.Fatalf("failed mandatory validator returned %+v", o)
			}
		})
	}
}

func TestHookCancellationNeverFailsOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := []config.HookConfig{{Event: "Stop", FailurePolicy: "warn", Command: "sh", Args: []string{"-c", "exit 0"}}}
	if o := agentio.EvaluateStopHooks(ctx, h, t.TempDir(), "completed", 1); o.Err == nil {
		t.Fatal("cancelled validation returned success")
	}
}

func TestHookBulkEnvironmentMigration(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "stdin", true: "legacy"}[legacy], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "env")
			t.Setenv("HAND_PROMPT", "inherited-stale-payload")
			hooks := []config.HookConfig{{Event: "UserPromptSubmit", LegacyEnv: legacy, Command: "sh", Args: []string{"-c", `printf '%s' "$HAND_PROMPT" > "$1"`, "hook", path}}}
			lc := agentio.BuildLifecycleHooks(hooks, t.TempDir(), nil, nil)
			if _, _, err := lc.OnUserPromptSubmit(context.Background(), "current", nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if legacy {
				want = "current"
			}
			if string(data) != want {
				t.Fatalf("environment = %q, want %q", data, want)
			}
			if legacy {
				if _, _, err := lc.OnUserPromptSubmit(context.Background(), strings.Repeat("x", 20000), nil); err == nil {
					t.Fatal("oversized legacy environment did not fail closed")
				}
			}
		})
	}
}

func TestHookStructuredToolInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	h := []config.HookConfig{{Event: "PreToolUse", Command: "sh", Args: []string{"-c", `cat > "$1"`, "hook", path}}}
	lc := agentio.BuildLifecycleHooks(h, t.TempDir(), nil, nil)
	if d, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{"command":"echo hello"}`)); err != nil || !d.Allow {
		t.Fatalf("decision %+v, %v", d, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got agentio.HookInput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Event != "PreToolUse" || got.ToolName != "bash" || string(got.ToolInput) != `{"command":"echo hello"}` {
		t.Fatalf("payload %+v", got)
	}
}

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
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

func alwaysAllow(_ context.Context, _ string, _ json.RawMessage) (runtime.HookDecision, error) {
	return runtime.HookDecision{Allow: true}, nil
}

func TestBuildLifecycleHooks_PreToolUse_AllowsOnExitZero(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Command: "sh", Args: []string{"-c", "exit 0"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatalf("decision = %+v, want Allow=true", decision)
	}
}

func TestBuildLifecycleHooks_PreToolUse_DeniesOnExitTwoWithStderrAndEnv(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Command: "sh", Args: []string{"-c", `echo "denied: $HAND_TOOL_NAME" >&2; exit 2`}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if decision.Allow {
		t.Fatal("decision.Allow = true, want false (hook exited 2)")
	}
	if !strings.Contains(decision.Reason, "denied: bash") {
		t.Fatalf("decision.Reason = %q, want it to contain the hook's stderr with HAND_TOOL_NAME set", decision.Reason)
	}
}

func TestBuildLifecycleHooks_PreToolUse_FailsOpenOnOtherNonzeroExit(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Command: "sh", Args: []string{"-c", "exit 1"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatal("a non-2 nonzero exit should fail open (allow), got denied")
	}
}

func TestBuildLifecycleHooks_PreToolUse_MatcherRestrictsToNamedTool(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Matcher: "bash", Command: "sh", Args: []string{"-c", "exit 2"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "read_file", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatal("a hook matched to \"bash\" must not fire for \"read_file\"")
	}
}

func TestBuildLifecycleHooks_PreToolUse_WildcardMatcherMatchesEveryTool(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Matcher: "*", Command: "sh", Args: []string{"-c", "exit 2"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "read_file", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if decision.Allow {
		t.Fatal("a \"*\" matcher should fire for every tool, including read_file")
	}
}

func TestBuildLifecycleHooks_PreToolUse_FallsThroughToApprovalHookWhenNotDenied(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Command: "sh", Args: []string{"-c", "exit 0"}},
	}
	denyApproval := func(_ context.Context, _ string, _ json.RawMessage) (runtime.HookDecision, error) {
		return runtime.HookDecision{Allow: false, Reason: "user denied bash"}, nil
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", denyApproval)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if decision.Allow || decision.Reason != "user denied bash" {
		t.Fatalf("decision = %+v, want the approval hook's own denial to still apply", decision)
	}
}

func TestBuildLifecycleHooks_PreToolUse_TimeoutFailsOpen(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "PreToolUse", Command: "sh", Args: []string{"-c", "sleep 5"}, Timeout: 1},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatal("a hook that times out should fail open (allow), not deny")
	}
}

func TestBuildLifecycleHooks_UserPromptSubmit_ExitTwoReturnsError(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "UserPromptSubmit", Command: "sh", Args: []string{"-c", "echo blocked >&2; exit 2"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	_, _, err := lc.OnUserPromptSubmit(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("OnUserPromptSubmit returned nil error, want the hook's exit-2 to abort the turn")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("error = %q, want it to contain the hook's stderr", err.Error())
	}
}

func TestBuildLifecycleHooks_UserPromptSubmit_ExitZeroPassesThroughUnchanged(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "UserPromptSubmit", Command: "sh", Args: []string{"-c", "exit 0"}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	prompt, images, err := lc.OnUserPromptSubmit(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("OnUserPromptSubmit returned error: %v", err)
	}
	if prompt != "hello" || images != nil {
		t.Fatalf("OnUserPromptSubmit = (%q, %v), want the prompt/images unchanged", prompt, images)
	}
}

func TestBuildLifecycleHooks_AfterToolUse_RunsRegardlessOfExitCode(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "posttooluse.out")
	hooks := []config.HookConfig{
		{Event: "PostToolUse", Command: "sh", Args: []string{"-c", `echo "$HAND_TOOL_NAME $HAND_TOOL_RESULT" > "` + outFile + `"; exit 1`}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	lc.AfterToolUse(context.Background(), "bash", json.RawMessage(`{}`), tool.ToolResult{Output: "ok"})

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("PostToolUse hook did not run: %v", err)
	}
	if !strings.Contains(string(got), "bash ok") {
		t.Fatalf("hook output = %q, want it to see HAND_TOOL_NAME and HAND_TOOL_RESULT", got)
	}
}

func TestBuildLifecycleHooks_OnSessionStart_Runs(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "sessionstart.out")
	hooks := []config.HookConfig{
		{Event: "SessionStart", Command: "sh", Args: []string{"-c", `echo started > "` + outFile + `"`}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	lc.OnSessionStart(context.Background(), nil)

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("SessionStart hook did not run: %v", err)
	}
}

func TestBuildLifecycleHooks_OnStop_RunsWithReason(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "stop.out")
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", `echo "$HAND_STOP_REASON" > "` + outFile + `"`}},
	}
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow)

	lc.OnStop(context.Background(), "completed")

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("Stop hook did not run: %v", err)
	}
	if strings.TrimSpace(string(got)) != "completed" {
		t.Fatalf("hook saw HAND_STOP_REASON=%q, want %q", strings.TrimSpace(string(got)), "completed")
	}
}

func TestBuildLifecycleHooks_NoHooksConfiguredFallsThroughToApprovalHook(t *testing.T) {
	lc := agentio.BuildLifecycleHooks(nil, "/tmp/work", alwaysAllow)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatal("with no hooks configured, BeforeToolUse should defer entirely to the approval hook")
	}
}

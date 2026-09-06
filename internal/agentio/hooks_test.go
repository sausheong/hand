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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", denyApproval, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

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
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, nil)

	lc.OnSessionStart(context.Background(), nil)

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("SessionStart hook did not run: %v", err)
	}
}

// Regression: OnStop must only capture the stop reason for the caller
// to read afterward — it must NOT run configured Stop-event hook
// commands itself. Those run from agentio.EvaluateStopHooks instead,
// called by hand's own driver (cmd/hand/main.go, internal/tui) once a
// turn's event channel has closed, since harness's Run() holds its own
// mutex for the full duration of OnStop's defer, making it impossible to
// build a continue-the-loop feature from inside OnStop itself.
func TestBuildLifecycleHooks_OnStop_CapturesReasonWithoutRunningStopHooks(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "stop.out")
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", `echo "$HAND_STOP_REASON" > "` + outFile + `"`}},
	}
	var stopReason string
	lc := agentio.BuildLifecycleHooks(hooks, "/tmp/work", alwaysAllow, &stopReason)

	lc.OnStop(context.Background(), "completed")

	if stopReason != "completed" {
		t.Fatalf("stopReason = %q, want %q", stopReason, "completed")
	}
	if _, err := os.Stat(outFile); err == nil {
		t.Fatal("OnStop ran the configured Stop hook's command; it must only capture the reason")
	}
}

func TestBuildLifecycleHooks_OnStop_NilStopReasonIsSafe(t *testing.T) {
	lc := agentio.BuildLifecycleHooks(nil, "/tmp/work", alwaysAllow, nil)
	lc.OnStop(context.Background(), "completed") // must not panic
}

func TestBuildLifecycleHooks_NoHooksConfiguredFallsThroughToApprovalHook(t *testing.T) {
	lc := agentio.BuildLifecycleHooks(nil, "/tmp/work", alwaysAllow, nil)

	decision, err := lc.BeforeToolUse(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("BeforeToolUse returned error: %v", err)
	}
	if !decision.Allow {
		t.Fatal("with no hooks configured, BeforeToolUse should defer entirely to the approval hook")
	}
}

func TestEvaluateStopHooks_NoHooksConfigured(t *testing.T) {
	outcome := agentio.EvaluateStopHooks(context.Background(), nil, "/tmp/work", "completed", 1)
	if outcome.Continue {
		t.Fatal("with no Stop hooks configured, Continue should be false")
	}
}

func TestEvaluateStopHooks_ExitZeroMeansDone(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}},
	}
	outcome := agentio.EvaluateStopHooks(context.Background(), hooks, "/tmp/work", "completed", 1)
	if outcome.Continue {
		t.Fatal("an exit-0 Stop hook means the goal was met; Continue should be false")
	}
}

func TestEvaluateStopHooks_ExitTwoUsesStdoutAsNextPrompt(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "echo 'fix the failing test'; exit 2"}},
	}
	outcome := agentio.EvaluateStopHooks(context.Background(), hooks, "/tmp/work", "completed", 1)
	if !outcome.Continue {
		t.Fatal("an exit-2 Stop hook means the goal was not met; Continue should be true")
	}
	if outcome.NextPrompt != "fix the failing test" {
		t.Fatalf("NextPrompt = %q, want the hook's stdout", outcome.NextPrompt)
	}
}

func TestEvaluateStopHooks_ExitTwoFallsBackToStderrThenDefaultMessage(t *testing.T) {
	stderrOnly := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "echo 'tests still red' >&2; exit 2"}},
	}
	outcome := agentio.EvaluateStopHooks(context.Background(), stderrOnly, "/tmp/work", "completed", 1)
	if outcome.NextPrompt != "tests still red" {
		t.Fatalf("NextPrompt = %q, want the hook's stderr when stdout is empty", outcome.NextPrompt)
	}

	neither := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 2"}},
	}
	outcome = agentio.EvaluateStopHooks(context.Background(), neither, "/tmp/work", "completed", 1)
	if outcome.NextPrompt == "" {
		t.Fatal("NextPrompt should fall back to a fixed default message when stdout and stderr are both empty")
	}
}

func TestEvaluateStopHooks_FirstAllowingHookDefersToNext(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 0"}},
		{Event: "Stop", Command: "sh", Args: []string{"-c", "echo 'second hook says keep going'; exit 2"}},
	}
	outcome := agentio.EvaluateStopHooks(context.Background(), hooks, "/tmp/work", "completed", 1)
	if !outcome.Continue || outcome.NextPrompt != "second hook says keep going" {
		t.Fatalf("outcome = %+v, want the second hook's denial to apply", outcome)
	}
}

func TestEvaluateStopHooks_FailsOpenOnOtherExitOrTimeout(t *testing.T) {
	other := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "exit 1"}},
	}
	if outcome := agentio.EvaluateStopHooks(context.Background(), other, "/tmp/work", "completed", 1); outcome.Continue {
		t.Fatal("a non-2 nonzero exit should fail open (Continue=false)")
	}

	timeout := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", "sleep 5"}, Timeout: 1},
	}
	if outcome := agentio.EvaluateStopHooks(context.Background(), timeout, "/tmp/work", "completed", 1); outcome.Continue {
		t.Fatal("a timed-out hook should fail open (Continue=false)")
	}
}

func TestEvaluateStopHooks_EnvVarsReachTheScript(t *testing.T) {
	hooks := []config.HookConfig{
		{Event: "Stop", Command: "sh", Args: []string{"-c", `echo "$HAND_STOP_REASON $HAND_GOAL_ITERATION $HAND_WORKSPACE"; exit 2`}},
	}
	outcome := agentio.EvaluateStopHooks(context.Background(), hooks, "/tmp/work", "max_turns", 3)
	if outcome.NextPrompt != "max_turns 3 /tmp/work" {
		t.Fatalf("NextPrompt = %q, want the hook to see HAND_STOP_REASON, HAND_GOAL_ITERATION, and HAND_WORKSPACE", outcome.NextPrompt)
	}
}

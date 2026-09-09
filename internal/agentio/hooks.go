package agentio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/process"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

// BeforeToolUseFunc is the shape of hand's existing approval-prompt
// hook (NewApprovalHook / NewOneShotApprovalHook) — the thing
// BuildLifecycleHooks composes its own PreToolUse hook commands with.
type BeforeToolUseFunc func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)

// hookDenyExitCode expresses a deliberate denial or request for more work.
const hookDenyExitCode = 2

// matchesHook reports whether matcher restricts a hook to toolName.
// "" or "*" matches every tool; anything else must match exactly. No
// glob/regex support in v1 — exact-or-everything is the smallest thing
// that works.
func matchesHook(matcher, toolName string) bool {
	return matcher == "" || matcher == "*" || matcher == toolName
}

// hookOutcome is the result of running one hook command.
type hookOutcome struct {
	exitCode int
	stdout   string
	stderr   string
	// timedOut and spawnErr distinguish "the command ran and exited
	// nonzero" from "the command never produced an exit code at all" —
	// the configured failure policy determines whether either blocks.
	timedOut  bool
	spawnErr  error
	cancelled bool
	truncated bool
}

// runHookCommand runs cfg.Command/Args with env layered on top of the
// current process's environment, bounded by cfg.Timeout (or
// config.DefaultHookTimeoutSeconds when zero).
const hookOutputLimit = 64 * 1024
const legacyHookPayloadLimit = 16 * 1024

type hookBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *hookBuffer) Len() int       { return b.buffer.Len() }
func (b *hookBuffer) String() string { return b.buffer.String() }

func (b *hookBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := hookOutputLimit - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

// HookInput is the version 1 JSON object delivered on stdin. ToolInput is
// structured JSON; tool output and error remain strings and may be large.
type HookInput struct {
	Identity      RunIdentity     `json:"identity"`
	Version       int             `json:"version"`
	Event         string          `json:"event"`
	Workspace     string          `json:"workspace"`
	Prompt        string          `json:"prompt,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolResult    string          `json:"tool_result,omitempty"`
	ToolError     string          `json:"tool_error,omitempty"`
	StopReason    string          `json:"stop_reason,omitempty"`
	GoalIteration int             `json:"goal_iteration,omitempty"`
}

func encodeHookInput(ctx context.Context, env map[string]string) ([]byte, error) {
	input := HookInput{Identity: IdentityFromContext(ctx), Version: 1, Event: env["HAND_HOOK_EVENT"], Workspace: env["HAND_WORKSPACE"], Prompt: env["HAND_PROMPT"], ToolName: env["HAND_TOOL_NAME"], ToolInput: json.RawMessage(env["HAND_TOOL_INPUT"]), ToolResult: env["HAND_TOOL_RESULT"], ToolError: env["HAND_TOOL_ERROR"], StopReason: env["HAND_STOP_REASON"]}
	input.GoalIteration, _ = strconv.Atoi(env["HAND_GOAL_ITERATION"])
	return json.Marshal(input)
}

func runHookCommand(ctx context.Context, cfg config.HookConfig, env map[string]string) hookOutcome {
	if err := config.ValidateHooks([]config.HookConfig{cfg}); err != nil {
		return hookOutcome{spawnErr: err}
	}
	payload, err := encodeHookInput(ctx, env)
	if err != nil {
		return hookOutcome{spawnErr: fmt.Errorf("encode hook input: %w", err)}
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(config.DefaultHookTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := process.Command(ctx, cfg.Command, cfg.Args...)
	// Do not inherit stale hook payloads from Hand's parent environment.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HAND_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	for k, v := range env {
		bulk := k == "HAND_PROMPT" || k == "HAND_TOOL_INPUT" || k == "HAND_TOOL_RESULT" || k == "HAND_TOOL_ERROR"
		if bulk && !cfg.LegacyEnv {
			continue
		}
		if len(v) > legacyHookPayloadLimit {
			return hookOutcome{spawnErr: fmt.Errorf("%s exceeds environment compatibility limit; read JSON stdin", k)}
		}
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Env = append(cmd.Env, "HAND_HOOK_VERSION=1")
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	// Bound inherited-pipe draining; the shared process runner also terminates
	// descendants on cancellation and after the hook's parent exits.
	cmd.WaitDelay = 250 * time.Millisecond
	var stdout, stderr hookBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = process.Run(cmd)
	o := hookOutcome{stdout: stdout.String(), stderr: stderr.String(), truncated: stdout.truncated || stderr.truncated}
	if ctx.Err() != nil {
		o.timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		o.cancelled = errors.Is(ctx.Err(), context.Canceled)
		return o
	}
	if err == nil {
		return o
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		o.exitCode = exitErr.ExitCode()
	} else {
		o.spawnErr = err
	}
	return o
}

func (o hookOutcome) failed() bool {
	return o.timedOut || o.cancelled || o.truncated || o.spawnErr != nil || o.exitCode != 0
}
func (o hookOutcome) explicitDeny() bool {
	return o.exitCode == hookDenyExitCode && !o.timedOut && !o.cancelled && !o.truncated && o.spawnErr == nil
}
func (o hookOutcome) failureReason() string {
	switch {
	case o.cancelled:
		return "hook cancelled"
	case o.timedOut:
		return "hook timed out"
	case o.truncated:
		return "hook output exceeded 64 KiB per stream"
	case o.spawnErr != nil:
		return "hook execution failed: " + o.spawnErr.Error()
	default:
		return fmt.Sprintf("hook exited %d: %s", o.exitCode, strings.TrimSpace(o.stderr))
	}
}
func hookMandatory(h config.HookConfig) bool {
	return h.FailurePolicy != "warn" && h.Event != "SessionStart" && h.Event != "PostToolUse"
}

// warnHookFailure logs a non-blocking problem with a hook command —
// used for every failure mode except an explicit hookDenyExitCode,
// which is a deliberate decision, not a failure.
func warnHookFailure(event, command string, o hookOutcome) {
	if o.failed() {
		slog.Warn("hook failed", "event", event, "command", command, "reason", o.failureReason())
	}
}

// runMatchingHooks stops on an explicit denial, cancellation, or a failed
// mandatory validator. Warning-only observers may continue after other errors.
func runMatchingHooks(ctx context.Context, hooks []config.HookConfig, event, toolName string, env map[string]string) (deny hookOutcome, denied bool) {
	for _, h := range hooks {
		if h.Event != event || !matchesHook(h.Matcher, toolName) {
			continue
		}
		o := runHookCommand(ctx, h, env)
		if o.explicitDeny() {
			return o, true
		}
		if o.cancelled || (hookMandatory(h) && o.failed()) {
			o.stderr = o.failureReason()
			return o, true
		}
		warnHookFailure(event, h.Command, o)
	}
	return hookOutcome{}, false
}

// BuildLifecycleHooks composes hooks (from config.json's hooks list)
// with hand's existing approval-prompt hook (beforeToolUse) into the
// full runtime.LifecycleHooks harness dispatches. A PreToolUse hook
// runs BEFORE beforeToolUse, so it can deny a call before the user is
// ever prompted — matching Claude Code's own PreToolUse ordering
// relative to its permission system. See config.HookConfig's doc
// comment for the exit-code contract.
//
// Stop-event hooks are deliberately NOT run here — harness's OnStop
// fires from inside Run()'s own defer stack while Run()'s mutex is
// still held, so nothing invoked from it can ever call Run() again to
// build a loop (see EvaluateStopHooks). The OnStop closure below only
// captures harness's own stop reason into *stopReason for the caller to
// read once Run()'s event channel has closed (safe without a mutex:
// that channel close happens only after this defer runs, by the same
// LIFO defer ordering that rules out looping from inside OnStop itself
// — see runtime.go's Run()). stopReason may be nil if the caller has no
// use for it.
func BuildLifecycleHooks(hooks []config.HookConfig, workspace string, beforeToolUse BeforeToolUseFunc, stopReason *string) runtime.LifecycleHooks {
	return runtime.LifecycleHooks{
		OnUserPromptSubmit: func(ctx context.Context, prompt string, images []llm.ImageContent) (string, []llm.ImageContent, error) {
			env := map[string]string{
				"HAND_HOOK_EVENT": "UserPromptSubmit",
				"HAND_WORKSPACE":  workspace,
				"HAND_PROMPT":     prompt,
			}
			if o, denied := runMatchingHooks(ctx, hooks, "UserPromptSubmit", "", env); denied {
				return prompt, images, &VerificationError{Cause: errors.New(o.stderr)}
			}
			return prompt, images, nil
		},

		OnSessionStart: func(ctx context.Context, _ *session.Session) {
			env := map[string]string{
				"HAND_HOOK_EVENT": "SessionStart",
				"HAND_WORKSPACE":  workspace,
			}
			for _, h := range hooks {
				if h.Event != "SessionStart" {
					continue
				}
				warnHookFailure("SessionStart", h.Command, runHookCommand(ctx, h, env))
			}
		},

		BeforeToolUse: func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
			env := map[string]string{
				"HAND_HOOK_EVENT": "PreToolUse",
				"HAND_WORKSPACE":  workspace,
				"HAND_TOOL_NAME":  name,
				"HAND_TOOL_INPUT": string(input),
			}
			if o, denied := runMatchingHooks(ctx, hooks, "PreToolUse", name, env); denied {
				return runtime.HookDecision{Allow: false, Reason: o.stderr}, nil
			}
			if beforeToolUse != nil {
				return beforeToolUse(ctx, name, input)
			}
			return runtime.HookDecision{Allow: true}, nil
		},

		AfterToolUse: func(ctx context.Context, name string, input json.RawMessage, result tool.ToolResult) {
			env := map[string]string{
				"HAND_HOOK_EVENT":  "PostToolUse",
				"HAND_WORKSPACE":   workspace,
				"HAND_TOOL_NAME":   name,
				"HAND_TOOL_INPUT":  string(input),
				"HAND_TOOL_RESULT": result.Output,
			}
			if result.Error != "" {
				env["HAND_TOOL_ERROR"] = result.Error
			}
			for _, h := range hooks {
				if h.Event != "PostToolUse" || !matchesHook(h.Matcher, name) {
					continue
				}
				warnHookFailure("PostToolUse", h.Command, runHookCommand(ctx, h, env))
			}
		},

		OnStop: func(_ context.Context, reason string) {
			if stopReason != nil {
				*stopReason = reason
			}
		},
	}
}

// GoalLoopOutcome is the result of evaluating a turn's Stop-event hooks
// for whether hand's driver should automatically start another turn.
type GoalLoopOutcome struct {
	Verified bool
	// Err means required verification could not establish success.
	Err        error
	Continue   bool
	NextPrompt string
}

// EvaluateStopHooks runs every Stop-event hook in hooks, in order, and
// is meant to be called by hand's own driver code (cmd/hand/main.go's
// runOneShot, internal/tui's runEndedMsg handling) once a turn's event
// channel has fully closed — never from inside harness's OnStop
// callback itself, which cannot be used to build a loop (see
// BuildLifecycleHooks' doc comment on why). reason is the stop reason
// BuildLifecycleHooks captured via its stopReason parameter
// ("completed", "max_turns", "error", "aborted"); iteration is the
// 1-based count of turns run so far in this chain.
//
// The first hook that exits hookDenyExitCode means "the goal has not
// been met" — Continue is true and NextPrompt becomes the next prompt
// to run automatically, taken from that hook's stdout (falling back to
// stderr, falling back to a fixed message if both are empty). Any other
// outcome with a mandatory failure returns Err. Explicit warning-only hooks
// may log failures and continue; cancellation always returns an error. Exit 0
// from every required validator allows the driver to finish this check.
func EvaluateStopHooks(ctx context.Context, hooks []config.HookConfig, workspace, reason string, iteration int) GoalLoopOutcome {
	env := map[string]string{
		"HAND_HOOK_EVENT":     "Stop",
		"HAND_WORKSPACE":      workspace,
		"HAND_STOP_REASON":    reason,
		"HAND_GOAL_ITERATION": strconv.Itoa(iteration),
	}
	verified := false
	for _, h := range hooks {
		if h.Event != "Stop" {
			continue
		}
		o := runHookCommand(ctx, h, env)
		if o.explicitDeny() {
			next := strings.TrimSpace(o.stdout)
			if next == "" {
				next = strings.TrimSpace(o.stderr)
			}
			if next == "" {
				next = "Continue — the configured Stop hook indicated the goal has not been met yet."
			}
			return GoalLoopOutcome{Continue: true, NextPrompt: next}
		}
		if o.cancelled {
			return GoalLoopOutcome{Err: context.Canceled}
		}
		if hookMandatory(h) && o.failed() {
			return GoalLoopOutcome{Err: fmt.Errorf("mandatory Stop validation failed: %s", o.failureReason())}
		}
		if hookMandatory(h) && !o.failed() {
			verified = true
		}
		warnHookFailure("Stop", h.Command, o)
	}
	return GoalLoopOutcome{Verified: verified}
}

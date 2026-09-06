package agentio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

// BeforeToolUseFunc is the shape of hand's existing approval-prompt
// hook (NewApprovalHook / NewOneShotApprovalHook) — the thing
// BuildLifecycleHooks composes its own PreToolUse hook commands with.
type BeforeToolUseFunc func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error)

// hookDenyExitCode is the exit code a hook command uses to signal
// "deny/abort" — matches Claude Code's own hooks convention, chosen so
// anyone already familiar with it can write a hand hook without
// re-reading a new contract. Any other nonzero exit (or a timeout)
// fails open: a broken hook script shouldn't brick every tool call.
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
	// both fail open, but with a different warning.
	timedOut bool
	spawnErr error
}

// runHookCommand runs cfg.Command/Args with env layered on top of the
// current process's environment, bounded by cfg.Timeout (or
// config.DefaultHookTimeoutSeconds when zero).
func runHookCommand(ctx context.Context, cfg config.HookConfig, env map[string]string) hookOutcome {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(config.DefaultHookTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return hookOutcome{timedOut: true, stdout: stdout.String(), stderr: stderr.String()}
	}
	if err == nil {
		return hookOutcome{exitCode: 0, stdout: stdout.String(), stderr: stderr.String()}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return hookOutcome{exitCode: exitErr.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	}
	return hookOutcome{spawnErr: err, stdout: stdout.String(), stderr: stderr.String()}
}

// warnHookFailure logs a non-blocking problem with a hook command —
// used for every failure mode except an explicit hookDenyExitCode,
// which is a deliberate decision, not a failure.
func warnHookFailure(event, command string, o hookOutcome) {
	switch {
	case o.timedOut:
		slog.Warn("hook timed out; continuing (fail open)", "event", event, "command", command)
	case o.spawnErr != nil:
		slog.Warn("hook failed to run; continuing (fail open)", "event", event, "command", command, "err", o.spawnErr)
	case o.exitCode != 0:
		slog.Warn("hook exited nonzero; continuing (fail open)", "event", event, "command", command, "exit_code", o.exitCode, "stderr", o.stderr)
	}
}

// runMatchingHooks runs every hooks entry whose Event matches event and
// whose Matcher matches toolName (toolName is ignored, so pass "" for
// events with no tool name), in order, until one returns
// hookDenyExitCode. Returns the denying outcome (with ok=true) if one
// did; otherwise ok is false and every hook either allowed or failed
// open.
func runMatchingHooks(ctx context.Context, hooks []config.HookConfig, event, toolName string, env map[string]string) (deny hookOutcome, denied bool) {
	for _, h := range hooks {
		if h.Event != event || !matchesHook(h.Matcher, toolName) {
			continue
		}
		o := runHookCommand(ctx, h, env)
		if !o.timedOut && o.spawnErr == nil && o.exitCode == hookDenyExitCode {
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
				return prompt, images, errors.New(o.stderr)
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
// outcome — an allowing exit 0, a failed-open nonzero exit or timeout,
// or no Stop hooks configured at all — means Continue is false and the
// caller should stop as normal, exactly like before this feature
// existed.
func EvaluateStopHooks(ctx context.Context, hooks []config.HookConfig, workspace, reason string, iteration int) GoalLoopOutcome {
	env := map[string]string{
		"HAND_HOOK_EVENT":     "Stop",
		"HAND_WORKSPACE":      workspace,
		"HAND_STOP_REASON":    reason,
		"HAND_GOAL_ITERATION": strconv.Itoa(iteration),
	}
	for _, h := range hooks {
		if h.Event != "Stop" {
			continue
		}
		o := runHookCommand(ctx, h, env)
		if !o.timedOut && o.spawnErr == nil && o.exitCode == hookDenyExitCode {
			next := strings.TrimSpace(o.stdout)
			if next == "" {
				next = strings.TrimSpace(o.stderr)
			}
			if next == "" {
				next = "Continue — the configured Stop hook indicated the goal has not been met yet."
			}
			return GoalLoopOutcome{Continue: true, NextPrompt: next}
		}
		warnHookFailure("Stop", h.Command, o)
	}
	return GoalLoopOutcome{}
}

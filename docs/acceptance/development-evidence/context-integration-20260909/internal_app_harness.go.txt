package app

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/sausheong/hand/internal/sessionio"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

// HarnessBackend translates the runtime stream into application-owned values.
// The runtime and stop-reason pointer must not be mutated outside the service
// while an operation is active.
type HarnessBackend struct {
	Runtime *runtime.Runtime
	Reason  *string
}

func (b *HarnessBackend) StopReason() string {
	if b.Reason == nil {
		return ""
	}
	return *b.Reason
}
func (b *HarnessBackend) Run(ctx context.Context, prompt string, images []llm.ImageContent) (<-chan BackendEvent, error) {
	if b.Reason != nil {
		*b.Reason = ""
	}
	ctx, runCancel, _, err := b.PrepareSelectedRunBudget(ctx)
	if err != nil {
		return nil, err
	}
	sess := b.Runtime.Session
	if owner, ok := ctx.Value(serviceContextKey{}).(*Service); ok {
		ctx = runtime.WithSteering(ctx, owner.steeringSource)
	}
	ctx, usageError := observeSessionUsage(ctx, sess)
	if err := usageError(); err != nil {
		runCancel()
		return nil, err
	}
	stream, err := b.Runtime.Run(ctx, prompt, images)
	if err != nil {
		runCancel()
		return nil, err
	}
	if stream == nil {
		runCancel()
		return nil, errors.New("Harness returned a nil stream")
	}
	events := make(chan BackendEvent)
	go func() {
		defer close(events)
		defer runCancel()
		for event := range stream {
			translated, ok := translateHarnessEvent(event)
			if !ok {
				continue
			}
			if translated.Err != nil {
				events <- translated
				continue
			}
			select {
			case events <- translated:
			case <-ctx.Done():
			}
		}
		persistenceErr := usageError()
		if owner, ok := ctx.Value(serviceContextKey{}).(*Service); ok {
			persistenceErr = errors.Join(persistenceErr, owner.reconcileSteering(sess))
		}
		summary, summaryErr := sessionio.ReadUsage(sess)
		if err := errors.Join(persistenceErr, summaryErr); err != nil {
			events <- BackendEvent{Err: err}
		} else {
			events <- BackendEvent{Kind: "session_usage", Details: Details{UsageKnown: true, InputTokens: summary.Total.InputTokens, OutputTokens: summary.Total.OutputTokens, CacheCreationInputTokens: summary.Total.CacheCreationInputTokens, CacheReadInputTokens: summary.Total.CacheReadInputTokens, UsageRequests: summary.Requests, UsageUnknown: summary.Unknown, UsagePriorUnknown: summary.PriorUsageUnknown}}
		}

	}()
	return events, nil
}

func NewHarness(runtime *runtime.Runtime, reason *string, hooks []config.HookConfig, workspace string, maxIterations int, profiles ...config.ModelProfile) *Service {
	return NewHarnessWithAttachments(runtime, reason, hooks, workspace, maxIterations, nil, profiles...)
}

func NewHarnessWithAttachments(runtime *runtime.Runtime, reason *string, hooks []config.HookConfig, workspace string, maxIterations int, policy *agentio.AttachmentPolicy, profiles ...config.ModelProfile) *Service {
	sessionID := rand.Text()
	if runtime.Session != nil {
		sessionID = runtime.Session.ID
	}
	p := config.ModelProfile{}
	if len(profiles) > 0 {
		p = profiles[0]
	}
	return New(&HarnessBackend{Runtime: runtime, Reason: reason}, Options{ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(agentio.WithAttachmentPolicy(ctx, policy), workspace, text)
	}, Model: runtimeModelInfo(runtime, p), InputTypes: p.InputTypes, SessionID: sessionID, MaxIterations: maxIterations,
		Check: func(ctx context.Context, reason string, iteration int) agentio.GoalLoopOutcome {
			return agentio.EvaluateStopHooks(ctx, hooks, workspace, reason, iteration)
		},
	})
}

// Translation copies mutable runtime payloads before publishing them. All
// variable-size fields are bounded again at the application publication edge.
func translateHarnessEvent(event runtime.AgentEvent) (BackendEvent, bool) {
	e := BackendEvent{}
	switch event.Type {
	case runtime.EventTextDelta:
		e.Text = event.Text
	case runtime.EventRequestUsage:
		e.Kind = "context_usage"
		if event.RequestUsage != nil && event.RequestUsage.Usage != nil {
			e.Details.UsageKnown = true
			e.Details.InputTokens = event.RequestUsage.Usage.InputTokens
		}
	case runtime.EventDone:
		e.Done = true
		e.Kind = "usage"
		if u := event.Usage; u != nil {
			e.Details.UsageKnown = true
			e.Details.InputTokens = u.InputTokens
			e.Details.OutputTokens = u.OutputTokens
			e.Details.CacheCreationInputTokens = u.CacheCreationInputTokens
			e.Details.CacheReadInputTokens = u.CacheReadInputTokens
		}
	case runtime.EventError:
		e.Err = event.Error
		if e.Err == nil {
			e.Err = errors.New("runtime emitted an error without details")
		}
	case runtime.EventAborted:
		e.Err = errors.New("runtime aborted the turn")
	case runtime.EventToolCallStart:
		e.Kind = "tool_call"
	case runtime.EventToolResult:
		e.Kind = "tool_result"
	case runtime.EventCompactionStart:
		e.Kind = "compaction_start"
	case runtime.EventCompactionDone:
		e.Kind = "compaction_done"
	case runtime.EventCompactionSkipped:
		e.Kind = "compaction_skipped"
	default:
		return e, false
	}
	if t := event.ToolCall; t != nil {
		e.Details.ToolPresent = true
		e.Details.ToolID, e.Details.ToolName, e.Details.ToolInput = t.ID, t.Name, string(t.Input)
	}
	if r := event.Result; r != nil {
		e.Details.ResultPresent = true
		e.Details.Output, e.Details.ToolError = r.Output, r.Error
		e.Details.ImageCount = len(r.Images)
		if r.Metadata != nil {
			encoded, err := json.Marshal(r.Metadata)
			if err != nil {
				e.Details.Metadata = "metadata unavailable: " + err.Error()
			} else {
				e.Details.Metadata = string(encoded)
			}
		}
	}
	if c := event.Compaction; c != nil {
		e.Details.CompactionPresent, e.Details.Compacted = true, c.Compacted
		e.Details.CompactionReason, e.Details.Skipped, e.Details.Summary = string(c.Reason), c.Skipped, c.Summary
		e.Details.TurnsCompacted, e.Details.TokensBefore, e.Details.TokensAfter = c.TurnsCompacted, c.TokensBefore, c.TokensAfter
		e.Details.DurationMs = c.DurationMs
	}
	e.Details = boundDetails(e.Details)
	return e, true
}

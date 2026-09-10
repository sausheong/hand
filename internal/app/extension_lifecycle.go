package app

import (
	"context"
	"encoding/json"
	"github.com/sausheong/harness/compaction"
	"log/slog"
)

func (h *ExtensionHost) notifyLifecycle(ctx context.Context, event string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	diagnostics, err := h.manager.NotifyLifecycle(ctx, event, raw)
	for _, diagnostic := range diagnostics {
		slog.Warn("extension lifecycle delivery failed", "extension", diagnostic.ID, "event", event, "error", diagnostic.Error)
	}
	return err
}
func (h *ExtensionHost) attachRuntimeLifecycle() {
	rt := h.controller.Rt
	if rt == nil {
		return
	}
	start, finish := rt.AgentLoop.Hooks.OnRunStart, rt.AgentLoop.Hooks.OnRunFinish
	rt.AgentLoop.Hooks.OnRunStart = func(ctx context.Context) error {
		if start != nil {
			if err := start(ctx); err != nil {
				return err
			}
		}
		return h.notifyLifecycle(ctx, "run.start", map[string]string{"session_id": h.controller.SessionID()})
	}
	rt.AgentLoop.Hooks.OnRunFinish = func(ctx context.Context, reason string) {
		defer func() {
			if err := h.notifyLifecycle(ctx, "run.finish", map[string]string{"session_id": h.controller.SessionID(), "reason": reason}); err != nil {
				slog.Warn("extension finish observer failed", "error", err)
			}
		}()
		if finish != nil {
			finish(ctx, reason)
		}
	}
}

func (h *ExtensionHost) attachCompactionLifecycle() {
	rt := h.controller.Rt
	if rt == nil || rt.Compaction == nil {
		return
	}
	previous := rt.Compaction.OnCommitted
	rt.Compaction.OnCommitted = func(ctx context.Context, event compaction.CommitNotification) {
		defer func() {
			if h.closing.Load() {
				return
			}
			delivery := context.WithValue(ctx, sessionNotificationKey{}, "context.compacted")
			if err := h.notifyLifecycle(delivery, "context.compacted", map[string]any{"session_id": event.SessionID, "reason": string(event.Reason), "turns_compacted": event.TurnsCompacted, "tokens_before": event.TokensBefore, "tokens_after": event.TokensAfter}); err != nil {
				slog.Warn("extension compaction observer failed", "error", err)
			}
		}()
		if previous != nil {
			previous(ctx, event)
		}
	}
}

package app

import (
	"context"
	"log/slog"
	"time"
)

type sessionNotificationKey struct{}

// observeSessionSelection runs after the session mutex is released, while the
// application still owns the idle operation. Notifications describe committed
// transitions and never roll back a selected session on observer failure.
func (c *Controller) observeSessionSelection(ctx context.Context, previous string) {
	current := c.SessionID()
	if current == previous || c.Extensions == nil || c.Extensions.closing.Load() {
		return
	}
	c.Extensions.observeSessionTransition(ctx, previous, current)
}
func (h *ExtensionHost) observeSessionTransition(parent context.Context, previous, current string) {
	h.sessionObserved.Store(current != "")
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	for _, event := range []struct{ name, id string }{{"session.close", previous}, {"session.open", current}} {
		if event.id == "" {
			continue
		}
		delivery := context.WithValue(ctx, sessionNotificationKey{}, event.name)
		if err := h.notifyLifecycle(delivery, event.name, map[string]string{"session_id": event.id, "selected_session_id": current}); err != nil {
			slog.Warn("extension session observer failed", "event", event.name, "error", err)
		}
	}
}

package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/session"
)

// The getter is used after the caller has joined its provider attempts.
// Background callers must separately join their producers before treating it
// as final. Observation is chained with existing Harness usage observers.
func observeSessionUsage(ctx context.Context, sess *session.Session) (context.Context, func() error) {
	var mu sync.Mutex
	first := sessionio.BeginUsageTracking(sess)
	ctx = llm.WithUsageObserver(ctx, func(record llm.RequestUsage) {
		// Foreground and compaction callbacks can overlap. Keep validation and
		// durable append in one transaction under the run owner.
		mu.Lock()
		defer mu.Unlock()
		if err := sessionio.RecordRequestUsage(sess, record); err != nil {
			if first == nil {
				first = fmt.Errorf("persist request usage: %w", err)
			}
		}
	})
	return ctx, func() error { mu.Lock(); defer mu.Unlock(); return first }
}

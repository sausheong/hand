package app

import (
	"context"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestToolOutputIdentityAndCompleteResult(t *testing.T) {
	s := session.NewSession("hand", "key")
	raw := strings.Repeat("full result\n", 20000)
	s.Append(session.ToolResultEntry("call", raw, "partial error", nil))
	c := &Controller{Rt: &runtime.Runtime{Session: s}}
	result, err := c.ToolOutput(context.Background(), s.ID, "call")
	if err != nil || result.Output != raw || result.Error != "partial error" {
		t.Fatal("full result unavailable", err)
	}
	for _, ids := range [][2]string{{"wrong", "call"}, {s.ID, "missing"}, {s.ID, ""}} {
		if _, err := c.ToolOutput(context.Background(), ids[0], ids[1]); err == nil {
			t.Fatal("invalid identity accepted", ids)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ToolOutput(ctx, s.ID, "call"); err != context.Canceled {
		t.Fatal(err)
	}
	s.Append(session.ToolResultEntry("call", "conflict", "", nil))
	if _, err := c.ToolOutput(context.Background(), s.ID, "call"); err == nil {
		t.Fatal("duplicate tool identity silently selected")
	}
}

func TestToolOutputCancellationWhileControllerLocked(t *testing.T) {
	c := &Controller{}
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{})
	observed := &lockWaitContext{Context: ctx, waiting: waiting}
	done := make(chan error, 1)
	go func() { _, err := c.ToolOutput(observed, "session", "tool"); done <- err }()
	<-waiting
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked on controller lock")
	}
}

type lockWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *lockWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

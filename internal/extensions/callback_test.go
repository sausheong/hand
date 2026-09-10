//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sausheong/hand/extension/protocol"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallbacksRequireCapabilityAndAreBoundToRequest(t *testing.T) {
	for _, mode := range []string{"state-normal", "state-denied", "state-duplicate", "state-excess", "state-policy"} {
		t.Run(mode, func(t *testing.T) {
			c := fixtureConnection(t, mode)
			if _, err := c.Initialize(context.Background(), "fixture", []string{"commands", "state"}); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			if err := c.SetCallbackHandler(func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
				calls.Add(1)
				if method != "state.get" {
					t.Error("unapproved method reached handler", method)
				}
				if mode == "state-policy" {
					return nil, &protocol.Error{Code: "policy_denied", Message: "host policy refused"}
				}
				return json.RawMessage(`{"saved":true}`), nil
			}); err != nil {
				t.Fatal(err)
			}
			result, err := c.Call(context.Background(), "state", json.RawMessage(`{}`))
			switch mode {
			case "state-normal":
				if err != nil || string(result) != `{"saved":true}` || calls.Load() != 1 {
					t.Fatal(string(result), err, calls.Load())
				}
			case "state-denied":
				var remote *ResponseError
				if !errors.As(err, &remote) || remote.Code != "capability_denied" || calls.Load() != 0 {
					t.Fatal(err, calls.Load())
				}
			case "state-excess":
				if err == nil || calls.Load() != 32 {
					t.Fatal("callback limit not enforced", err, calls.Load())
				}
			case "state-policy":
				var remote *ResponseError
				if !errors.As(err, &remote) || remote.Code != "policy_denied" || calls.Load() != 1 {
					t.Fatal("host policy error lost", err, calls.Load())
				}
			case "state-duplicate":
				if err == nil || calls.Load() != 1 {
					t.Fatal("duplicate callback executed", err, calls.Load())
				}
			}
		})
	}
}
func TestCallbackCancellationJoinsHostHandler(t *testing.T) {
	c := fixtureConnection(t, "state-normal")
	if _, err := c.Initialize(context.Background(), "fixture", []string{"commands", "state"}); err != nil {
		t.Fatal(err)
	}
	started, joined := make(chan struct{}), make(chan struct{})
	if err := c.SetCallbackHandler(func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, *protocol.Error) {
		close(started)
		<-ctx.Done()
		close(joined)
		return nil, &protocol.Error{Code: "cancelled", Message: "cancelled"}
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Call(ctx, "state", json.RawMessage(`{}`)); done <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not started")
	}
	if err := c.SetCallbackHandler(nil); err == nil {
		t.Fatal("active callback handler replaced")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callback cancellation not joined")
	}
	select {
	case <-joined:
	default:
		t.Fatal("handler survived call")
	}
}

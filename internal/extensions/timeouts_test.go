//go:build unix

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
)

func TestInteractiveDeadlineFlowsThroughManagerAndConnection(t *testing.T) {
	for _, mode := range []string{"command", "run.start", "run.finish", "policy", "parent-deadline"} {
		t.Run(mode, func(t *testing.T) {
			observed := make(chan time.Duration, 1)
			joined := make(chan struct{})
			m, err := NewManager(context.Background(), func(operation, lifetime context.Context, spec Specification) (*Connection, error) {
				c := fixtureConnection(t, "question-deadline")
				err := c.SetCallbackHandler(func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, *protocol.Error) {
					deadline, ok := ctx.Deadline()
					if !ok {
						observed <- -1
					} else {
						observed <- time.Until(deadline)
					}
					<-ctx.Done()
					close(joined)
					return nil, &protocol.Error{Code: "cancelled", Message: "cancelled"}
				})
				return c, err
			})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if _, err = m.Reload(context.Background(), []Specification{{Name: "fixture", Digest: strings.Repeat("a", 64), Capabilities: []string{"commands", "questions", "lifecycle"}, Mandatory: true}}); err != nil {
				t.Fatal(err)
			}
			parent := context.Background()
			if mode == "parent-deadline" {
				var stop context.CancelFunc
				parent, stop = context.WithTimeout(parent, 10*time.Second)
				defer stop()
			}
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				switch mode {
				case "command", "parent-deadline":
					_, err = m.ExecuteCommand(ctx, "fixture", "inspect", "")
				case "policy":
					_, err = m.Call(ctx, "fixture", "policy.check", json.RawMessage(`{}`))
				default:
					_, err = m.NotifyLifecycle(ctx, mode, json.RawMessage(`{}`))
				}
				done <- err
			}()
			var remaining time.Duration
			select {
			case remaining = <-observed:
			case <-time.After(3 * time.Second):
				t.Fatal("question not delivered")
			}
			ceiling := protocolTimeout
			if mode == "command" || mode == "run.start" {
				ceiling = interactiveTimeout
			}
			if mode == "parent-deadline" {
				ceiling = 10 * time.Second
			}
			if remaining > ceiling || remaining < ceiling-3*time.Second {
				t.Fatalf("callback deadline %s, want near %s", remaining, ceiling)
			}
			cancel()
			select {
			case err = <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation failed to join")
			}
			select {
			case <-joined:
			default:
				t.Fatal("callback outlived operation")
			}
			if _, err = m.Reload(context.Background(), nil); err != nil {
				t.Fatalf("cancellation retained registry ownership: %v", err)
			}
		})
	}
}

package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/runtime"
)

func TestSessionCancellationWhileWaitingForControllerLock(t *testing.T) {
	for _, name := range []string{"resume-current", "new", "fork", "export", "rename", "tree", "select"} {
		t.Run(name, func(t *testing.T) {
			manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
			if err != nil {
				t.Fatal(err)
			}
			selected, err := manager.Open(context.Background(), "", false)
			if err != nil {
				t.Fatal(err)
			}
			c := &Controller{Sessions: manager, Rt: &runtime.Runtime{AgentID: "hand", Session: selected.Session}, SessionKey: selected.Record.StoreKey}
			defer func() { _ = c.Rt.Session.Close() }()
			before, err := manager.Catalogue().Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			destination := filepath.Join(t.TempDir(), "cancelled.jsonl")
			done := make(chan error, 1)
			c.mu.Lock()
			locked := true
			defer func() {
				if locked {
					c.mu.Unlock()
				}
			}()
			owner := c.owner()
			go func() {
				switch name {
				case "resume-current":
					done <- c.ResumeSessionContext(ctx, selected.Session.ID)
				case "new":
					done <- c.NewSessionContext(ctx)
				case "fork":
					done <- c.ForkSession(ctx)
				case "export":
					done <- c.ExportSession(ctx, destination)
				case "rename":
					done <- c.RenameSessionContext(ctx, "cancelled")
				case "tree":
					_, err := c.SessionTree(ctx)
					done <- err
				case "select":
					done <- c.SelectSessionNode(ctx, "missing")
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				owner.mu.Lock()
				active := owner.active
				owner.mu.Unlock()
				if active {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("operation did not reserve ownership")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			c.mu.Unlock()
			locked = false
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v, want context cancellation", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancelled operation failed to join")
			}
			after, err := manager.Catalogue().Snapshot()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("cancelled catalogue changed: %v", err)
			}
			if c.Rt.Session != selected.Session || c.SessionKey != selected.Record.StoreKey {
				t.Fatal("cancelled selection changed")
			}
			if _, release, err := owner.reserve(context.Background(), Idle); err != nil {
				t.Fatal("ownership leaked", err)
			} else {
				release()
			}
		})
	}
}

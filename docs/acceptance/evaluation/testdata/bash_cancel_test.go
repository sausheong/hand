package bash_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/harness/tools/bash"
)

// The oracle uses the public API shared by the defective and fixed revisions.
// A child signals readiness before cancellation; it must never publish late.
func TestAcceptanceBashCancellationOwnsDescendants(t *testing.T) {
	workspace := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, err := json.Marshal(map[string]any{"command": "(printf ready > ready; sleep 2; printf escaped > late) & wait"})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan string, 1)
	go func() {
		result, err := (&bash.BashTool{WorkDir: workspace}).Execute(ctx, input)
		finished <- fmt.Sprint(err) + result.Error
	}()
	// Join even a defective implementation before t.TempDir removes its files.
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-finished:
			case <-time.After(4 * time.Second):
				t.Error("cancelled command did not join")
			}
		}
	}()
	readyDeadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(workspace, "ready")); err == nil {
			break
		}
		if time.Now().After(readyDeadline) {
			t.Fatal("child never became ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case outcome := <-finished:
		joined = true
		if outcome == "<nil>" {
			t.Error("cancelled tool reported success")
		}
	case <-time.After(time.Second):
		t.Error("cancellation did not join within one second")
	}
	// Observe beyond the child's planned write, so early parent exit alone
	// cannot satisfy the oracle.
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(workspace, "late")); !os.IsNotExist(err) {
		t.Fatal("descendant published a file after cancellation", err)
	}
}

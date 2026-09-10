//go:build unix

package agentio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/config"
)

func TestHookCancellationOwnsDescendants(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan hookOutcome, 1)
	go func() {
		finished <- runHookCommand(ctx, config.HookConfig{Event: "PreToolUse", Command: "sh", Args: []string{"-c", `(printf ready > "$1/ready"; sleep 1; printf escaped > "$1/late") & wait`, "hook", root}}, map[string]string{"HAND_HOOK_EVENT": "PreToolUse"})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-finished
			t.Fatal("hook child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	start := time.Now()
	cancel()
	select {
	case result := <-finished:
		if !result.cancelled {
			t.Fatalf("hook cancellation not reported: %+v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled hook did not join")
	}
	t.Logf("hook_cancel_join_ms=%f", float64(time.Since(start).Nanoseconds())/1e6)
	// Observe beyond the child's scheduled write; parent exit alone is not proof.
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "late")); !os.IsNotExist(err) {
		t.Fatalf("hook descendant survived cancellation: %v", err)
	}
}

func TestHookNormalExitCleansDescendants(t *testing.T) {
	root := t.TempDir()
	result := runHookCommand(context.Background(), config.HookConfig{
		Event: "PreToolUse", Command: "sh", Timeout: 3,
		Args: []string{"-c", `(printf ready > "$1/ready"; sleep 1; printf escaped > "$1/late") >/dev/null 2>&1 &
while [ ! -f "$1/ready" ]; do sleep 0.01; done
exit 0`, "hook", root},
	}, map[string]string{"HAND_HOOK_EVENT": "PreToolUse"})
	if result.exitCode != 0 || result.spawnErr != nil || result.cancelled || result.timedOut {
		t.Fatalf("normal hook outcome changed: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "ready")); err != nil {
		t.Fatalf("descendant did not start: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "late")); !os.IsNotExist(err) {
		t.Fatalf("hook descendant survived normal exit: %v", err)
	}
}

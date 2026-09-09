package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestExtensionSessionEventsFollowCommittedTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	workspace := t.TempDir()
	manager, err := sessionio.NewManager(t.TempDir(), workspace, "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	c := &Controller{Rt: &runtime.Runtime{AgentID: "hand", Session: selected.Session}, Sessions: manager, SessionKey: selected.Record.StoreKey}
	defer func() { c.Rt.Session.Close() }()
	original := c.SessionID()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "events.jsonl")
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "lifecycle-fixture", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestLifecyclePeerProcess$"}, Environment: map[string]string{"HAND_LIFECYCLE_PEER": "sessions", "HAND_LIFECYCLE_LOG": log}, Capabilities: []string{"lifecycle"}})
	if err != nil {
		t.Fatal(err)
	}
	host, err := c.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "snapshots"), Identities: map[string]string{"lifecycle-fixture": "fixture/sessions"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if err = c.ResumeSessionContext(ctx, "missing"); err == nil {
		t.Fatal("missing session selected")
	}
	if err = c.ResumeSessionContext(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err = c.NewSessionContext(ctx); err != nil {
		t.Fatal(err)
	}
	next := c.SessionID()
	if err = c.ResumeSessionContext(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err = c.ForkSession(ctx); err != nil {
		t.Fatal(err)
	}
	fork := c.SessionID()
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	expected := []struct{ event, id, selected string }{{"session.open", original, original}, {"session.close", original, next}, {"session.open", next, next}, {"session.close", next, original}, {"session.open", original, original}, {"session.close", original, fork}, {"session.open", fork, fork}, {"session.close", fork, ""}}
	if len(lines) != len(expected) {
		t.Fatal("unexpected notifications", string(raw))
	}
	for i, line := range lines {
		var event struct {
			Event string            `json:"event"`
			Data  map[string]string `json:"data"`
		}
		if err = json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		want := expected[i]
		if event.Event != want.event || event.Data["session_id"] != want.id || event.Data["selected_session_id"] != want.selected {
			t.Fatal(i, event, want)
		}
	}
}

func TestExtensionSessionShutdownBoundsHungObserver(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	log := filepath.Join(t.TempDir(), "events.jsonl")
	snapshots := filepath.Join(t.TempDir(), "snapshots")
	sess := session.NewSession("hand", "shutdown")
	defer sess.Close()
	c := &Controller{Rt: &runtime.Runtime{Session: sess}}
	review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "lifecycle-fixture", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestLifecyclePeerProcess$"}, Environment: map[string]string{"HAND_LIFECYCLE_PEER": "sessions-hang-close", "HAND_LIFECYCLE_LOG": log}, Capabilities: []string{"lifecycle"}})
	if err != nil {
		t.Fatal(err)
	}
	host, err := c.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: snapshots, Identities: map[string]string{"lifecycle-fixture": "fixture/shutdown"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	firstErr := host.Close()
	if time.Since(started) > 4*time.Second {
		t.Fatal("hung observer delayed shutdown")
	}
	if secondErr := host.Close(); (firstErr == nil) != (secondErr == nil) {
		t.Fatal("close result changed", firstErr, secondErr)
	}
	raw, err := os.ReadFile(log)
	if err != nil || strings.Count(string(raw), `"event":"session.close"`) != 1 {
		t.Fatal(string(raw), err)
	}
	entries, err := os.ReadDir(snapshots)
	if err != nil || len(entries) != 0 {
		t.Fatal("snapshot leaked", entries, err)
	}
	if _, err = host.Execute(ctx, "lifecycle-fixture", "anything", ""); err == nil {
		t.Fatal("closed host accepted command")
	}
}

func TestExtensionCompactionNotificationFollowsCommit(t *testing.T) {
	for _, mode := range []string{"manual", "background"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sess := session.NewSession("hand", "compaction-event")
			defer sess.Close()
			for i := 0; i < 10; i++ {
				sess.Append(session.UserMessageEntry("task"))
				sess.Append(session.AssistantMessageEntry("work"))
			}
			c := &Controller{Rt: &runtime.Runtime{Session: sess, Compaction: &compaction.Manager{Summarizer: &compaction.Summarizer{Provider: &focusProvider{}, Model: "fixture", MaxOutputTokens: 1024}, PreserveTurns: 2}}}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			log := filepath.Join(t.TempDir(), "events.jsonl")
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "lifecycle-fixture", Executable: exe, Workspace: workspace, Arguments: []string{"-test.run=^TestLifecyclePeerProcess$"}, Environment: map[string]string{"HAND_LIFECYCLE_PEER": "compaction", "HAND_LIFECYCLE_LOG": log}, Capabilities: []string{"lifecycle"}})
			if err != nil {
				t.Fatal(err)
			}
			host, err := c.ActivateExtensions(ctx, ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "snapshots"), Identities: map[string]string{"lifecycle-fixture": "fixture/compaction"}, Reviews: []extensions.LaunchReview{review}}, workspace, true)
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			var result compaction.Result
			expectedReason := "manual"
			if mode == "background" {
				expectedReason = "preventive"
				c.Rt.Compaction.MaybeCompactAsyncContext(ctx, sess, compaction.ReasonPreventive)
				var joined bool
				result, joined, err = c.Rt.Compaction.JoinInFlight(ctx, sess)
				if !joined {
					t.Fatal("background result not joined")
				}
			} else {
				result, err = c.Compact(ctx)
			}
			if err != nil || !result.Compacted {
				t.Fatal(result, err)
			}
			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(raw), "\n") != 1 {
				t.Fatal(string(raw))
			}
			var event struct {
				Event string `json:"event"`
				Data  struct {
					SessionID string `json:"session_id"`
					Reason    string `json:"reason"`
					Turns     int    `json:"turns_compacted"`
				} `json:"data"`
			}
			if err = json.Unmarshal(raw, &event); err != nil || event.Event != "context.compacted" || event.Data.SessionID != sess.ID || event.Data.Reason != expectedReason || event.Data.Turns != result.TurnsCompacted {
				t.Fatal(event, err)
			}

		})
	}
}

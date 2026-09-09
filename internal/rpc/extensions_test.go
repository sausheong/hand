package rpc

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	extensionprotocol "github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

func TestExtensionRPCQuestionReplayAndDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := os.Getenv("HAND_TEST_RPC_NOTE_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "note")
		build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
		build.Dir = "../.."
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %v %s", err, out)
		}
	} else if !filepath.IsAbs(binary) {
		t.Fatal("HAND_TEST_RPC_NOTE_BINARY must be absolute")
	}

	for _, mode := range []string{"answer", "cancel", "disconnect", "journal_failure"} {
		t.Run(mode, func(t *testing.T) {
			sess := session.NewSession("hand", "rpc-note")
			defer sess.Close()
			c := &app.Controller{Rt: &runtime.Runtime{Session: sess}, Owner: app.New(nil, app.Options{SessionID: "rpc-note"})}
			review, err := extensions.ReviewHostLaunch(ctx, extensions.LaunchConfig{Name: "task-note", Executable: binary, Workspace: t.TempDir(), Capabilities: []string{"commands", "questions", "state", "context.transform"}})
			if err != nil {
				t.Fatal(err)
			}
			factory, err := extensions.NewHostFactory(filepath.Join(t.TempDir(), "private"), []extensions.LaunchReview{review}, func(context.Context, extensions.LaunchReview) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			host, err := app.NewExtensionHost(ctx, c, factory, map[string]string{"task-note": "example/note"})
			if err != nil {
				t.Fatal(err)
			}
			defer host.Close()
			c.Extensions = host
			if _, err = host.Reload(ctx, []extensions.Specification{review.Specification}); err != nil {
				t.Fatal(err)
			}
			ledgerFile := ledgerPath(t)
			ledger, err := OpenLedger(ledgerFile)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			d := NewControllerDispatcher(c, ledger)
			defer d.Close()
			if response := d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`)); response.Error != nil {
				t.Fatal(response.Error)
			}
			request := rpcRequest("command", "extension.command", `{"name":"task-note","command":"note","arguments":""}`)
			if response := d.Dispatch(ctx, request); response.Error != nil {
				t.Fatal(response.Error)
			}
			var pending []extensions.PendingQuestion
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for len(pending) == 0 {
				response := d.Dispatch(ctx, rpcRequest("questions", "extension.questions", `{}`))
				if response.Error != nil {
					t.Fatal(response.Error)
				}
				if err = json.Unmarshal(response.Result, &pending); err != nil {
					t.Fatal(err)
				}
				if len(pending) == 0 {
					select {
					case <-ctx.Done():
						t.Fatal("question timeout")
					case <-ticker.C:
					}
				}
			}
			if mode == "journal_failure" {
				if err := ledger.file.Close(); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				response := d.Dispatch(ctx, rpcRequest("emergency", "cancel", `{}`))
				if response.Error == nil || response.Error.Code != "ledger_failure" || len(response.Result) != 0 {
					t.Fatal("undurable extension cancellation reported success", response)
				}
				joined := make(chan struct{})
				go func() { d.wg.Wait(); close(joined) }()
				select {
				case <-joined:
				case <-time.After(5 * time.Second):
					t.Fatal("extension command did not join after journal failure")
				}
				if len(host.Pending()) != 0 {
					t.Fatal("cancelled extension question retained")
				}
				state, err := extensions.NewStateStore(sess, "example/note")
				if err != nil {
					t.Fatal(err)
				}
				saved, err := state.Get(ctx)
				if err != nil || saved.Revision != 0 {
					t.Fatal("cancelled command committed state", saved, err)
				}
				d.mu.Lock()
				active, paused, failure := d.extensionCancel != nil, d.autoPaused, d.failure
				d.mu.Unlock()
				if active || !paused || failure == "" {
					t.Fatal("command ownership or failure status incorrect")
				}
				t.Logf("journal_failure_extension_join_ms=%f", float64(time.Since(start).Nanoseconds())/1e6)
				d.Close()
				ledger.Close()
				reopened, err := OpenLedger(ledgerFile)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				record, err := reopened.Lookup(request.ID)
				if err != nil || record.State != "uncertain" || len(record.Result) != 0 {
					t.Fatal("undurable command became completion", record, err)
				}
				next := NewControllerDispatcher(c, reopened)
				defer next.Close()
				next.Dispatch(ctx, rpcRequest("h2", "hello", `{}`))
				replay := next.Dispatch(ctx, request)
				var replayRecord RequestRecord
				if replay.Error != nil || json.Unmarshal(replay.Result, &replayRecord) != nil || replayRecord.State != "uncertain" {
					t.Fatal("uncertain command replay changed", replay)
				}
				next.mu.Lock()
				active = next.extensionCancel != nil
				next.mu.Unlock()
				if active || len(host.Pending()) != 0 {
					t.Fatal("uncertain command dispatched again")
				}
				return
			}
			switch mode {
			case "answer":
				params, _ := json.Marshal(map[string]any{"token": pending[0].Token, "answer": extensionprotocol.Answer{ID: pending[0].Question.ID, Text: "RPC note"}})
				answer := rpcRequest("answer", "extension.answer", string(params))
				first := d.Dispatch(ctx, answer)
				if first.Error != nil {
					t.Fatal(first.Error)
				}
				replay := d.Dispatch(ctx, answer)
				if replay.Error != nil || string(replay.Result) != string(first.Result) {
					t.Fatal("answer replay changed result")
				}
			case "cancel":
				if response := d.Dispatch(ctx, rpcRequest("cancel", "cancel", `{}`)); response.Error != nil {
					t.Fatal(response.Error)
				}
			case "disconnect":
				d.Close()
			}
			var record RequestRecord
			for {
				record, err = ledger.Lookup(request.ID)
				if err != nil {
					t.Fatal(err)
				}
				if record.State == "completed" {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("completion timeout")
				case <-ticker.C:
				}
			}
			var response protocol.Response
			if err = json.Unmarshal(record.Result, &response); err != nil {
				t.Fatal(err)
			}
			if (response.Error == nil) != (mode == "answer") {
				t.Fatalf("terminal response: %+v", response)
			}
			if len(host.Pending()) != 0 {
				t.Fatal("question leaked after completion")
			}
			state, _ := extensions.NewStateStore(sess, "example/note")
			saved, err := state.Get(ctx)
			if err != nil {
				t.Fatal(err)
			}
			expected := 0
			if mode == "answer" {
				expected = 1
			}
			if saved.Revision != expected {
				t.Fatalf("unexpected state revision %d", saved.Revision)
			}
			if mode != "disconnect" {
				if replay := d.Dispatch(ctx, request); replay.Error != nil {
					t.Fatal(replay.Error)
				}
				if len(host.Pending()) != 0 {
					t.Fatal("command replay executed again")
				}
			}
		})
	}
}

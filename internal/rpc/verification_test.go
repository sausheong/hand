//go:build unix

package rpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/process"
)

func TestVerificationRPCAsyncCompletionAndCancel(t *testing.T) {
	for _, mode := range []string{"complete", "cancel", "disconnect", "journal_failure"} {
		t.Run(mode, func(t *testing.T) {
			cancelRun := mode != "complete"
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			work := t.TempDir()
			output, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "output"))
			if err != nil {
				t.Fatal(err)
			}
			processes, err := app.NewProcesses(ctx, work, output)
			if err != nil {
				t.Fatal(err)
			}
			store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "checkpoints"), work, checkpoints.DefaultStoreLimits())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			owner := app.New(&queuedBackend{}, app.Options{SessionID: "session", RunBoundary: &app.WorkspaceCheckpoints{Workspace: work, Store: store, Limits: checkpoints.DefaultLimits(), Processes: processes}})
			controller := &app.Controller{Owner: owner, Processes: processes, OutputStore: output}
			evidence := t.TempDir()
			os.Chmod(evidence, 0700)
			command := "printf verified"
			if cancelRun {
				command = `printf %s "$$" > marker; exec sleep 30`
			}
			if err := controller.ConfigureVerification(ctx, config.VerificationConfig{Directory: evidence, Profiles: []config.VerificationProfile{{Name: "test", Command: []string{"/bin/sh", "-c", command}}}}); err != nil {
				t.Fatal(err)
			}
			ledgerFile := ledgerPath(t)
			ledger, err := OpenLedger(ledgerFile)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			d := NewControllerDispatcher(controller, ledger)
			defer d.Close()
			d.Dispatch(ctx, rpcRequest("hello", "hello", `{}`))
			views, err := controller.VerificationProfiles(ctx)
			if err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"confirmed": true, "profile": "test", "digest": views[0].Digest})
			request := rpcRequest("verify", "verification.run", string(params))
			if r := d.Dispatch(ctx, request); r.Error != nil {
				t.Fatal(r.Error)
			}
			if cancelRun {
				for {
					if _, err := os.Stat(filepath.Join(work, "marker")); err == nil {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("command did not start")
					case <-time.After(time.Millisecond):
					}
				}
				if mode == "journal_failure" {
					if err := ledger.file.Close(); err != nil {
						t.Fatal(err)
					}
					start := time.Now()
					response := d.Dispatch(ctx, rpcRequest("emergency", "cancel", `{}`))
					if response.Error == nil || response.Error.Code != "ledger_failure" || len(response.Result) != 0 {
						t.Fatal("undurable cancellation reported success", response)
					}
					joined := make(chan struct{})
					go func() { d.wg.Wait(); close(joined) }()
					select {
					case <-joined:
					case <-time.After(5 * time.Second):
						t.Fatal("journal failure prevented verification cleanup")
					}
					raw, err := os.ReadFile(filepath.Join(work, "marker"))
					if err != nil {
						t.Fatal(err)
					}
					pid, err := strconv.Atoi(string(raw))
					if err != nil {
						t.Fatal(err)
					}
					if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
						t.Fatal("verification process survived emergency cancellation", pid, err)
					}
					elapsed := time.Since(start)
					if elapsed > 5*time.Second {
						t.Fatal("verification cleanup exceeded five seconds", elapsed)
					}
					t.Logf("journal_failure_verification_cleanup_ms=%f", float64(elapsed.Nanoseconds())/1e6)
					var state struct {
						Failure string `json:"failure"`
						Paused  bool   `json:"followup_paused"`
					}
					status := d.Dispatch(ctx, rpcRequest("state", "state", `{}`))
					if status.Error != nil || json.Unmarshal(status.Result, &state) != nil || !state.Paused || state.Failure == "" {
						t.Fatal("storage failure not disclosed", status)
					}
					d.Close()
					ledger.Close()
					reopened, err := OpenLedger(ledgerFile)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					record, err := reopened.Lookup("verify")
					if err != nil || record.State != "uncertain" || len(record.Result) != 0 {
						t.Fatal("failed verification receipt became completion", record, err)
					}
					next := NewControllerDispatcher(controller, reopened)
					defer next.Close()
					next.Dispatch(ctx, rpcRequest("hello-recovered", "hello", `{}`))
					replay := next.Dispatch(ctx, request)
					var replayRecord RequestRecord
					if replay.Error != nil || json.Unmarshal(replay.Result, &replayRecord) != nil || replayRecord.State != "uncertain" {
						t.Fatal("uncertain verification replay changed", replay)
					}
					next.mu.Lock()
					active := next.verificationCancel != nil
					next.mu.Unlock()
					if active {
						t.Fatal("uncertain verification dispatched again")
					}
					return
				}
				if mode == "disconnect" {
					joined := make(chan struct{})
					go func() { d.Close(); close(joined) }()
					select {
					case <-joined:
					case <-time.After(5 * time.Second):
						t.Fatal("disconnect failed to join verification")
					}
				} else if r := d.Dispatch(ctx, rpcRequest("cancel", "cancel", `{}`)); r.Error != nil {
					t.Fatal(r.Error)
				}
			}
			var record RequestRecord
			for {
				record, err = ledger.Lookup("verify")
				if err != nil {
					t.Fatal(err)
				}
				if record.State == "completed" {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("verification did not join")
				case <-time.After(time.Millisecond):
				}
			}
			var response protocol.Response
			if err := json.Unmarshal(record.Result, &response); err != nil {
				t.Fatal(err)
			}
			var value struct {
				Verification app.VerificationResult
				Completed    bool
			}
			if err := json.Unmarshal(response.Result, &value); err != nil {
				t.Fatal(err)
			}
			if !cancelRun && (value.Verification.ID == "" || value.Verification.Assessment.Status != "passed") {
				t.Fatal(value)
			}
			if cancelRun && value.Verification.Assessment.Status == "passed" {
				t.Fatal("cancelled command passed")
			}
			if cancelRun {
				raw, err := os.ReadFile(filepath.Join(work, "marker"))
				if err != nil {
					t.Fatal(err)
				}
				pid, err := strconv.Atoi(string(raw))
				if err != nil {
					t.Fatal(err)
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatal("verification process survived completion", pid, err)
				}
			}
			d.Close()
			if err := ledger.Close(); err != nil {
				t.Fatal(err)
			}
			ledger, err = OpenLedger(ledgerFile)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			d = NewControllerDispatcher(controller, ledger)
			defer d.Close()
			d.Dispatch(ctx, rpcRequest("hello-again", "hello", `{}`))
			again := d.Dispatch(ctx, request)
			if again.Error != nil {
				t.Fatal(again.Error)
			}
			after, err := ledger.Lookup("verify")
			if err != nil || string(after.Result) != string(record.Result) {
				t.Fatal("verification replayed", err)
			}
			if !cancelRun {
				listed := d.Dispatch(ctx, rpcRequest("list", "verification.list", `{}`))
				var page struct {
					IDs   []string
					Total int
				}
				if listed.Error != nil {
					t.Fatal(listed.Error)
				}
				if err := json.Unmarshal(listed.Result, &page); err != nil || page.Total != 1 || page.IDs[0] != value.Verification.ID {
					t.Fatal(page, err)
				}
				unconfirmed, _ := json.Marshal(map[string]any{"id": value.Verification.ID})
				if response := d.Dispatch(ctx, rpcRequest("delete-no", "verification.delete", string(unconfirmed))); response.Error == nil {
					t.Fatal("unconfirmed deletion accepted")
				}
				if _, err := os.Stat(filepath.Join(evidence, value.Verification.ID+".json")); err != nil {
					t.Fatal("unconfirmed deletion removed file", err)
				}
				confirmed, _ := json.Marshal(map[string]any{"id": value.Verification.ID, "confirmed": true})
				deletion := rpcRequest("delete-yes", "verification.delete", string(confirmed))
				removed := d.Dispatch(ctx, deletion)
				if removed.Error != nil {
					t.Fatal(removed.Error)
				}
				replay := d.Dispatch(ctx, deletion)
				if replay.Error != nil || string(replay.Result) != string(removed.Result) {
					t.Fatal("deletion replay changed", replay)
				}
				if _, err := os.Stat(filepath.Join(evidence, value.Verification.ID+".json")); !os.IsNotExist(err) {
					t.Fatal("record not removed", err)
				}
				if _, err := store.Load(ctx, value.Verification.Record.Before); err != nil {
					t.Fatal("deletion removed snapshot", err)
				}
				if _, err := controller.CheckSavedVerification(ctx, "test", value.Verification.ID); err == nil {
					t.Fatal("deleted evidence stayed available")
				}
			}

		})
	}
}

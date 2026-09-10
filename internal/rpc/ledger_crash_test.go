//go:build darwin || linux

package rpc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLedgerCrashFixture(t *testing.T) {
	path := os.Getenv("HAND_RPC_CRASH_LEDGER")
	if path == "" {
		return
	}
	l, err := OpenLedger(path)
	if err != nil {
		os.Exit(10)
	}
	req := promptRequest()
	begin := l.Begin
	if os.Getenv("HAND_RPC_CRASH_KIND") == "control" {
		req = rpcRequest("control", "session.new", `{}`)
		begin = l.BeginControl
	}
	if _, execute, err := begin(req, "session"); err != nil || !execute {
		os.Exit(11)
	}
	stage := os.Getenv("HAND_RPC_CRASH_STAGE")
	if stage == "accepted" || stage == "completed" {
		if err = l.Bind(req.ID, "run-1"); err != nil {
			os.Exit(12)
		}
	}
	if stage == "completed" {
		result := ledgerTerminal()
		if os.Getenv("HAND_RPC_CRASH_KIND") == "control" {
			result = json.RawMessage(`{"version":1,"request_id":"control","result":{"session_id":"new-session"}}`)
		}
		if err = l.Complete(req.ID, result); err != nil {
			os.Exit(13)
		}
	}
	if err = os.WriteFile(os.Getenv("HAND_RPC_CRASH_READY"), []byte(stage), 0600); err != nil {
		os.Exit(14)
	}
	// The parent kills us while the ledger and its exclusive lock remain open.
	for {
		time.Sleep(time.Hour)
	}
}
func TestLedgerKilledWriterNeverReexecutes(t *testing.T)        { testKilledWriter(t, "") }
func TestControlLedgerKilledWriterNeverReexecutes(t *testing.T) { testKilledWriter(t, "control") }
func testKilledWriter(t *testing.T, kind string) {
	for _, stage := range []string{"pending", "accepted", "completed"} {
		t.Run(stage, func(t *testing.T) {
			path := ledgerPath(t)
			ready := filepath.Join(t.TempDir(), "ready")
			cmd := exec.Command(os.Args[0], "-test.run=^TestLedgerCrashFixture$")
			cmd.Env = append(os.Environ(), "HAND_RPC_CRASH_KIND="+kind, "HAND_RPC_CRASH_LEDGER="+path, "HAND_RPC_CRASH_STAGE="+stage, "HAND_RPC_CRASH_READY="+ready)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			joined := false
			defer func() {
				if !joined {
					_ = cmd.Process.Kill()
					<-done
				}
			}()
			deadline := time.After(10 * time.Second)
		waiting:
			for {
				if _, err := os.Stat(ready); err == nil {
					break waiting
				}
				select {
				case err := <-done:
					joined = true
					t.Fatalf("fixture exited before barrier: %v", err)
				case <-deadline:
					t.Fatal("fixture did not reach durable barrier")
				case <-time.After(time.Millisecond):
				}
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err == nil {
				t.Fatal("fixture was not terminated")
			}
			joined = true
			l, err := OpenLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			req := promptRequest()
			begin := l.Begin
			if kind == "control" {
				req = rpcRequest("control", "session.new", `{}`)
				begin = l.BeginControl
			}
			record, execute, err := begin(req, "session")
			if err != nil || execute {
				t.Fatalf("killed writer granted execution: %+v %t %v", record, execute, err)
			}
			want := "uncertain"
			if stage == "completed" {
				want = "completed"
			}
			if record.State != want {
				t.Fatalf("state %s, want %s", record.State, want)
			}
			identity := record.RunID
			if kind == "control" {
				identity = record.OperationID
				if record.Kind != "control" || record.RunID != "" {
					t.Fatal("control record identity changed")
				}
			}
			if stage != "pending" && identity != "run-1" {
				t.Fatal("bound run identity lost")
			}
			wantResult := string(ledgerTerminal())
			if kind == "control" {
				wantResult = `{"version":1,"request_id":"control","result":{"session_id":"new-session"}}`
			}
			if stage == "completed" && string(record.Result) != wantResult {
				t.Fatal("completed result lost")
			}
		})
	}
}

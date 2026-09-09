package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/mcp"
)

func TestStartupMCPRuntimeOwnership(t *testing.T) {
	for _, mode := range []string{"transfer", "disconnect", "late_cancel", "partial_failure"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "joined")
			reg := tool.NewRegistry()
			ready, release := make(chan struct{}), make(chan struct{})
			failure := errors.New("startup finalisation failed")
			a := NewStartupAttempt(context.Background(), time.Minute, func(ctx context.Context) (*runtime.Runtime, error) {
				rt, err := runtime.BuildRuntimeContext(ctx, runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, runtime.AgentSpec{
					ID: "startup", MCPServers: []mcp.ServerConfig{{Name: "startup", Command: os.Args[0],
						Args: []string{"-test.run=^TestOptionalMCPFixture$"},
						Env:  map[string]string{"HAND_OPTIONAL_MCP_MARKER": marker, "GORACE": "atexit_sleep_ms=0"}}},
				})
				close(ready)
				<-release
				if err == nil && mode == "partial_failure" {
					err = failure
				}
				return rt, err
			})
			a.Start()
			<-ready
			// Release before assertions so test failure cannot strand construction.
			if mode == "late_cancel" {
				a.Cancel()
			}
			close(release)
			<-a.Done()
			defer func() {
				rt, _ := a.Finish(errors.New("test cleanup"))
				if rt != nil {
					_ = rt.Close()
				}
			}()
			if names := reg.Names(); len(names) != 1 || names[0] != "mcp__startup__echo" {
				t.Fatalf("real MCP registration missing: %v, %v", names, a.Err())
			}
			var cause error
			if mode == "disconnect" {
				cause = errors.New("terminal disconnected")
			}
			rt, err := a.Finish(cause)
			if mode == "transfer" {
				if rt == nil || err != nil {
					t.Fatalf("transfer: %v %v", rt, err)
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("transfer closed MCP connection")
				}
				if err := rt.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				want := cause
				if mode == "late_cancel" {
					want = context.Canceled
				}
				if mode == "partial_failure" {
					want = failure
				}
				if rt != nil || !errors.Is(err, want) {
					t.Fatalf("abandoned runtime: %v %v", rt, err)
				}
			}
			if contents, err := os.ReadFile(marker); err != nil || string(contents) != "joined" {
				t.Fatalf("MCP child not joined before return: %q %v", contents, err)
			}
		})
	}
}

func TestStartupCancelledBuilderCannotTransferRuntime(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	a := NewStartupAttempt(context.Background(), time.Minute, func(context.Context) (*runtime.Runtime, error) {
		close(started)
		<-release
		return &runtime.Runtime{}, nil // A builder may finish successfully after cancellation.
	})
	a.Start()
	<-started
	a.Cancel()
	select {
	case <-a.Done():
		t.Fatal("cancellation reported completion before construction joined")
	default:
	}
	close(release)
	rt, err := a.Finish(nil)
	if rt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled runtime transferred: %v %v", rt, err)
	}
}

func TestStartupRejectsMissingResultAndPreservesDisconnect(t *testing.T) {
	for _, build := range []func(context.Context) (*runtime.Runtime, error){nil,
		func(context.Context) (*runtime.Runtime, error) { return nil, nil }} {
		a := NewStartupAttempt(context.Background(), time.Minute, build)
		a.Start()
		<-a.Done()
		if a.Err() == nil {
			t.Fatal("missing runtime accepted")
		}
		disconnect := errors.New("disconnected")
		if rt, err := a.Finish(disconnect); rt != nil || !errors.Is(err, disconnect) {
			t.Fatalf("disconnect lost: %v %v", rt, err)
		}
	}
}

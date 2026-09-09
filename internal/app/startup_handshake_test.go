//go:build darwin || linux

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/mcp"
)

func TestStartupCancellationDuringHandshakeJoinsAllChildren(t *testing.T) {
	dir := t.TempDir()
	marker, pidPath := filepath.Join(dir, "joined"), filepath.Join(dir, "hung.pid")
	healthyPIDPath := filepath.Join(dir, "healthy.pid")
	reg := tool.NewRegistry()
	a := NewStartupAttempt(context.Background(), time.Minute, func(ctx context.Context) (*runtime.Runtime, error) {
		return runtime.BuildRuntimeContext(ctx, runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, runtime.AgentSpec{
			ID: "startup", MCPServers: []mcp.ServerConfig{
				{Name: "healthy", Command: os.Args[0], Args: []string{"-test.run=^TestOptionalMCPFixture$"},
					Env: map[string]string{"HAND_OPTIONAL_MCP_MARKER": marker, "HAND_OPTIONAL_MCP_PID": healthyPIDPath, "GORACE": "atexit_sleep_ms=0"}},
				{Name: "hung", Command: "sh", Args: []string{"-c", `echo $$ > "$1"; exec sleep 30`, "fixture", pidPath}},
			},
		})
	})
	a.Start()
	defer func() { a.Cancel(); _, _ = a.Finish(context.Canceled) }()
	deadline := time.Now().Add(5 * time.Second)
	var pid, healthyPID int
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidPath)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			healthyData, _ := os.ReadFile(healthyPIDPath)
			healthyPID, _ = strconv.Atoi(strings.TrimSpace(string(healthyData)))
			if pid > 0 && healthyPID > 0 {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 || healthyPID == 0 {
		t.Fatal("both MCP children did not start")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("child not alive before cancellation: %v", err)
	}
	select {
	case <-a.Done():
		t.Fatal("stalled handshake completed before cancellation")
	default:
	}
	start := time.Now()
	a.Cancel()
	rt, err := a.Finish(nil)
	if rt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result: %v %v", rt, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cleanup took %s", elapsed)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("hung child not reaped before return: %v", err)
	}
	if err := syscall.Kill(healthyPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("healthy child not reaped before return: %v", err)
	}
	if names := reg.Names(); len(names) != 0 {
		t.Fatalf("partial tool catalogue published: %v", names)
	}
}

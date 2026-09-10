package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/mcp"
)

func TestOptionalMCPFixture(t *testing.T) {
	marker := os.Getenv("HAND_OPTIONAL_MCP_MARKER")
	if marker == "" {
		return
	}
	if pidPath := os.Getenv("HAND_OPTIONAL_MCP_PID"); pidPath != "" {
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(3)
		}
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "optional-test", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(marker, []byte("joined"), 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func awaitMCPState(t *testing.T, m *OptionalMCP, state string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if s := m.Status(); len(s) == 1 && s[0].State == state {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected %s, got %+v", state, m.Status())
		case <-time.After(time.Millisecond):
		}
	}
}
func TestOptionalMCPDefersAttachmentDuringForegroundWork(t *testing.T) {
	reg := tool.NewRegistry()
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, runtime.AgentSpec{ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	c := &Controller{Rt: rt, Owner: New(nil, Options{})}
	_, release, err := c.Owner.reserve(context.Background(), Idle)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	marker := filepath.Join(t.TempDir(), "joined")
	manager, err := NewOptionalMCP(c, []mcp.ServerConfig{{Name: "optional", Optional: true, Command: os.Args[0], Args: []string{"-test.run=^TestOptionalMCPFixture$"}, Env: map[string]string{"HAND_OPTIONAL_MCP_MARKER": marker, "GORACE": "atexit_sleep_ms=0"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	awaitMCPState(t, manager, "ready")
	if len(reg.Names()) != 0 {
		t.Fatal("catalogue changed during active operation")
	}
	release()
	awaitMCPState(t, manager, "connected")
	if len(reg.Names()) != 1 || reg.Names()[0] != "mcp__optional__echo" {
		t.Fatalf("catalogue %v", reg.Names())
	}
	manager.Close()
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("discovery closed transferred client")
	}
	if err = rt.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatal("runtime did not join transferred client")
	}
}
func TestOptionalMCPRetryAndShutdown(t *testing.T) {
	manager, err := NewOptionalMCP(&Controller{Owner: New(nil, Options{})}, []mcp.ServerConfig{{Name: "missing", Optional: true, Command: filepath.Join(t.TempDir(), "missing")}})
	if err != nil {
		t.Fatal(err)
	}
	awaitMCPState(t, manager, "unavailable")
	if err = manager.Retry("missing"); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	if err = manager.Retry("missing"); err == nil {
		t.Fatal("retry after shutdown")
	}
	if manager.Status()[0].State != "unavailable" {
		t.Fatal("unfinished discovery after shutdown")
	}
}

func TestOptionalMCPShutdownJoinsReadyUntransferredClient(t *testing.T) {
	reg := tool.NewRegistry()
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: reg}, runtime.AgentSpec{ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	c := &Controller{Rt: rt, Owner: New(nil, Options{})}
	_, release, err := c.Owner.reserve(context.Background(), Idle)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	marker := filepath.Join(t.TempDir(), "joined")
	manager, err := NewOptionalMCP(c, []mcp.ServerConfig{{Name: "optional", Optional: true, Command: os.Args[0], Args: []string{"-test.run=^TestOptionalMCPFixture$"}, Env: map[string]string{"HAND_OPTIONAL_MCP_MARKER": marker, "GORACE": "atexit_sleep_ms=0"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	awaitMCPState(t, manager, "ready")
	manager.Close()
	if _, err = os.Stat(marker); err != nil {
		t.Fatal("untransferred client not joined")
	}
	if len(reg.Names()) != 0 || manager.Status()[0].State != "unavailable" {
		t.Fatal("shutdown published tools or left retryable state before cleanup")
	}
}

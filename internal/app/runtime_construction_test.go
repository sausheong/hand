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

func TestRuntimeConstructionOptionalPolicyAndSnapshots(t *testing.T) {
	server := mcp.ServerConfig{Name: "optional", Optional: true, Command: filepath.Join(t.TempDir(), "missing"),
		Args: []string{"original"}, Env: map[string]string{"VALUE": "original"}}
	c, err := NewRuntimeConstruction(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: tool.NewRegistry()},
		runtime.AgentSpec{ID: "test", MCPServers: []mcp.ServerConfig{server}}, true)
	if err != nil {
		t.Fatal(err)
	}
	server.Env["VALUE"] = "changed"
	server.Args[0] = "changed"
	optional := c.OptionalServers()
	if c.ServerCount() != 0 || len(optional) != 1 || optional[0].Env["VALUE"] != "original" || optional[0].Args[0] != "original" {
		t.Fatal("startup configuration aliases caller data")
	}
	optional[0].Env["VALUE"] = "mutated snapshot"
	if c.OptionalServers()[0].Env["VALUE"] != "original" {
		t.Fatal("optional snapshot mutates construction")
	}
	rt, err := c.BuildWithTimeout(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if len(rt.MCPStatus()) != 0 {
		t.Fatal("deferred optional server was attempted before UI startup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if rt, err := c.BuildWithTimeout(ctx, 0); rt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation ignored: %v %v", rt, err)
	}
}

func TestRuntimeConstructionRejectsAmbiguousNames(t *testing.T) {
	for _, deferred := range []bool{true, false} {
		for _, servers := range [][]mcp.ServerConfig{{{Name: ""}}, {{Name: "same"}, {Name: "same", Optional: true}}} {
			if c, err := NewRuntimeConstruction(runtime.RuntimeDeps{}, runtime.RuntimeInputs{}, runtime.AgentSpec{MCPServers: servers}, deferred); c != nil || err == nil {
				t.Fatal("invalid names accepted")
			}
		}
	}
}

func TestRuntimeConstructionOwnsConnectedMCP(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "joined")
	registry := tool.NewRegistry()
	c, err := NewRuntimeConstruction(runtime.RuntimeDeps{}, runtime.RuntimeInputs{Tools: registry}, runtime.AgentSpec{
		ID: "test", MCPServers: []mcp.ServerConfig{{Name: "required", Command: os.Args[0], Args: []string{"-test.run=^TestOptionalMCPFixture$"},
			Env: map[string]string{"HAND_OPTIONAL_MCP_MARKER": marker, "GORACE": "atexit_sleep_ms=0"}}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := c.BuildWithTimeout(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if names := registry.Names(); len(names) != 1 || names[0] != "mcp__required__echo" {
		t.Fatalf("missing connected tool: %v", names)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("startup timer closed transferred connection")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "joined" {
		t.Fatalf("close did not join MCP: %q %v", data, err)
	}
}

package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tools/mcp"
)

// DefaultMCPConnectTimeout bounds startup connection and discovery work.
const DefaultMCPConnectTimeout = 15 * time.Second

// RuntimeConstruction owns MCP startup policy for CLI and interactive clients.
// Dependencies and inputs remain owned by the caller until construction joins.
// Build must not run concurrently or be retried before the previous attempt joins.
type RuntimeConstruction struct {
	deps     runtime.RuntimeDeps
	inputs   runtime.RuntimeInputs
	spec     runtime.AgentSpec
	optional []mcp.ServerConfig
}

func cloneStartupServer(server mcp.ServerConfig) mcp.ServerConfig {
	server.Args = slices.Clone(server.Args)
	server.Env = maps.Clone(server.Env)
	server.Headers = maps.Clone(server.Headers)
	if server.ClientImplementation != nil {
		value := *server.ClientImplementation
		server.ClientImplementation = &value
	}
	return server
}

func NewRuntimeConstruction(deps runtime.RuntimeDeps, inputs runtime.RuntimeInputs, spec runtime.AgentSpec, deferOptional bool) (*RuntimeConstruction, error) {
	c := &RuntimeConstruction{deps: deps, inputs: inputs, spec: spec}
	c.spec.MCPServers = nil
	names := make(map[string]bool)
	for _, original := range spec.MCPServers {
		if original.Name == "" || names[original.Name] {
			return nil, errors.New("MCP server names must be nonempty and unique")
		}
		names[original.Name] = true
		server := cloneStartupServer(original)
		if deferOptional && server.Optional {
			c.optional = append(c.optional, server)
		} else {
			c.spec.MCPServers = append(c.spec.MCPServers, server)
		}
	}
	return c, nil
}

func (c *RuntimeConstruction) ServerCount() int { return len(c.spec.MCPServers) }

func (c *RuntimeConstruction) OptionalServers() []mcp.ServerConfig {
	result := make([]mcp.ServerConfig, len(c.optional))
	for i, server := range c.optional {
		result[i] = cloneStartupServer(server)
	}
	return result
}

// Build transfers successful connections to the returned runtime. Harness joins
// failed/partial connections before returning; the startup owner joins Build.
func (c *RuntimeConstruction) Build(ctx context.Context) (*runtime.Runtime, error) {
	return runtime.BuildRuntimeContext(ctx, c.deps, c.inputs, c.spec)
}

func (c *RuntimeConstruction) BuildWithTimeout(ctx context.Context, timeout time.Duration) (*runtime.Runtime, error) {
	if c.ServerCount() == 0 {
		return c.Build(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rt, err := c.Build(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("timed out after %s connecting configured MCP servers (check mcp_servers in ~/.hand/config.json): %w", timeout, err)
	}
	return rt, err
}

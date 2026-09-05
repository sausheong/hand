// Command mcpserver is a minimal stdio MCP server used only as a test
// fixture for internal/agentio's BuildRuntimeWithTimeout tests — it
// answers the initialize handshake immediately with zero tools, giving a
// real (not faked) fast-resolving mcp.Connect call to test against. Not
// part of hand itself; built on demand by the test from
// internal/agentio/testdata, a directory name the go tool always
// excludes from ./... package patterns.
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "hand-test-fixture"}, nil)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("mcpserver fixture: %v", err)
	}
}

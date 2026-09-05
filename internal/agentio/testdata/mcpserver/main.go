// Command mcpserver is a minimal stdio MCP server used only as a test
// fixture for internal/agentio's BuildRuntimeWithTimeout tests — it
// answers the initialize handshake with zero tools, giving a real (not
// faked) mcp.Connect call to test against. Not part of hand itself;
// built on demand by the test from internal/agentio/testdata, a
// directory name the go tool always excludes from ./... package
// patterns.
//
// -delay simulates a slow-but-not-hung server: sleep before starting the
// transport, so a short test timeout fires before the handshake
// completes.
//
// -marker writes an empty file at the given path once Run returns —
// which happens when the client side closes the stdio pipes (what
// (*runtime.Runtime).Close does via the MCP client it holds). Tests use
// this as an external, black-box signal that the connection was
// actually torn down, without needing to instrument hand's own
// cleanup code.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	delay := flag.Duration("delay", 0, "sleep before starting the transport")
	marker := flag.String("marker", "", "file to create once Run returns (the client closed the connection)")
	flag.Parse()

	if *delay > 0 {
		time.Sleep(*delay)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "hand-test-fixture"}, nil)
	runErr := server.Run(context.Background(), &mcp.StdioTransport{})

	if *marker != "" {
		if err := os.WriteFile(*marker, nil, 0o644); err != nil {
			log.Printf("mcpserver fixture: write marker: %v", err)
		}
	}
	if runErr != nil {
		log.Printf("mcpserver fixture: %v", runErr)
	}
}

package agentio

import "testing"

func TestMcpToolServer(t *testing.T) {
	cases := []struct {
		name       string
		toolName   string
		allServers []string
		wantServer string
		wantOK     bool
	}{
		{"well-formed tool matches its configured server", "mcp__github__create_issue", []string{"github"}, "github", true},
		{"non-MCP built-in tool never matches", "bash", []string{"github"}, "", false},
		{"no servers configured at all", "mcp__github__create_issue", nil, "", false},
		{"server not in the configured set", "mcp__github__create_issue", []string{"other"}, "", false},
		// Regression: a configured server name containing "__" (e.g. a
		// user naming it "brave__search") must resolve to its own exact
		// name, not get truncated at the first "__" the way a naive
		// strings.Cut-based split would.
		{"server name itself contains double underscore", "mcp__brave__search__web_search", []string{"brave__search"}, "brave__search", true},
		// When only the shorter name is actually configured, it IS the
		// real server (the tool itself is just named "search__web_search")
		// — correctly resolving to "brave" here, not a bug.
		{"only the short name configured resolves to it", "mcp__brave__search__web_search", []string{"brave"}, "brave", true},
		// The actual ambiguous case: both "brave" and "brave__search" are
		// configured servers. The longer, more specific name must win.
		{"longest match wins when both overlapping names are configured", "mcp__brave__search__web_search", []string{"brave", "brave__search"}, "brave__search", true},
		{"unrelated configured server never matches", "mcp__no_double_underscore", []string{"other-server"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, ok := mcpToolServer(tc.toolName, tc.allServers)
			if server != tc.wantServer || ok != tc.wantOK {
				t.Fatalf("mcpToolServer(%q, %v) = (%q, %v), want (%q, %v)", tc.toolName, tc.allServers, server, ok, tc.wantServer, tc.wantOK)
			}
		})
	}
}

func TestIsGated(t *testing.T) {
	cases := []struct {
		name           string
		tool           string
		allServers     []string
		trustedServers map[string]bool
		want           bool
	}{
		{"built-in gated tool", "bash", nil, nil, true},
		{"built-in ungated tool", "read_file", nil, nil, false},
		{
			"untrusted MCP tool", "mcp__untrusted__tool",
			[]string{"untrusted", "trusted-server"}, map[string]bool{"trusted-server": true},
			true,
		},
		{
			"trusted MCP tool", "mcp__trusted-server__tool",
			[]string{"untrusted", "trusted-server"}, map[string]bool{"trusted-server": true},
			false,
		},
		{
			"MCP tool whose server isn't even configured fails safe as gated", "mcp__unknown__tool",
			[]string{"trusted-server"}, map[string]bool{"trusted-server": true},
			true,
		},
		// End-to-end regressions for the "__"-in-server-name bug, through
		// the actual isGated entry point rather than just mcpToolServer.
		{
			"trusted server whose name contains a double underscore is correctly ungated", "mcp__brave__search__web_search",
			[]string{"brave", "brave__search"}, map[string]bool{"brave__search": true},
			false,
		},
		{
			"a shorter trusted name must not be mistaken for the real, untrusted, longer server", "mcp__brave__search__web_search",
			[]string{"brave", "brave__search"}, map[string]bool{"brave": true},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isGated(tc.tool, tc.allServers, tc.trustedServers)
			if got != tc.want {
				t.Fatalf("isGated(%q, %v, %+v) = %v, want %v", tc.tool, tc.allServers, tc.trustedServers, got, tc.want)
			}
		})
	}
}

func TestIsGated_NoTrustedServersGatesEveryMCPTool(t *testing.T) {
	if !isGated("mcp__anything__tool", []string{"anything"}, nil) {
		t.Fatal("expected an MCP tool to be gated when trustedServers is nil")
	}
	if !isGated("mcp__anything__tool", nil, nil) {
		t.Fatal("expected an MCP tool to be gated when no servers are configured at all")
	}
}

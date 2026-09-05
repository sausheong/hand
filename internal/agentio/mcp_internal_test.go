package agentio

import "testing"

func TestMcpServerName(t *testing.T) {
	cases := []struct {
		name       string
		toolName   string
		wantServer string
		wantIsMCP  bool
	}{
		{"well-formed MCP tool", "mcp__github__create_issue", "github", true},
		{"non-MCP built-in tool", "bash", "", false},
		// strings.Cut returns the whole input as "before" when the
		// separator isn't found, so the leftover server string here is
		// "no_double_underscore", not empty. isMCP staying true is what
		// actually matters for safety (see TestIsGated below) — the
		// leftover string itself is never a real server name.
		{"malformed but prefixed still reports MCP", "mcp__no_double_underscore", "no_double_underscore", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, isMCP := mcpServerName(tc.toolName)
			if server != tc.wantServer || isMCP != tc.wantIsMCP {
				t.Fatalf("mcpServerName(%q) = (%q, %v), want (%q, %v)", tc.toolName, server, isMCP, tc.wantServer, tc.wantIsMCP)
			}
		})
	}
}

func TestIsGated(t *testing.T) {
	trustedServers := map[string]bool{"trusted-server": true}
	cases := []struct {
		name string
		tool string
		want bool
	}{
		{"built-in gated tool", "bash", true},
		{"built-in ungated tool", "read_file", false},
		{"untrusted MCP tool", "mcp__untrusted__tool", true},
		{"trusted MCP tool", "mcp__trusted-server__tool", false},
		{"malformed MCP tool name fails safe as gated", "mcp__no_double_underscore", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isGated(tc.tool, trustedServers)
			if got != tc.want {
				t.Fatalf("isGated(%q, %+v) = %v, want %v", tc.tool, trustedServers, got, tc.want)
			}
		})
	}
}

func TestIsGated_NilTrustedServersGatesEveryMCPTool(t *testing.T) {
	if !isGated("mcp__anything__tool", nil) {
		t.Fatal("expected an MCP tool to be gated when trustedServers is nil")
	}
}

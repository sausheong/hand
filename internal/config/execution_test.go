package config

import (
	"strings"
	"testing"
)

func TestExecutionConfigurationFailsClosed(t *testing.T) {
	valid := ExecutionConfig{Backend: "container", Docker: "/docker", Socket: "/socket", Worker: "/worker", WorkerSHA256: strings.Repeat("b", 64), Image: "sha256:" + strings.Repeat("a", 64)}
	if err := ValidateExecution(Config{Execution: valid}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []ExecutionConfig{{Backend: "typo"}, {Backend: "host", Network: true}, {Backend: "container"}, {Backend: "container", Docker: "docker", Socket: "/socket", Worker: "/worker", Image: valid.Image}} {
		if err := ValidateExecution(Config{Execution: e}); err == nil {
			t.Fatal("invalid boundary accepted")
		}
	}
	cfg := Config{Execution: valid, MCPServers: []MCPServer{{Name: "external"}}}
	if err := ValidateExecution(cfg); err == nil {
		t.Fatal("implicit external MCP trust")
	}
	cfg.Execution.TrustExternalMCP = true
	if err := ValidateExecution(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Hooks = []HookConfig{{}}
	if err := ValidateExecution(cfg); err == nil {
		t.Fatal("implicit hook trust")
	}
	cfg.Execution.TrustExternalHooks = true
	if err := ValidateExecution(cfg); err != nil {
		t.Fatal(err)
	}
}

package config

import (
	"fmt"
	"path/filepath"
	"regexp"
)

type ExecutionConfig struct {
	WorkerSHA256       string `json:"worker_sha256,omitempty"`
	Backend            string `json:"backend,omitempty"`
	Docker             string `json:"docker,omitempty"`
	Socket             string `json:"socket,omitempty"`
	Image              string `json:"image,omitempty"`
	Worker             string `json:"worker,omitempty"`
	Writable           bool   `json:"writable,omitempty"`
	Network            bool   `json:"network,omitempty"`
	TrustExternalMCP   bool   `json:"trust_external_mcp,omitempty"`
	TrustExternalHooks bool   `json:"trust_external_hooks,omitempty"`
}

func ValidateExecution(c Config) error {
	e := c.Execution
	switch e.Backend {
	case "", "host":
		if e.Docker != "" || e.Socket != "" || e.Image != "" || e.Worker != "" || e.WorkerSHA256 != "" || e.Writable || e.Network || e.TrustExternalMCP || e.TrustExternalHooks {
			return fmt.Errorf("container execution options require backend=container")
		}
	case "container":
		if !filepath.IsAbs(e.Docker) || !filepath.IsAbs(e.Socket) || !filepath.IsAbs(e.Worker) || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(e.Image) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(e.WorkerSHA256) {
			return fmt.Errorf("container execution requires absolute docker/socket/worker paths and immutable image/worker SHA-256 digests")
		}
		if len(c.MCPServers) > 0 && !e.TrustExternalMCP {
			return fmt.Errorf("configured MCP servers run outside the container; explicit trust_external_mcp is required")
		}
		if len(c.Hooks) > 0 && !e.TrustExternalHooks {
			return fmt.Errorf("configured hooks run outside the container; explicit trust_external_hooks is required")
		}
	default:
		return fmt.Errorf("unknown execution backend %q", e.Backend)
	}
	return nil
}

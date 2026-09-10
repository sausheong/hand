package isolation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/toolproxy"
	"github.com/sausheong/hand/internal/toolworker"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
)

func Prepare(ctx context.Context, cfg config.Config, workspace string) (execution.Backend, string, error) {
	if err := config.ValidateExecution(cfg); err != nil {
		return nil, "", err
	}
	e := cfg.Execution
	if e.Backend != "container" {
		return nil, "unrestricted host", nil
	}
	backend := execution.Container{Docker: e.Docker, Socket: e.Socket, Image: e.Image, WorkerPath: e.Worker, WorkerSHA256: e.WorkerSHA256, Workspace: workspace, Writable: e.Writable, Network: e.Network}
	result, err := backend.Run(ctx, execution.Request{Argv: []string{"/bin/bash", "-c", "exit 0"}})
	if err != nil || result.ExitCode != 0 {
		return nil, "", fmt.Errorf("container shell probe failed: %v: %s", err, result.Stderr)
	}
	raw, _ := json.Marshal(toolworker.Request{Version: 1, Tool: "health", Input: json.RawMessage(`{}`)})
	result, err = backend.Run(ctx, execution.Request{Argv: []string{"/hand-worker"}, Stdin: raw})
	if err != nil || result.ExitCode != 0 || result.StdoutTruncated {
		return nil, "", fmt.Errorf("container worker probe failed: %v: %s", err, result.Stderr)
	}
	var response toolworker.Response
	if err = json.Unmarshal([]byte(result.Stdout), &response); err != nil || response.Version != 1 || response.Result.Output != "ready" {
		return nil, "", fmt.Errorf("container worker protocol probe failed")
	}
	boundary := backend.Boundary() + "; model-provider requests remain on host"
	if len(cfg.MCPServers) > 0 {
		boundary += "; explicitly trusted MCP servers execute outside container"
	}
	if len(cfg.Hooks) > 0 {
		boundary += "; explicitly trusted hooks execute on host"
	}
	return backend, boundary, nil
}

// Install runs after runtime construction has registered load_skill and MCP.
func Install(reg *tool.Registry, backend execution.Backend, workspace string) error {
	if backend == nil {
		return nil
	}
	var replacements []tool.Tool
	for _, name := range reg.Names() {
		original, _ := reg.Get(name)
		switch name {
		case "bash":
			replacements = append(replacements, &bash.BashTool{Backend: backend})
		case "read_file", "write_file", "edit_file", "search", "todo_write", "skill_manage", "load_skill", "web_fetch", "web_search":
			replacements = append(replacements, &toolproxy.Tool{Tool: original, Backend: backend, Workspace: workspace})
		case "process": // already configured through NewProcessesWithBackend
		default:
			// MCP tools are admitted only by explicit external trust in Prepare.
			if len(name) < 5 || name[:5] != "mcp__" {
				return fmt.Errorf("tool %q has no configured execution boundary", name)
			}
		}
	}
	for _, replacement := range replacements {
		reg.Register(replacement)
	}
	return nil
}

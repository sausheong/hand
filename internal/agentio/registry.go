package agentio

import (
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/bash"
	"github.com/sausheong/harness/tools/file"
	"github.com/sausheong/harness/tools/todo"
	"github.com/sausheong/harness/tools/web"
)

// BuildRegistry returns the tool.Registry for agcode's four Phase 1
// tool packages, all scoped to workDir. bash.BashTool's ExecPolicy is
// left nil (full) — the approval bridge in approval.go is the safety
// net for bash in Phase 1, not the exec policy.
func BuildRegistry(workDir string) *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(&file.ReadFileTool{WorkDir: workDir})
	reg.Register(&file.WriteFileTool{WorkDir: workDir})
	reg.Register(&file.EditFileTool{WorkDir: workDir})
	reg.Register(&bash.BashTool{WorkDir: workDir})
	reg.Register(&web.WebFetchTool{})
	reg.Register(&web.WebSearchTool{})
	reg.Register(&todo.TodoWriteTool{WorkDir: workDir})
	return reg
}

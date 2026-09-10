package agentio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverNestedInstructions loads only directories between the workspace and
// the accessed file's parent. Unrelated siblings and outside paths are excluded.
func DiscoverNestedInstructions(workspace, path string) InstructionReport {
	var report InstructionReport
	work, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return InstructionReport{Diagnostics: []string{"Nested guidance workspace: " + err.Error()}}
	}
	work, err = filepath.Abs(work)
	if err != nil {
		return report
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(work, path)
	}
	resolved, err := resolveInstructionTarget(path)
	if err != nil {
		return InstructionReport{Diagnostics: []string{"Nested guidance path: " + err.Error()}}
	}
	rel, err := filepath.Rel(work, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return report
	}
	var reverse []string
	for dir := filepath.Dir(resolved); dir != work; dir = filepath.Dir(dir) {
		if len(reverse) >= instructionDepthLimit || filepath.Dir(dir) == dir {
			return InstructionReport{Diagnostics: []string{"Nested guidance depth exceeds limit; excluded"}}
		}
		reverse = append(reverse, dir)
	}
	dirs := make([]string, len(reverse))
	for i := range reverse {
		dirs[len(reverse)-1-i] = reverse[i]
	}
	return discoverInstructionDirectories(dirs, report)
}

type instructionReadTool struct{ *file.ReadFileTool }

// ReadFileWithInstructions preserves the file tool's permission and path rules.
// Nested guidance is returned with a successful read, within the same backend.
func ReadFileWithInstructions(workspace string, exact bool) tool.Tool {
	return &instructionReadTool{&file.ReadFileTool{WorkDir: workspace, ExactPath: exact}}
}
func (t *instructionReadTool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	result, err := t.ReadFileTool.Execute(ctx, input)
	if err != nil || result.Error != "" || ctx.Err() != nil {
		return result, err
	}
	var in struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(input, &in) != nil {
		return result, err
	}
	path := in.Path
	if !t.ExactPath {
		path = tool.ExpandHome(path)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.WorkDir, path)
	}
	if !t.ExactPath {
		path = tool.ResolveExistingPath(path)
	}
	report := DiscoverNestedInstructions(t.WorkDir, path)
	if len(report.Sources) == 0 && len(report.Diagnostics) == 0 {
		return result, nil
	}
	result.Output = report.Format() + "\nRead file contents:\n" + result.Output
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["instruction_sources"] = report.Sources
	result.Metadata["instruction_diagnostics"] = report.Diagnostics
	return result, nil
}

// Resolve an existing target or its nearest existing ancestor. New files still
// inherit the guidance of existing parent directories before their creation.
func resolveInstructionTarget(path string) (string, error) {
	candidate := filepath.Clean(path)
	var suffix []string
	for depth := 0; depth < instructionDepthLimit; depth++ {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", err
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
	return "", fmt.Errorf("nested guidance path exceeds depth limit")
}

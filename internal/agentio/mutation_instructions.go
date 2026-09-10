package agentio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
)

// The digest is an acknowledgement of currently applicable guidance, never an
// execution grant. Existing permission hooks still authorize the tool call.
type instructionMutationTool struct {
	tool.Tool
	workspace   string
	exact, edit bool
}

func WriteFileWithInstructions(workspace string, exact bool) tool.Tool {
	return &instructionMutationTool{Tool: &file.WriteFileTool{WorkDir: workspace, ExactPath: exact}, workspace: workspace, exact: exact}
}
func EditFileWithInstructions(workspace string, exact bool) tool.Tool {
	return &instructionMutationTool{Tool: &file.EditFileTool{WorkDir: workspace, ExactPath: exact}, workspace: workspace, exact: exact, edit: true}
}
func (t *instructionMutationTool) Parameters() json.RawMessage {
	var schema map[string]any
	if json.Unmarshal(t.Tool.Parameters(), &schema) != nil {
		return t.Tool.Parameters()
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return t.Tool.Parameters()
	}
	props["instruction_digest"] = map[string]any{"type": "string", "description": "If the tool returns nested project guidance, read it, adjust the mutation to comply, and repeat with the returned digest. A changed guidance digest requires a fresh review.", "maxLength": 64}
	raw, _ := json.Marshal(schema)
	return raw
}
func (t *instructionMutationTool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return tool.ToolResult{}, err
	}
	var in struct {
		Path   string `json:"path"`
		Digest string `json:"instruction_digest"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.ToolResult{Error: "invalid file mutation input"}, nil
	}
	if in.Path == "" {
		return t.Tool.Execute(ctx, input)
	}
	path := in.Path
	if !t.exact {
		path = tool.ExpandHome(path)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	if t.edit && !t.exact {
		path = tool.ResolveExistingPath(path)
	}
	if err := tool.ValidatePathInWorkDir(path, t.workspace); err != nil {
		return tool.ToolResult{Error: err.Error()}, nil
	}
	report := DiscoverNestedInstructions(t.workspace, path)
	if len(report.Sources) == 0 && len(report.Diagnostics) == 0 {
		return t.Tool.Execute(ctx, input)
	}
	encoded, _ := json.Marshal(struct {
		Path   string
		Report InstructionReport
	}{filepath.Clean(path), report})
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	if in.Digest != digest {
		return tool.ToolResult{Error: "nested instructions must be read before this mutation; no file was changed", Output: report.Format() + fmt.Sprintf("\nRead this guidance, adapt the mutation if needed, and repeat with instruction_digest %s. This is not execution permission.", digest), Metadata: map[string]any{"instruction_digest": digest, "instruction_sources": report.Sources, "instruction_diagnostics": report.Diagnostics}}, nil
	}
	if err := ctx.Err(); err != nil {
		return tool.ToolResult{}, err
	}
	return t.Tool.Execute(ctx, input)
}

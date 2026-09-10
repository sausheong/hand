// Package toolproxy routes admitted built-in requests into the selected backend.
package toolproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/sausheong/hand/internal/toolworker"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tool"
)

// Tool retains the original definition and concurrency contract, but never
// invokes its host Execute method. The backend must provide /hand-worker.
type Tool struct {
	Workspace string // trusted host workspace mounted at /workspace
	tool.Tool
	Backend execution.Backend
}

func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	if t.Tool == nil || t.Backend == nil {
		return tool.ToolResult{}, errors.New("tool worker backend unavailable")
	}
	if err := ctx.Err(); err != nil {
		return tool.ToolResult{}, err
	}
	mapped, err := mapInput(t.Workspace, t.Name(), input)
	if err != nil {
		return tool.ToolResult{}, err
	}
	raw, err := json.Marshal(toolworker.Request{Version: 1, Tool: t.Name(), Input: mapped})
	if err != nil {
		return tool.ToolResult{}, err
	}
	if len(raw) > toolworker.MaxRequestBytes {
		return tool.ToolResult{}, errors.New("tool worker request exceeds limit")
	}
	result, err := t.Backend.Run(ctx, execution.Request{Argv: []string{"/hand-worker"}, Stdin: raw, CaptureLimit: toolworker.MaxResponseBytes})
	if err != nil {
		return tool.ToolResult{}, fmt.Errorf("tool worker execution failed; operation may have completed: %w", err)
	}
	if result.ExitCode != 0 || result.StdoutTruncated {
		return tool.ToolResult{}, errors.New("tool worker failed or response truncated; operation may have completed")
	}
	var response struct {
		Version int                `json:"version"`
		Result  *tool.ToolResult   `json:"result"`
		Images  []llm.ImageContent `json:"images,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(result.Stdout)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&response); err != nil {
		return tool.ToolResult{}, fmt.Errorf("invalid tool worker response; operation may have completed: %w", err)
	}
	if err = decoder.Decode(new(any)); err != io.EOF || response.Version != 1 || response.Result == nil {
		return tool.ToolResult{}, errors.New("invalid tool worker response framing or version")
	}
	response.Result.Images = response.Images
	if response.Result.Metadata == nil {
		response.Result.Metadata = make(map[string]any)
	}
	response.Result.Metadata["execution_boundary"] = t.Backend.Boundary()
	return *response.Result, nil
}

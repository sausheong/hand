// Package toolworker dispatches a single already-admitted built-in operation.
// It is an execution component, not a permission service. The parent must put
// this worker inside the selected backend before providing untrusted requests.
package toolworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tool"
)

const MaxRequestBytes = 1 << 20
const MaxResponseBytes = 16 << 20

type Request struct {
	Version int             `json:"version"`
	Tool    string          `json:"tool"`
	Input   json.RawMessage `json:"input"`
}
type Response struct {
	Images  []llm.ImageContent `json:"images,omitempty"`
	Version int                `json:"version"`
	Result  tool.ToolResult    `json:"result"`
}

func Serve(ctx context.Context, workspace string, input io.Reader, output io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(input, MaxRequestBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > MaxRequestBytes {
		return errors.New("tool worker request exceeds limit")
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return errors.New("tool worker expects one request")
	}
	if request.Version != 1 {
		return errors.New("unsupported tool worker version")
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(request.Input, &object); err != nil || object == nil {
		return errors.New("tool worker input must be an object")
	}
	// Shell output uses the execution backend's artifact channel. Background
	// processes and external MCP/extensions have separate owned lifecycles.
	if request.Tool == "health" {
		return json.NewEncoder(output).Encode(Response{Version: 1, Result: tool.ToolResult{Output: "ready"}})
	}
	switch request.Tool {
	case "read_file", "write_file", "edit_file", "search", "todo_write", "skill_manage", "load_skill", "web_fetch", "web_search":
	default:
		return errors.New("tool is not supported by the built-in worker")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	provider, store, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		return err
	}
	registry := agentio.BuildRegistry(workspace, store)
	registry.Register(agentio.ReadFileWithInstructions(workspace, true))
	registry.Register(agentio.WriteFileWithInstructions(workspace, true))
	registry.Register(agentio.EditFileWithInstructions(workspace, true))
	registry.Register(&tool.LoadSkillTool{Lookup: provider.Get})
	result, err := registry.Execute(ctx, request.Tool, request.Input)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(Response{Version: 1, Result: result, Images: result.Images})
	if err != nil {
		return err
	}
	if len(encoded)+1 > MaxResponseBytes {
		return errors.New("tool worker result exceeds limit; operation may already have completed")
	}
	encoded = append(encoded, '\n')
	n, err := output.Write(encoded)
	if err == nil && n != len(encoded) {
		return io.ErrShortWrite
	}
	return err
}

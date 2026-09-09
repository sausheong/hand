package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sausheong/harness/tool"
)

// ProcessTool is dispatched through the runtime's BeforeToolUse approval hook.
// Direct Execute callers, like other tools, must provide their own admission.
type ProcessTool struct{ Processes *Processes }

func (*ProcessTool) Name() string { return "process" }
func (t *ProcessTool) Description() string {
	return "Manage invocation-owned background shell processes: start, list, read, send, cancel, wait or forget. Start executes in the startup workspace. Effective boundary: " + t.Processes.Boundary() + ". Background work survives foreground cancellation but is cancelled when Hand exits. Send writes exact data; include a newline for line-oriented input. Read returns bounded output and artifact references. Wait defaults to 1000ms, maximum 30000ms, then reports current state."
}
func (*ProcessTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["start","list","read","send","cancel","wait","forget"]},"command":{"type":"string","maxLength":65536},"id":{"type":"string"},"data":{"type":"string","maxLength":65536},"wait_ms":{"type":"integer","minimum":1,"maximum":30000}},"required":["action"],"additionalProperties":false}`)
}
func (*ProcessTool) IsConcurrencySafe(json.RawMessage) bool { return false }
func (t *ProcessTool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return tool.ToolResult{}, err
	}
	if t.Processes == nil {
		return tool.ToolResult{}, errors.New("background processes unavailable")
	}
	if len(input) > 256<<10 {
		return tool.ToolResult{}, errors.New("process input exceeds 256 KiB")
	}
	var in struct {
		Action  string `json:"action"`
		Command string `json:"command"`
		ID      string `json:"id"`
		Data    string `json:"data"`
		WaitMS  int    `json:"wait_ms"`
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return tool.ToolResult{}, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return tool.ToolResult{}, errors.New("expected one JSON object")
	}
	if in.WaitMS < 0 || in.WaitMS > 30000 {
		return tool.ToolResult{}, errors.New("wait_ms must be within 1..30000 or omitted")
	}
	p := t.Processes
	result := func(value any, err error) (tool.ToolResult, error) {
		if err != nil {
			return tool.ToolResult{}, err
		}
		b, err := json.Marshal(value)
		return tool.ToolResult{Output: string(b)}, err
	}
	switch in.Action {
	case "start":
		v, err := p.Start(ctx, in.Command)
		return result(v, err)
	case "list":
		return result(p.List(), nil)
	case "send":
		return result(map[string]string{"id": in.ID, "state": "input_sent"}, p.Send(ctx, in.ID, []byte(in.Data)))
	case "cancel":
		return result(map[string]string{"id": in.ID, "state": "cancellation_requested"}, p.Cancel(in.ID))
	case "forget":
		return result(map[string]string{"id": in.ID, "state": "forgotten"}, p.Forget(in.ID))
	case "read", "wait":
		if in.Action == "wait" {
			if in.WaitMS == 0 {
				in.WaitMS = 1000
			}
			wait, cancel := context.WithTimeout(ctx, time.Duration(in.WaitMS)*time.Millisecond)
			_, err := p.Wait(wait, in.ID)
			cancel()
			if ctx.Err() != nil {
				return tool.ToolResult{}, ctx.Err()
			}
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				return tool.ToolResult{}, err
			}
		}
		snapshot, err := p.Read(in.ID)
		out, err := result(snapshot, err)
		if err == nil {
			out.Metadata = map[string]any{"stdout_artifact": snapshot.StdoutArtifact, "stderr_artifact": snapshot.StderrArtifact}
		}
		return out, err
	default:
		return tool.ToolResult{}, fmt.Errorf("unknown process action %q", in.Action)
	}
}

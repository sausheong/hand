package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/harness/runtime"
)

func (h *ExtensionHost) transformToolContext(ctx context.Context, input []runtime.ToolContextText) ([]runtime.ToolContextText, error) {
	enabled, err := h.manager.ContextTransformEnabled()
	if err != nil {
		return nil, err
	}
	out := append([]runtime.ToolContextText(nil), input...)
	if !enabled {
		return out, nil
	}
	for start := 0; start < len(input); {
		batch := []extensions.ContextItem{}
		size := 0
		end := start
		for end < len(input) && len(batch) < 64 {
			text := input[end].Text
			if len(text) > 16<<10 {
				return nil, errors.New("tool context contribution exceeds extension transform limit of 16 KiB")
			}
			if size+len(text) > 64<<10 {
				break
			}
			batch = append(batch, extensions.ContextItem{ID: fmt.Sprintf("context-%d", input[end].Index), Kind: "tool_result", Text: text})
			size += len(text)
			end++
		}
		transformed, diagnostics, err := h.manager.Transform(ctx, batch)
		if err != nil {
			return nil, err
		}
		for _, diagnostic := range diagnostics {
			slog.Warn("optional extension context transform failed", "extension", diagnostic.ID, "error", diagnostic.Error)
		}
		for i, item := range transformed {
			out[start+i].Text = item.Text
		}
		start = end
	}
	return out, nil
}

// attachRuntimeContext is called only before serving this controller. Runtime
// owns each hook invocation; acquiring another application reservation here
// would deadlock the active turn. Reload already uses the shared owner gate.
func (h *ExtensionHost) attachRuntimeContext() {
	rt := h.controller.Rt
	if rt == nil {
		return
	}
	previous := rt.AgentLoop.Hooks.TransformToolContext
	rt.AgentLoop.Hooks.TransformToolContext = func(ctx context.Context, input []runtime.ToolContextText) ([]runtime.ToolContextText, error) {
		if previous != nil {
			var err error
			input, err = previous(ctx, input)
			if err != nil {
				return nil, err
			}
		}
		return h.transformToolContext(ctx, input)
	}
}

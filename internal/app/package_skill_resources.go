package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

// packageResourceSkillTool extends only the selected package's data loader.
// It never delegates auxiliary reads to unrestricted host filesystem tools.
type packageResourceSkillTool struct {
	base tool.Tool
	rt   *runtime.Runtime
}

func (t *packageResourceSkillTool) Name() string { return "load_skill" }
func (t *packageResourceSkillTool) Description() string {
	return t.base.Description() + " For an explicitly selected package skill, optionally supply resource to read a declared text file relative to that skill. Reads verify the pinned package hash and never execute resources."
}
func (t *packageResourceSkillTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Exact selected skill name"},"resource":{"type":"string","description":"Optional declared text resource relative to the package skill; no execution"}},"required":["name"],"additionalProperties":false}`)
}
func (t *packageResourceSkillTool) IsConcurrencySafe(raw json.RawMessage) bool {
	return t.base.IsConcurrencySafe(raw)
}
func (t *packageResourceSkillTool) Execute(ctx context.Context, raw json.RawMessage) (tool.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return tool.ToolResult{}, err
	}
	var input struct {
		Name     string  `json:"name"`
		Resource *string `json:"resource"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return tool.ToolResult{Error: "invalid skill request"}, nil
	}
	if decoder.Decode(new(any)) != io.EOF || input.Name == "" {
		return tool.ToolResult{Error: "one skill request with a name is required"}, nil
	}
	selected, ok := t.rt.Skills.(*packageSkills)
	authored := false
	if ok && selected.base != nil {
		_, authored = selected.base.Get(input.Name)
	}
	if input.Resource == nil {
		if ok && !authored {
			for _, entry := range selected.entries {
				if entry.name == input.Name {
					return tool.ToolResult{Output: packageSkillBody(entry)}, nil
				}
			}
		}
		// Keep ordinary skills on the original execution backend. In
		// container mode this is the worker proxy, never a host fallback.
		return t.base.Execute(ctx, raw)
	}
	if !ok {
		return tool.ToolResult{Error: "no package skills selected"}, nil
	}
	if authored {
		return tool.ToolResult{Error: "skill belongs to the authored provider; package resources do not override it"}, nil
	}
	for _, entry := range selected.entries {
		if entry.name != input.Name {
			continue
		}
		resource, err := entry.resource.ReadRelativeResource(ctx, *input.Resource)
		if err != nil {
			return tool.ToolResult{Error: err.Error()}, nil
		}
		return tool.ToolResult{Output: fmt.Sprintf("Package resource: %s/%s\nPackage digest: %s\nResource SHA-256: %s\nResource base directory: %q\nRead-only data; no execution permission granted.\n\n%s", resource.Package, resource.Path, resource.PackageDigest, resource.SHA256, resource.BaseDirectory, resource.Text)}, nil
	}
	return tool.ToolResult{Error: "package skill not selected"}, nil
}

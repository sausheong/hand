package agentio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/sausheong/harness/tool"
)

// maxDiffLines bounds how many combined old+new lines lineDiff will
// actually diff line-by-line. Above this, a full-file rewrite would
// dump thousands of lines into an approval prompt for no benefit — a
// one-line size summary is more useful than that flood.
const maxDiffLines = 4000

// diffContextLines is how many unchanged lines are shown around a
// changed region, on each side.
const diffContextLines = 2

// splitLines splits s on "\n", but returns a nil (zero-length) slice for
// an empty string instead of strings.Split's [""] — otherwise an empty
// old file would show up as one phantom blank line in the diff.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lineDiff renders a unified-diff-style preview of oldText -> newText:
// lines prefixed "- " (removed), "+ " (added), or "  " (context).
//
// Separate edits produce separate unified hunks. Input line and byte limits
// bound preview work; larger changes remain explicitly labelled as omitted.
func lineDiff(oldText, newText string) string {
	if oldText == newText {
		return "(no changes)"
	}

	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	if len(oldLines)+len(newLines) > maxDiffLines {
		return fmt.Sprintf("(large change: %d lines -> %d lines, diff omitted)", len(oldLines), len(newLines))
	}

	if len(oldText)+len(newText) > 2<<20 {
		return "(large change: diff exceeds 2 MiB preview limit)"
	}
	unified := udiff.Unified("before", "after", oldText, newText)
	lines := strings.Split(strings.TrimRight(unified, "\n"), "\n")
	var output []string
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
			line = line[:1] + " " + line[1:]
		}
		output = append(output, line)
	}
	return strings.Join(output, "\n")
}

// resolvePath mirrors tools/file's own path resolution (ExpandHome, then
// join against the workspace when relative) so the preview reads the
// same file the tool will actually touch.
func resolvePath(workspace, path string) string {
	path = tool.ExpandHome(path)
	if workspace != "" && !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	return path
}

// buildPreview returns a human-readable preview of what a gated tool
// call is about to do, for display on the approval prompt. Empty means
// nothing to preview — used when the input can't be parsed or, for
// edit_file, when the target file can't be read; this is a best-effort
// preview, not a validation step, so a failure here just means no
// preview instead of blocking the prompt.
func buildPreview(workspace, toolName string, input json.RawMessage) string {
	switch toolName {
	case "process":
		return "Background process control (host permissions):\n" + string(input)
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return ""
		}
		return "$ " + in.Command

	case "write_file":
		var in struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return ""
		}
		path := resolvePath(workspace, in.Path)
		oldContent := ""
		if data, err := os.ReadFile(path); err == nil {
			oldContent = string(data)
		}
		return "Path: " + in.Path + "\n" + lineDiff(oldContent, in.Content)

	case "edit_file":
		var in struct {
			Path      string `json:"path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return ""
		}
		path := resolvePath(workspace, in.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		oldContent := string(data)
		newContent := strings.Replace(oldContent, in.OldString, in.NewString, 1)
		return "Path: " + in.Path + "\n" + lineDiff(oldContent, newContent)

	default:
		return ""
	}
}

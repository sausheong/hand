package agentio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
// This is a common-prefix/common-suffix diff with context, not a
// minimal LCS diff — deliberately. LLM-driven file edits are
// overwhelmingly "change one contiguous region," which this handles
// well, in O(n) time with no pathological blowup risk, unlike a naive
// LCS table over large files.
func lineDiff(oldText, newText string) string {
	if oldText == newText {
		return "(no changes)"
	}

	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	if len(oldLines)+len(newLines) > maxDiffLines {
		return fmt.Sprintf("(large change: %d lines -> %d lines, diff omitted)", len(oldLines), len(newLines))
	}

	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	oldEnd, newEnd := len(oldLines), len(newLines)
	for oldEnd > prefix && newEnd > prefix && oldLines[oldEnd-1] == newLines[newEnd-1] {
		oldEnd--
		newEnd--
	}

	var b strings.Builder
	ctxStart := max(prefix-diffContextLines, 0)
	for i := ctxStart; i < prefix; i++ {
		b.WriteString("  " + oldLines[i] + "\n")
	}
	for i := prefix; i < oldEnd; i++ {
		b.WriteString("- " + oldLines[i] + "\n")
	}
	for i := prefix; i < newEnd; i++ {
		b.WriteString("+ " + newLines[i] + "\n")
	}
	ctxEndOld := min(oldEnd+diffContextLines, len(oldLines))
	for i := oldEnd; i < ctxEndOld; i++ {
		b.WriteString("  " + oldLines[i] + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
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
		return lineDiff(oldContent, in.Content)

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
		return lineDiff(oldContent, newContent)

	default:
		return ""
	}
}

package agentio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineDiff_AddedLine(t *testing.T) {
	got := lineDiff("a\nb\n", "a\nb\nc\n")
	if !strings.Contains(got, "+ c") {
		t.Fatalf("diff = %q, want it to contain \"+ c\"", got)
	}
	if strings.Contains(got, "- ") {
		t.Fatalf("diff = %q, want no removed lines", got)
	}
}

func TestLineDiff_RemovedLine(t *testing.T) {
	got := lineDiff("a\nb\nc\n", "a\nb\n")
	if !strings.Contains(got, "- c") {
		t.Fatalf("diff = %q, want it to contain \"- c\"", got)
	}
}

func TestLineDiff_ChangedRegion(t *testing.T) {
	got := lineDiff("a\nb\nc\n", "a\nX\nc\n")
	if !strings.Contains(got, "- b") || !strings.Contains(got, "+ X") {
		t.Fatalf("diff = %q, want it to contain \"- b\" and \"+ X\"", got)
	}
}

func TestLineDiff_NewFileFromEmpty(t *testing.T) {
	got := lineDiff("", "hello\nworld\n")
	if !strings.Contains(got, "+ hello") || !strings.Contains(got, "+ world") {
		t.Fatalf("diff = %q, want both new lines added", got)
	}
	if strings.Contains(got, "- ") {
		t.Fatalf("diff = %q, want no removed lines for a brand new file", got)
	}
}

func TestLineDiff_IdenticalContent(t *testing.T) {
	got := lineDiff("same\n", "same\n")
	if got != "(no changes)" {
		t.Fatalf("diff = %q, want \"(no changes)\"", got)
	}
}

func TestLineDiff_LargeChangeFallsBackToSummary(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 3000; i++ {
		oldLines = append(oldLines, "old line")
		newLines = append(newLines, "new line")
	}
	got := lineDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if strings.Contains(got, "+ new line\n+ new line") {
		t.Fatal("expected large diffs to fall back to a summary, not a full line-by-line dump")
	}
	if !strings.Contains(got, "3000") {
		t.Fatalf("diff = %q, want it to mention the line counts", got)
	}
}

func TestBuildPreview_Bash(t *testing.T) {
	got := buildPreview("/tmp/workspace", "bash", json.RawMessage(`{"command":"go test ./..."}`))
	if got != "$ go test ./..." {
		t.Fatalf("preview = %q, want \"$ go test ./...\"", got)
	}
}

func TestBuildPreview_WriteFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	input, _ := json.Marshal(map[string]string{"path": "new.txt", "content": "hello\n"})

	got := buildPreview(dir, "write_file", input)
	if !strings.Contains(got, "+ hello") {
		t.Fatalf("preview = %q, want it to show the new content as added", got)
	}
}

func TestBuildPreview_WriteFile_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	input, _ := json.Marshal(map[string]string{"path": "existing.txt", "content": "new\n"})

	got := buildPreview(dir, "write_file", input)
	if !strings.Contains(got, "- old") || !strings.Contains(got, "+ new") {
		t.Fatalf("preview = %q, want it to show old removed and new added", got)
	}
}

func TestBuildPreview_EditFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("foo bar baz\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	input, _ := json.Marshal(map[string]string{"path": "f.txt", "old_string": "bar", "new_string": "QUX"})

	got := buildPreview(dir, "edit_file", input)
	if !strings.Contains(got, "- foo bar baz") || !strings.Contains(got, "+ foo QUX baz") {
		t.Fatalf("preview = %q, want it to show the replaced line", got)
	}
}

func TestBuildPreview_EditFile_UnreadableFileOmitsPreview(t *testing.T) {
	dir := t.TempDir()
	input, _ := json.Marshal(map[string]string{"path": "does-not-exist.txt", "old_string": "x", "new_string": "y"})

	got := buildPreview(dir, "edit_file", input)
	if got != "" {
		t.Fatalf("preview = %q, want empty when the file can't be read", got)
	}
}

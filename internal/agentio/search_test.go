package agentio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
)

func TestSearchTool_NameGlobMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a")
	writeFile(t, dir, "b.txt", "not go")

	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"name_glob":"*.go"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "a.go") {
		t.Errorf("Output = %q, want it to contain a.go", res.Output)
	}
	if strings.Contains(res.Output, "b.txt") {
		t.Errorf("Output = %q, want it not to contain b.txt", res.Output)
	}
}

func TestSearchTool_ContentMatchFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "line one\nfindme here\nline three")

	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"content":"findme"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := filepath.Join(dir, "a.go") + ":2:findme here"
	if !strings.Contains(res.Output, want) {
		t.Errorf("Output = %q, want it to contain %q", res.Output, want)
	}
}

func TestSearchTool_MaxResultsCapsWalk(t *testing.T) {
	dir := t.TempDir()
	for i := range 10 {
		writeFile(t, dir, fmt.Sprintf("f%d.go", i), "package a")
	}

	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"name_glob":"*.go","max_results":3}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := strings.Count(res.Output, "\n") + 1
	if got != 3 {
		t.Errorf("got %d results, want exactly 3 (max_results cap)", got)
	}
}

func TestSearchTool_BinaryFileSkippedForContentSearch(t *testing.T) {
	dir := t.TempDir()
	binData := append([]byte("findme"), 0, 1, 2)
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), binData, 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"content":"findme"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output != "no matches" {
		t.Errorf("Output = %q, want \"no matches\" (binary file should be skipped)", res.Output)
	}
}

func TestSearchTool_OversizedFileSkippedForContentButNotNameGlob(t *testing.T) {
	dir := t.TempDir()
	big := bytes.Repeat([]byte("findme\n"), 20000) // well over 64 KiB
	if err := os.WriteFile(filepath.Join(dir, "big.go"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &agentio.SearchTool{WorkDir: dir}

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"content":"findme"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Output != "no matches" {
		t.Errorf("content search Output = %q, want \"no matches\" (oversized file should be skipped)", res.Output)
	}

	res, err = tool.Execute(context.Background(), json.RawMessage(`{"name_glob":"*.go"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "big.go") {
		t.Errorf("name_glob search Output = %q, want it to contain big.go (size cap shouldn't apply)", res.Output)
	}
}

func TestSearchTool_MissingBothParametersErrors(t *testing.T) {
	tool := &agentio.SearchTool{WorkDir: t.TempDir()}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Error, "name_glob") || !strings.Contains(res.Error, "content") {
		t.Errorf("Error = %q, want it to mention both name_glob and content", res.Error)
	}
}

// Regression: filepath.WalkDir's callback swallows every walk error
// (including the root-stat error for a path that doesn't exist), so
// without an explicit upfront check a typo'd path silently produced
// the same "no matches" output as a valid, empty directory.
func TestSearchTool_NonexistentPathReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"totally/bogus/dir","name_glob":"*"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected an error for a nonexistent path, got none (silently reported as \"no matches\"?)")
	}
	if res.Output == "no matches" {
		t.Fatal("a nonexistent path must not be indistinguishable from a valid, empty result")
	}
}

func TestSearchTool_PathIsNotADirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"not-a-dir.txt","name_glob":"*"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Error, "not a directory") {
		t.Errorf("Error = %q, want it to say the path isn't a directory", res.Error)
	}
}

func TestSearchTool_PathOutsideWorkDirRejected(t *testing.T) {
	dir := t.TempDir()
	tool := &agentio.SearchTool{WorkDir: dir}
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"../../etc","name_glob":"*"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected an error for a path outside WorkDir, got none")
	}
	if !strings.Contains(res.Error, "outside workspace") {
		t.Errorf("Error = %q, want it to report the path as outside the workspace (from tool.ValidatePathInWorkDir)", res.Error)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

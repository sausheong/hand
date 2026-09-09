package agentio_test

import (
	"context"
	"encoding/json"
	"github.com/sausheong/hand/internal/agentio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchLargeSourceMustNotDisappear(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "large.go", strings.Repeat("a line\n", 20000)+"needle\n")
	r, err := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"needle"}`))
	if err != nil || r.Error != "" || !strings.Contains(r.Output, "needle") {
		t.Fatalf("large file silently lost: %+v %v", r, err)
	}
}

func TestSearchInvalidGlobMustError(t *testing.T) {
	r, _ := (&agentio.SearchTool{WorkDir: t.TempDir()}).Execute(context.Background(), json.RawMessage(`{"name_glob":"["}`))
	if r.Error == "" {
		t.Fatal("invalid glob silently treated as no matches")
	}
}

func TestSearchCancellationMustBeExplicit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := (&agentio.SearchTool{WorkDir: t.TempDir()}).Execute(ctx, json.RawMessage(`{"content":"needle"}`))
	if r.Metadata["cancelled"] != true || r.Output == "no matches" {
		t.Fatalf("cancelled search misreported: %+v", r)
	}
}

func TestSearchNestedIgnoreAndOverrides(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"src", "vendor", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, dir, ".gitignore", "*.log\nvendor/\n!vendor/keep.txt\n")
	writeFile(t, dir, "src/.gitignore", "!keep.log\n")
	writeFile(t, dir, "src/keep.log", "needle")
	writeFile(t, dir, "src/drop.log", "needle")
	writeFile(t, dir, "vendor/keep.txt", "needle")
	writeFile(t, dir, "node_modules/pkg.txt", "needle")
	writeFile(t, dir, ".hidden", "needle")
	for _, tc := range []struct {
		input  string
		want   []string
		absent []string
	}{
		{`{"content":"needle"}`, []string{"src/keep.log"}, []string{"src/drop.log", "vendor/keep.txt", "node_modules/pkg.txt", ".hidden"}},
		{`{"path":"src","content":"needle"}`, []string{"src/keep.log"}, []string{"src/drop.log"}},
		{`{"content":"needle","include_ignored":true,"include_hidden":true}`, []string{"src/drop.log", "vendor/keep.txt", "node_modules/pkg.txt", ".hidden"}, nil},
	} {
		r, _ := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(tc.input))
		for _, want := range tc.want {
			if !strings.Contains(r.Output, want) {
				t.Errorf("%s missing %s: %s", tc.input, want, r.Output)
			}
		}
		for _, absent := range tc.absent {
			if strings.Contains(r.Output, absent) {
				t.Errorf("ignored %s appeared: %s", absent, r.Output)
			}
		}
	}
}

func TestSearchLongLineReportsIncompleteAndBoundsOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "long.txt", strings.Repeat("x", 2*1024*1024)+"needle")
	r, _ := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"needle","max_bytes":1024}`))
	if r.Metadata["complete"] != false || r.Metadata["issue_count"] != 1 {
		t.Fatalf("long line hidden: %+v", r)
	}
	if len(r.Output) > 1024 {
		t.Fatalf("output budget violated: %d", len(r.Output))
	}
	writeFile(t, dir, "many.txt", strings.Repeat("needle\n", 10000))
	r, _ = (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"name_glob":"many.txt","content":"needle","max_bytes":1024}`))
	if r.Metadata["truncated"] != true || len(r.Output) > 1024 {
		t.Fatalf("result budget violated: %+v", r)
	}
}

func TestSearchReportsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readable.txt", "needle")
	writeFile(t, dir, "unreadable.txt", "needle")
	name := filepath.Join(dir, "unreadable.txt")
	if err := os.Chmod(name, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(name, 0600)
	r, _ := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"needle"}`))
	if r.Metadata["complete"] != false || r.Metadata["issue_count"] != 1 {
		t.Fatalf("unreadable file not reported (run this fixture as an unprivileged user): %+v", r)
	}
	if !strings.Contains(r.Output, "readable.txt:1:needle") {
		t.Fatalf("readable partial result lost: %+v", r)
	}
}

func TestSearchDoesNotFollowFileSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", "needle")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	r, _ := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"needle"}`))
	if strings.Contains(r.Output, ":1:needle") || r.Metadata["excluded"].(map[string]int)["non_regular"] != 1 {
		t.Fatalf("symlink scope failure: %+v", r)
	}
}

func TestSearchWorksWithoutExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "needle")
	r, _ := (&agentio.SearchTool{WorkDir: dir}).Execute(context.Background(), json.RawMessage(`{"content":"needle"}`))
	if r.Error != "" || !strings.Contains(r.Output, ":1:needle") {
		t.Fatalf("external dependency: %+v", r)
	}
}

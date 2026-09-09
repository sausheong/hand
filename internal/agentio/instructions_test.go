package agentio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstructionDiscoveryPrecedenceAndProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	workspace := filepath.Join(root, "nested")
	for path, body := range map[string]string{filepath.Join(home, ".hand", "AGENTS.md"): "personal rule", filepath.Join(root, "AGENTS.md"): "broad rule", filepath.Join(workspace, "HAND.md"): "near rule", filepath.Join(workspace, "AGENTS.md"): "shadowed body"} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report := DiscoverInstructions(workspace)
	if len(report.Sources) != 3 {
		t.Fatalf("sources: %+v", report)
	}
	for i, want := range []string{"personal rule", "broad rule", "near rule"} {
		if report.Sources[i].Body != want {
			t.Fatalf("precedence: %+v", report)
		}
	}
	rendered := report.Format()
	if strings.Contains(rendered, "shadowed body") || !strings.Contains(rendered, "shadowed by") || !strings.Contains(rendered, workspace) || !strings.Contains(rendered, "does not grant tool permissions") {
		t.Fatal(rendered)
	}
}

func TestInstructionDiscoveryExclusions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, kind := range []string{"oversized", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "HAND.md")
			var err error
			switch kind {
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("x", instructionFileLimit+1)), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "symlink":
				err = os.Symlink("AGENTS.md", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("must not silently fall back"), 0600); err != nil {
				t.Fatal(err)
			}
			r := DiscoverInstructions(dir)
			if len(r.Sources) != 0 || len(r.Diagnostics) != 2 {
				t.Fatalf("report: %+v", r)
			}
		})
	}
}

func TestInstructionDiscoveryTotalBound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	for i := 0; i < 6; i++ {
		dir = filepath.Join(dir, "child")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "HAND.md"), []byte(strings.Repeat("x", instructionFileLimit)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := DiscoverInstructions(dir)
	if len(r.Sources) != 4 || len(r.Diagnostics) != 2 {
		t.Fatalf("sources=%d diagnostics=%v", len(r.Sources), r.Diagnostics)
	}
}

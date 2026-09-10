package packages

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitFixtureCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Hand test", "GIT_AUTHOR_EMAIL=hand@example.invalid", "GIT_COMMITTER_NAME=Hand test", "GIT_COMMITTER_EMAIL=hand@example.invalid")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Git fixture %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func TestGitImportPinnedCommitIgnoresWorkingTreeAndInstalls(t *testing.T) {
	source := writeFixture(t)
	_, pin, err := VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	gitFixtureCommand(t, source, "init", "-q")
	gitFixtureCommand(t, source, "add", "--", ManifestName, "note.py")
	gitFixtureCommand(t, source, "commit", "-qm", "package fixture")
	commit := gitFixtureCommand(t, source, "rev-parse", "HEAD")
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(source, ".git", "hooks", "post-checkout")
	if err = os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "note.py"), []byte("uncommitted change"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "untracked"), []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := privateStageParent(t)
	snapshot, err := ImportGit(context.Background(), source, parent, commit, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	_, got, err := VerifyDirectory(context.Background(), snapshot.Directory())
	if err != nil || got != pin {
		t.Fatal("Git snapshot pin mismatch", got, err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Git hook executed")
	}
	raw, err := os.ReadFile(filepath.Join(source, "note.py"))
	if err != nil || string(raw) != "uncommitted change" {
		t.Fatal("import changed worktree")
	}
	store, err := OpenStore(privateStageParent(t), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	review, err := store.PrepareSnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	approveChange(t, store, review)
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	assertStoredOrigin(t, store, Origin{Kind: "git", Location: source, Reference: commit})
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("Git import left temporary files", entries, err)
	}
}
func TestGitImportRejectsRefsLinksAndWrongPin(t *testing.T) {
	source := writeFixture(t)
	_, pin, err := VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	gitFixtureCommand(t, source, "init", "-q")
	gitFixtureCommand(t, source, "add", "--", ManifestName, "note.py")
	gitFixtureCommand(t, source, "commit", "-qm", "package fixture")
	commit := gitFixtureCommand(t, source, "rev-parse", "HEAD")
	for _, mode := range []string{"branch", "wrong-pin", "symlink", "submodule"} {
		t.Run(mode, func(t *testing.T) {
			selected, want := commit, pin
			switch mode {
			case "branch":
				selected = "HEAD"
			case "wrong-pin":
				want = strings.Repeat("0", 64)
			case "symlink":
				if err := os.Symlink("/etc/passwd", filepath.Join(source, "link")); err != nil {
					t.Fatal(err)
				}
				gitFixtureCommand(t, source, "add", "link")
				gitFixtureCommand(t, source, "commit", "-qm", "link fixture")
				selected = gitFixtureCommand(t, source, "rev-parse", "HEAD")
			case "submodule":
				gitFixtureCommand(t, source, "read-tree", commit)
				gitFixtureCommand(t, source, "update-index", "--add", "--cacheinfo", "160000,"+commit+",nested")
				gitFixtureCommand(t, source, "commit", "-qm", "submodule fixture")
				selected = gitFixtureCommand(t, source, "rev-parse", "HEAD")
			}
			parent := privateStageParent(t)
			snapshot, err := ImportGit(context.Background(), source, parent, selected, want)
			if err == nil || snapshot != nil {
				t.Fatal("unsafe Git import accepted", snapshot, err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed Git import left output", entries, err)
			}
		})
	}
}

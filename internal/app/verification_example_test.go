package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/packages"
	"github.com/sausheong/harness/runtime"
)

func TestVerificationExampleHostCommandAndFinishObserver(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "verification-hook")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-verification-hook")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	workspace := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	git("init", "-q")
	file := filepath.Join(workspace, "tracked.txt")
	if err := os.WriteFile(file, []byte("clean\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "tracked.txt")
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := packages.Manifest{Schema: 1, Name: "verification-hook", Version: "1.0.0", Compatibility: packages.Compatibility{MinimumHand: "0.1.0", ExtensionProtocol: 1}, Files: []packages.File{{Path: "verification-hook", Kind: "extension", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), Executable: true}}, Extensions: []packages.Extension{{Name: "verification-hook", Entrypoint: "verification-hook", Capabilities: []string{"commands", "lifecycle"}}}}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Dir(binary)
	if err = os.WriteFile(filepath.Join(source, packages.ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin, err := packages.VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := packages.OpenStore(filepath.Join(t.TempDir(), "packages"), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	change, err := store.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := change.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, change, approval); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	snapshots := filepath.Join(t.TempDir(), "snapshots")
	selected, err := ReviewPackageExtensions(ctx, store, []string{manifest.Name}, workspace, snapshots, nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &runtime.Runtime{}
	c := &Controller{Rt: rt}
	host, err := c.ActivateExtensions(ctx, selected, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	result, err := host.Execute(ctx, "verification-hook", "verify-whitespace", "")
	if err != nil || len(result.Blocks) != 1 || !strings.Contains(result.Blocks[0].Text, "checks passed") {
		t.Fatal(result, err)
	}
	diagnostics, err := host.manager.NotifyLifecycle(ctx, "run.finish", []byte(`{"reason":"completed"}`))
	if err != nil || len(diagnostics) != 0 {
		t.Fatal(diagnostics, err)
	}
	if err = os.WriteFile(file, []byte("bad whitespace \n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = host.Execute(ctx, "verification-hook", "verify-whitespace", "")
	if err != nil || len(result.Blocks) != 1 || !strings.Contains(result.Blocks[0].Text, "did not pass") {
		t.Fatal(result, err)
	}
	diagnostics, err = host.manager.NotifyLifecycle(ctx, "run.finish", []byte(`{"reason":"completed"}`))
	if err != nil || len(diagnostics) != 1 || diagnostics[0].ID != "verification-hook" {
		t.Fatal(diagnostics, err)
	}
	// The actual attached finish observer must return normally on failed checks.
	if rt.AgentLoop.Hooks.OnRunFinish == nil {
		t.Fatal("finish observer not attached")
	}
	rt.AgentLoop.Hooks.OnRunFinish(ctx, "completed")
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(snapshots)
	if err != nil || len(entries) != 0 {
		t.Fatal("snapshots survived shutdown", entries, err)
	}
}

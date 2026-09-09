package verification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/process"
)

func options(t *testing.T, command string) Options {
	t.Helper()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "input"), []byte("user content"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := checkpoints.OpenStore(filepath.Join(t.TempDir(), "checkpoints"), work, checkpoints.DefaultStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	output, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	return Options{Workspace: work, Profile: "test", ProfileDigest: strings.Repeat("a", 64), Argv: []string{"/bin/sh", "-c", command}, Limits: checkpoints.DefaultLimits(), Backend: execution.Host{Workspace: work}, Checkpoints: store, Output: output}
}
func current(t *testing.T, o Options) *checkpoints.Snapshot {
	t.Helper()
	s, err := checkpoints.Capture(context.Background(), o.Workspace, o.Limits)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestPassingCommandBoundToSnapshot(t *testing.T) {
	o := options(t, "cat input; printf diagnostic >&2")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	v := r.View()
	if v.ExitCode != 0 || v.Stdout != "user content" || v.Stderr != "diagnostic" || v.Before != v.After || r.Status(current(t, o)) != "passed" {
		t.Fatal(v)
	}
	if v.StdoutBytes != int64(len("user content")) {
		t.Fatal("incorrect output count")
	}
	if _, err = o.Checkpoints.Load(context.Background(), v.Before); err != nil {
		t.Fatal("missing durable snapshot", err)
	}
	v.Command[0] = "mutated"
	if r.View().Command[0] != "/bin/sh" {
		t.Fatal("record is mutable")
	}
	if err = os.WriteFile(filepath.Join(o.Workspace, "input"), []byte("later user edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if r.Status(current(t, o)) != "stale" {
		t.Fatal("later edit remained verified")
	}
}
func TestCommandMutationNotVerified(t *testing.T) {
	o := options(t, "printf changed > input")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if r.View().Before == r.View().After || r.Status(current(t, o)) != "stale" {
		t.Fatal(r.View())
	}
}
func TestFailureRetainsExitAndOutput(t *testing.T) {
	o := options(t, "printf failure >&2; exit 7")
	r, err := Run(context.Background(), o)
	if err == nil || r == nil {
		t.Fatal(r, err)
	}
	if r.View().ExitCode != 7 || r.View().Stderr != "failure" || r.Status(current(t, o)) != "failed" {
		t.Fatal(r.View())
	}
}
func TestAdmissionFailureHasNoCommandEffect(t *testing.T) {
	o := options(t, "touch executed")
	o.Backend = execution.Host{Workspace: t.TempDir()}
	r, err := Run(context.Background(), o)
	if err == nil || r != nil {
		t.Fatal(r, err)
	}
	if _, err = os.Stat(filepath.Join(o.Workspace, "executed")); !os.IsNotExist(err) {
		t.Fatal("command ran")
	}
	o = options(t, "touch executed")
	o.Limits.MaxFileBytes = 1
	r, err = Run(context.Background(), o)
	if err == nil || r != nil {
		t.Fatal(r, err)
	}
	if _, err = os.Stat(filepath.Join(o.Workspace, "executed")); !os.IsNotExist(err) {
		t.Fatal("command ran after snapshot failure")
	}
}
func TestAfterCaptureFailureCannotPass(t *testing.T) {
	o := options(t, "printf verylargecontent > input")
	o.Limits.MaxFileBytes = 12
	r, err := Run(context.Background(), o)
	if err == nil || r == nil {
		t.Fatal(r, err)
	}
	if r.View().ExitCode != 0 || r.Status(nil) != "unverified" || r.View().SnapshotError == "" {
		t.Fatal(r.View())
	}
}
func TestCancelledBeforeCommand(t *testing.T) {
	o := options(t, "touch executed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Run(ctx, o)
	if err == nil || r != nil {
		t.Fatal(r, err)
	}
	if _, err = os.Stat(filepath.Join(o.Workspace, "executed")); !os.IsNotExist(err) {
		t.Fatal("cancelled command ran")
	}
}

func TestLargeOutputAndProfileChange(t *testing.T) {
	o := options(t, "head -c 100000 /dev/zero")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	v := r.View()
	if v.StdoutBytes != 100000 || !v.StdoutTruncated || v.StdoutArtifact.Path == "" || v.StdoutArtifact.SHA256 == "" || v.StdoutArtifact.Error != "" {
		t.Fatal(v)
	}
	if r.StatusFor(current(t, o), strings.Repeat("b", 64)) != "stale" {
		t.Fatal("changed profile reused evidence")
	}
	if r.StatusFor(current(t, o), o.ProfileDigest) != "passed" {
		t.Fatal("unchanged scope lost evidence")
	}
}

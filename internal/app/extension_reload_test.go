package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/extensions"
)

func TestReviewedReloadReuseFailureAndRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, out)
	}
	workspace := t.TempDir()
	cfg := extensions.LaunchConfig{Name: "task-note", Executable: binary, Workspace: workspace, Capabilities: []string{"commands", "questions", "state", "context.transform"}}
	review, err := extensions.ReviewHostLaunch(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	selected := ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "private"), Identities: map[string]string{"task-note": "example/note"}, Reviews: []extensions.LaunchReview{review}}
	controller := &Controller{}
	host, err := controller.ActivateExtensions(ctx, selected, workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	write := func(s ExtensionStartup) (string, string) {
		t.Helper()
		raw, _ := json.Marshal(s)
		path := filepath.Join(t.TempDir(), "review.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		return path, hex.EncodeToString(sum[:])
	}
	path, digest := write(selected)
	report, err := host.ReloadFile(ctx, path, digest)
	if err != nil || !report.Committed || len(report.Reused) != 1 {
		t.Fatalf("reuse %+v %v", report, err)
	}
	cfg.Capabilities = []string{"commands"}
	bad, err := extensions.ReviewHostLaunch(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	changed := selected
	changed.Reviews = []extensions.LaunchReview{bad}
	path, digest = write(changed)
	report, err = host.ReloadFile(ctx, path, digest)
	if err == nil || report.Committed {
		t.Fatal("mismatched capability handshake committed")
	}
	path, digest = write(selected)
	report, err = host.ReloadFile(ctx, path, digest)
	if err != nil || len(report.Reused) != 1 {
		t.Fatal("old peer lost after failed staged handshake", report, err)
	}
	changed = selected
	changed.Identities = map[string]string{"task-note": "another/package"}
	path, digest = write(changed)
	if _, err = host.ReloadFile(ctx, path, digest); err == nil {
		t.Fatal("identity rebound")
	}
	empty := selected
	empty.Identities = map[string]string{}
	empty.Reviews = []extensions.LaunchReview{}
	path, digest = write(empty)
	report, err = host.ReloadFile(ctx, path, digest)
	if err != nil || !report.Committed || len(report.Removed) != 1 {
		t.Fatal("remove all failed", report, err)
	}
	entries, err := os.ReadDir(selected.SnapshotRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("removed peer snapshot leaked", err)
	}
}

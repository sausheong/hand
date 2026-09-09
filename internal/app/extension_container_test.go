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
	"github.com/sausheong/harness/execution"
)

func TestContainerExtensionActivationAndReviewedReload(t *testing.T) {
	image, socket, arch := os.Getenv("HARNESS_TEST_CONTAINER_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET"), os.Getenv("HAND_TEST_CONTAINER_ARCH")
	if image == "" || socket == "" || arch == "" {
		t.Skip("native container fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "viewer")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-tool-viewer")
	build.Dir = "../.."
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	workspace := t.TempDir()
	cfg := extensions.LaunchConfig{Name: "tool-viewer", Executable: binary, Workspace: workspace, Capabilities: []string{"commands", "presentation"}}
	boundary := extensions.ContainerLaunch{Docker: "/usr/local/bin/docker", Socket: socket, Image: image}
	review, err := extensions.ReviewContainerLaunch(ctx, cfg, boundary)
	if err != nil {
		t.Fatal(err)
	}
	selected := ExtensionStartup{Version: 1, SnapshotRoot: filepath.Join(t.TempDir(), "snapshots"), Reviews: []extensions.LaunchReview{review}, Identities: map[string]string{"tool-viewer": "fixture/viewer"}}
	backend := execution.Container{Docker: boundary.Docker, Socket: socket, Image: image, Workspace: workspace}
	c := &Controller{}
	if _, err = c.ActivateExtensions(ctx, selected, workspace, true); err == nil {
		t.Fatal("container review activated on host")
	}
	wrong := backend
	wrong.Network = true
	if _, err = c.ActivateExtensionsWithBackend(ctx, selected, workspace, wrong); err == nil {
		t.Fatal("runtime boundary mismatch accepted")
	}
	host, err := c.ActivateExtensionsWithBackend(ctx, selected, workspace, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	command := func() {
		t.Helper()
		result, err := host.Execute(ctx, "tool-viewer", "view-tool", `{"tool":"fixture","output":"isolated"}`)
		if err != nil || len(result.Blocks) == 0 {
			t.Fatal(result, err)
		}
	}
	command()
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
		t.Fatal(report, err)
	}
	changedBoundary := boundary
	changedBoundary.Network = true
	changedReview, err := extensions.ReviewContainerLaunch(ctx, cfg, changedBoundary)
	if err != nil {
		t.Fatal(err)
	}
	changed := selected
	changed.Reviews = []extensions.LaunchReview{changedReview}
	path, digest = write(changed)
	report, err = host.ReloadFile(ctx, path, digest)
	if err == nil || report.Committed {
		t.Fatal("reload changed runtime boundary", report, err)
	}
	command()
	changed.Reviews = nil
	changed.Identities = map[string]string{}
	path, digest = write(changed)
	report, err = host.ReloadFile(ctx, path, digest)
	if err != nil || !report.Committed || len(report.Removed) != 1 {
		t.Fatal(report, err)
	}
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
}

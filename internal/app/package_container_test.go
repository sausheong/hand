package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/sausheong/hand/internal/extensions"
	"github.com/sausheong/hand/internal/packages"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeContainerPackagePythonReviewAndLaunch(t *testing.T) {
	image, socket := os.Getenv("HAND_TEST_PYTHON_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	if image == "" || socket == "" {
		t.Skip("native Python image required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	body, err := os.ReadFile("../../examples/extensions/python-task-note/main.py")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err = os.WriteFile(filepath.Join(source, "main.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	requirement := packages.Runtime{Name: "python", Command: "python3", MinimumVersion: "3.9.0"}
	m := packages.Manifest{Schema: 1, Name: "container-note", Version: "1.0.0", Compatibility: packages.Compatibility{MinimumHand: "1.0.0", ExtensionProtocol: 1}, Runtimes: []packages.Runtime{requirement}, Files: []packages.File{{Path: "main.py", Kind: "extension", SHA256: hex.EncodeToString(hash[:]), Size: int64(len(body))}}, Extensions: []packages.Extension{{Name: "task-note", Entrypoint: "main.py", Runtime: "python", Capabilities: []string{"commands", "questions", "state", "context.transform"}}}}
	raw, _ := json.Marshal(m)
	if err = os.WriteFile(filepath.Join(source, packages.ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin, err := packages.VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := packages.OpenStore(filepath.Join(t.TempDir(), "store"), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	change, err := store.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := change.ApprovalDigest()
	if _, err = store.Apply(ctx, change, digest); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	boundary := extensions.ContainerLaunch{Docker: "/usr/local/bin/docker", Socket: socket, Image: image}
	runtimeReview, err := packages.ReviewContainerRuntime(ctx, requirement, boundary.Docker, socket, image, "/usr/local/bin/python3")
	if err != nil {
		t.Fatal(err)
	}
	runtimeDigest, _ := runtimeReview.Digest()
	approvals := []PackageContainerRuntimeApproval{{Package: m.Name, Review: runtimeReview, ApprovedDigest: runtimeDigest}}
	workspace, snapshots := t.TempDir(), filepath.Join(t.TempDir(), "snapshots")
	if _, err = ReviewContainerPackageExtensions(ctx, store, []string{m.Name}, workspace, snapshots, boundary, nil); err == nil {
		t.Fatal("missing runtime approval accepted")
	}
	wrong := boundary
	wrong.Image = "sha256:" + strings.Repeat("a", 64)
	if _, err = ReviewContainerPackageExtensions(ctx, store, []string{m.Name}, workspace, snapshots, wrong, approvals); err == nil {
		t.Fatal("probe bound to wrong image")
	}
	selected, err := ReviewContainerPackageExtensions(ctx, store, []string{m.Name}, workspace, snapshots, boundary, approvals)
	if err != nil {
		t.Fatal(err)
	}
	r := selected.Reviews[0]
	if r.Binary.SHA256 != hex.EncodeToString(hash[:]) || len(r.ImageInterpreter) != 3 || r.ImageInterpreter[0] != "/usr/local/bin/python3" || selected.Identities["task-note"] != "package:container-note:task-note" {
		t.Fatal(selected)
	}
	factory, err := extensions.NewContainerFactory(selected.Reviews, func(context.Context, extensions.LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	connection, err := factory(ctx, ctx, r.Specification)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err = connection.Initialize(ctx, "task-note", r.Specification.Capabilities); err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
}

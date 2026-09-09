package packages

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestContainerRuntimeReviewAndApproval(t *testing.T) {
	requirement := Runtime{Name: "python", Command: "python3", MinimumVersion: "3.9.0"}
	review, err := ReviewContainerRuntime(context.Background(), requirement, "/missing/docker", "/missing/socket", "sha256:"+strings.Repeat("a", 64), "/usr/local/bin/python3")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := review.Digest()
	if err != nil || len(digest) != 64 {
		t.Fatal(digest, err)
	}
	if _, err = ProbeContainerRuntime(context.Background(), review, ""); err == nil || !strings.Contains(err.Error(), "approval required") {
		t.Fatal(err)
	}
	changed := review
	changed.Image = "sha256:" + strings.Repeat("b", 64)
	if _, err = ProbeContainerRuntime(context.Background(), changed, digest); err == nil || !strings.Contains(err.Error(), "approval required") {
		t.Fatal("changed image admitted", err)
	}
	changed = review
	changed.Executable = "/tmp/python3"
	if _, err = changed.Digest(); err == nil {
		t.Fatal("mutable interpreter admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ProbeContainerRuntime(ctx, review, digest); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestNativeContainerPythonRuntimeVersion(t *testing.T) {
	image, socket := os.Getenv("HAND_TEST_PYTHON_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	if image == "" || socket == "" {
		t.Skip("explicit native Python image fixture required")
	}
	r, err := ReviewContainerRuntime(context.Background(), Runtime{Name: "python", Command: "python3", MinimumVersion: "3.9.0"}, "/usr/local/bin/docker", socket, image, "/usr/local/bin/python3")
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := r.Digest()
	version, err := ProbeContainerRuntime(context.Background(), r, digest)
	if err != nil || !strings.HasPrefix(version, "3.") {
		t.Fatal(version, err)
	}
	r.Requirement.MinimumVersion = "99.0.0"
	digest, _ = r.Digest()
	if _, err = ProbeContainerRuntime(context.Background(), r, digest); err == nil || !strings.Contains(err.Error(), "requires >=") {
		t.Fatal("incompatible runtime admitted", err)
	}
}

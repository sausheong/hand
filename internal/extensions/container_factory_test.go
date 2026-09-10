package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sausheong/hand/extension/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContainerReviewSeparatesBoundaryApproval(t *testing.T) {
	host := launchFixture(t)
	cfg := LaunchConfig{Name: "fixture", Executable: host.Binary.Path, Workspace: host.Workspace, Capabilities: []string{"commands"}}
	boundary := ContainerLaunch{Docker: "/usr/local/bin/docker", Socket: "/tmp/nonexistent.sock", Image: "sha256:" + strings.Repeat("a", 64)}
	r, err := ReviewContainerLaunch(context.Background(), cfg, boundary)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(context.Context, LaunchReview) error { return nil }
	if _, err = NewHostFactory(filepath.Join(t.TempDir(), "snapshots"), []LaunchReview{r}, admit); err == nil {
		t.Fatal("container review accepted by host")
	}
	if _, err = NewContainerFactory([]LaunchReview{host}, admit); err == nil {
		t.Fatal("host review accepted by container")
	}
	changed := cloneReview(r)
	changed.Container.Network = true
	if _, err = NewContainerFactory([]LaunchReview{changed}, admit); err == nil {
		t.Fatal("network change preserved approval")
	}
	denied := errors.New("fixture denied")
	factory, err := NewContainerFactory([]LaunchReview{r}, func(context.Context, LaunchReview) error { return denied })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = factory(context.Background(), context.Background(), r.Specification); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	factory, err = NewContainerFactory([]LaunchReview{r}, admit)
	if err != nil {
		t.Fatal(err)
	}
	if connection, err := factory(context.Background(), context.Background(), r.Specification); err == nil {
		connection.Close()
		t.Fatal("missing container socket fell back to host")
	}
}

func TestContainerExtensionRealExample(t *testing.T) {
	image, socket, arch := os.Getenv("HARNESS_TEST_CONTAINER_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET"), os.Getenv("HAND_TEST_CONTAINER_ARCH")
	if image == "" || socket == "" || arch == "" {
		t.Skip("native container fixture and explicit architecture required")
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
	review, err := ReviewContainerLaunch(ctx, LaunchConfig{Name: "tool-viewer", Executable: binary, Workspace: t.TempDir(), Capabilities: []string{"commands", "presentation"}}, ContainerLaunch{Docker: "/usr/local/bin/docker", Socket: socket, Image: image})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewContainerFactory([]LaunchReview{review}, func(context.Context, LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	connection, err := factory(ctx, ctx, review.Specification)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err = connection.Initialize(ctx, "tool-viewer", review.Specification.Capabilities); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"name": "view-tool", "arguments": `{"tool":"fixture","output":"container result"}`})
	raw, err := connection.Call(ctx, "command.execute", args)
	if err != nil || !strings.Contains(string(raw), "container result") {
		t.Fatal(string(raw), err)
	}
	boundary, _, _, _, _ := connection.Status()
	if !strings.Contains(boundary, image) || !strings.Contains(boundary, "network=false") {
		t.Fatal(boundary)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestContainerImageInterpreterReview(t *testing.T) {
	script := filepath.Join(t.TempDir(), "peer.py")
	if err := os.WriteFile(script, []byte("raise RuntimeError('must not execute during review')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := LaunchConfig{Name: "fixture", Executable: script, Workspace: t.TempDir(), ImageInterpreter: []string{"/usr/local/bin/python3", "-I", "-B"}, Capabilities: []string{"commands"}}
	boundary := ContainerLaunch{Docker: "/missing/docker", Socket: "/missing/socket", Image: "sha256:" + strings.Repeat("a", 64)}
	if _, err := ReviewHostLaunch(context.Background(), cfg); err == nil {
		t.Fatal("image interpreter admitted on host")
	}
	r, err := ReviewContainerLaunch(context.Background(), cfg, boundary)
	if err != nil {
		t.Fatal(err)
	}
	if r.Binary.Mode&0111 != 0 || len(r.ImageInterpreter) != 3 {
		t.Fatal(r)
	}
	changed := cloneReview(r)
	changed.ImageInterpreter[0] = "/bin/sh"
	if _, err = NewContainerFactory([]LaunchReview{changed}, func(context.Context, LaunchReview) error { return nil }); err == nil {
		t.Fatal("interpreter change retained approval")
	}
	for _, name := range []string{"python3", "/workspace/python", "/tmp/python", "/usr/../bin/python"} {
		cfg.ImageInterpreter = []string{name}
		if _, err := ReviewContainerLaunch(context.Background(), cfg, boundary); err == nil {
			t.Fatal("invalid interpreter admitted", name)
		}
	}
}

func TestContainerPythonImageInterpreter(t *testing.T) {
	image, socket := os.Getenv("HAND_TEST_PYTHON_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	if image == "" || socket == "" {
		t.Skip("native Python image fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source, err := filepath.Abs("../../examples/extensions/python-task-note/main.py")
	if err != nil {
		t.Fatal(err)
	}
	review, err := ReviewContainerLaunch(ctx, LaunchConfig{Name: "task-note", Executable: source, Workspace: t.TempDir(), ImageInterpreter: []string{"/usr/local/bin/python3", "-I", "-B"}, Capabilities: []string{"commands", "questions", "state", "context.transform"}}, ContainerLaunch{Docker: "/usr/local/bin/docker", Socket: socket, Image: image})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewContainerFactory([]LaunchReview{review}, func(context.Context, LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	connection, err := factory(ctx, ctx, review.Specification)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err = connection.SetCallbackHandler(func(_ context.Context, method string, raw json.RawMessage) (json.RawMessage, *protocol.Error) {
		if method != "user.question" {
			t.Fatal(method)
		}
		return json.RawMessage(`{"id":"note","cancelled":true}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Initialize(ctx, "task-note", review.Specification.Capabilities); err != nil {
		t.Fatal(err)
	}
	result, err := connection.Call(ctx, "command.execute", json.RawMessage(`{"name":"note","arguments":""}`))
	if err != nil || !strings.Contains(string(result), "Note cancelled.") {
		t.Fatal(string(result), err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
}

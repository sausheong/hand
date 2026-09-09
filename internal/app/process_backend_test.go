package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/process"
)

func TestNativeBackgroundContainerInputOutputAndShutdown(t *testing.T) {
	image, socket := os.Getenv("HARNESS_TEST_CONTAINER_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	if image == "" || socket == "" {
		t.Skip("native container fixture not configured")
	}
	workspace := t.TempDir()
	store, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "capture"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProcessesWithBackend(context.Background(), workspace, store, execution.Container{Docker: "/usr/local/bin/docker", Socket: socket, Image: image, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if !strings.Contains(p.Boundary(), "container") {
		t.Fatal("boundary absent")
	}
	info, err := p.Start(context.Background(), `set -eu; read line; test ! -e /var/run/docker.sock; ! touch /etc/outside; printf 'received:%s' "$line"`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = p.Send(ctx, info.ID, []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Wait(ctx, info.ID)
	if err != nil || snapshot.Running || snapshot.ExitCode != 0 || snapshot.Stdout != "received:hello" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	info, err = p.Start(context.Background(), "sleep 60 & wait")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	p.Close()
	if time.Since(start) > 5*time.Second {
		t.Fatal("shutdown exceeded cleanup bound")
	}
	snapshot, err = p.Read(info.ID)
	if err != nil || snapshot.Running {
		t.Fatalf("shutdown not joined: %+v %v", snapshot, err)
	}
}

package toolproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/toolworker"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
)

type hostTrap struct{ tool.Tool }

func (*hostTrap) Name() string { return "read_file" }
func (*hostTrap) Execute(context.Context, json.RawMessage) (tool.ToolResult, error) {
	panic("host execution must never occur")
}

type fixtureBackend struct {
	reply   execution.Result
	err     error
	request execution.Request
}

func (*fixtureBackend) Boundary() string { return "test container" }
func (b *fixtureBackend) Run(_ context.Context, r execution.Request) (execution.Result, error) {
	b.request = r
	return b.reply, b.err
}
func TestProxyPreservesLargeResponseAndImagesWithoutHostFallback(t *testing.T) {
	response := toolworker.Response{Version: 1, Result: tool.ToolResult{Output: strings.Repeat("x", 100000)}, Images: []llm.ImageContent{{MimeType: "image/png", Data: []byte("fixture")}}}
	raw, _ := json.Marshal(response)
	backend := &fixtureBackend{reply: execution.Result{Stdout: string(raw), ExitCode: 0}}
	proxy := &Tool{Workspace: t.TempDir(), Tool: &hostTrap{}, Backend: backend}
	result, err := proxy.Execute(context.Background(), json.RawMessage(`{"path":"file"}`))
	if err != nil || len(result.Output) != 100000 || len(result.Images) != 1 || string(result.Images[0].Data) != "fixture" {
		t.Fatalf("response lost: %v", err)
	}
	if backend.request.CaptureLimit != toolworker.MaxResponseBytes || len(backend.request.Argv) != 1 || backend.request.Argv[0] != "/hand-worker" {
		t.Fatal("invalid backend request")
	}
	for _, reply := range []execution.Result{{Stdout: `{"version":1}`, ExitCode: 0}, {Stdout: string(raw), StdoutTruncated: true}, {Stdout: string(raw), ExitCode: 1}, {Stdout: string(raw) + `{}`, ExitCode: 0}} {
		backend.reply = reply
		if _, err = proxy.Execute(context.Background(), json.RawMessage(`{"path":"file"}`)); err == nil {
			t.Fatal("invalid worker response accepted")
		}
	}
	backend.err = errors.New("unavailable")
	if _, err = proxy.Execute(context.Background(), json.RawMessage(`{"path":"file"}`)); err == nil {
		t.Fatal("backend failure hidden")
	}
}
func TestNativeProxyWorkerRoundtrip(t *testing.T) {
	image, socket, worker := os.Getenv("HARNESS_TEST_CONTAINER_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET"), os.Getenv("HAND_TEST_WORKER_BINARY")
	if image == "" || socket == "" || worker == "" {
		t.Skip("native tool proxy fixture not configured")
	}
	workspace := t.TempDir()
	if err := os.Chmod(workspace, 0777); err != nil {
		t.Fatal(err)
	}
	workerBytes, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	workerHash := sha256.Sum256(workerBytes)
	backend := execution.Container{WorkerSHA256: hex.EncodeToString(workerHash[:]), Docker: "/usr/local/bin/docker", Socket: socket, Image: image, Workspace: workspace, Writable: true, WorkerPath: worker}
	write := &Tool{Workspace: workspace, Tool: &file.WriteFileTool{WorkDir: "/must-not-execute-on-host"}, Backend: backend}
	result, err := write.Execute(context.Background(), json.RawMessage(`{"path":"proxy.txt","content":"through container"}`))
	if err != nil || result.Error != "" {
		t.Fatalf("write: %+v %v", result, err)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "proxy.txt"))
	if err != nil || string(raw) != "through container" {
		t.Fatalf("file: %q %v", raw, err)
	}
	read := &Tool{Workspace: workspace, Tool: &file.ReadFileTool{WorkDir: "/must-not-execute-on-host"}, Backend: backend}
	result, err = read.Execute(context.Background(), json.RawMessage(`{"path":"proxy.txt"}`))
	if err != nil || !strings.Contains(result.Output, "through container") {
		t.Fatalf("read: %+v %v", result, err)
	}
	large := strings.Repeat("large-response-", 8000)
	if err = os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(large), 0644); err != nil {
		t.Fatal(err)
	}
	result, err = read.Execute(context.Background(), json.RawMessage(`{"path":"large.txt"}`))
	if err != nil || !strings.Contains(result.Output, large) {
		t.Fatalf("large native response lost: bytes=%d error=%v tool_error=%s", len(result.Output), err, result.Error)
	}
	backend.WorkerPath = filepath.Join(workspace, "missing-worker")
	read.Backend = backend
	if _, err = read.Execute(context.Background(), json.RawMessage(`{"path":"proxy.txt"}`)); err == nil {
		t.Fatal("missing worker fell back")
	}
}

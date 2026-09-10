package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/sausheong/hand/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBinaryRPCRejectsOversizedFrameBeforeProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binary := os.Getenv("HAND_TEST_RPC_FRAME_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "hand")
		if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	} else if !filepath.IsAbs(binary) {
		t.Fatal("HAND_TEST_RPC_FRAME_BINARY must be absolute")
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected provider call", 500)
	}))
	defer server.Close()
	home, workspace := t.TempDir(), t.TempDir()
	command := exec.CommandContext(ctx, binary, "--rpc", "--model", "local/fixture", "--base-url", server.URL+"/v1")
	command.Dir = workspace
	command.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	reader := protocol.NewReader(output)
	for _, rejected := range []struct{ frame, code, id string }{
		{`{"version":2,"id":"wrong-version","method":"prompt","params":{"text":"must not execute"}}`, "unsupported_version", ""},
		{`{"version":1,"id":"first","id":"second","method":"prompt","params":{"text":"must not execute"}}`, "invalid_request", ""},
		{`{"version":1,"id":"before-hello","method":"prompt","params":{"text":"must not execute"}}`, "not_negotiated", "before-hello"},
	} {
		if _, err := io.WriteString(input, rejected.frame+"\n"); err != nil {
			t.Fatal(err)
		}
		raw, err := reader.ReadFrame()
		if err != nil {
			t.Fatal("rejection did not preserve connection", err)
		}
		var response protocol.Response
		if err := json.Unmarshal(raw, &response); err != nil || response.Version != 1 || response.RequestID != rejected.id || response.Error == nil || response.Error.Code != rejected.code || len(response.Result) != 0 {
			t.Fatal("wrong bounded protocol rejection", string(raw), err)
		}
		if len(raw) > 4096 {
			t.Fatal("unbounded protocol error")
		}
	}
	if err := protocol.NewWriter(input).Write(protocol.Request{Version: 1, ID: "hello", Method: "hello", Params: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var hello protocol.Response
	if err := json.Unmarshal(raw, &hello); err != nil || hello.RequestID != "hello" || hello.Error != nil {
		t.Fatal("stdout is not a valid negotiation response", string(raw), err)
	}
	// The complete JSON request is valid except for its frame size. A second
	// prompt follows it to ensure the stream cannot resume mid-rejected frame.
	oversized := `{"version":1,"id":"oversized","method":"prompt","params":{"text":"` + strings.Repeat("x", protocol.MaxFrameBytes) + `"}}` + "\n" + `{"version":1,"id":"later","method":"prompt","params":{"text":"must not execute"}}` + "\n"
	sent := make(chan struct{})
	go func() { defer close(sent); io.WriteString(input, oversized); input.Close() }()
	remaining, err := io.ReadAll(io.LimitReader(output, 4097))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatal("fatal frame emitted unexpected stdout", string(remaining))
	}
	if err := command.Wait(); err == nil {
		t.Fatal("oversized frame reported success")
	}
	<-sent
	if ctx.Err() != nil {
		t.Fatal("frame rejection hung", ctx.Err())
	}
	if !strings.Contains(stderr.String(), "1048576") || stderr.Len() > 4096 {
		t.Fatal("missing or unbounded size diagnostic", stderr.String())
	}
	if calls.Load() != 0 {
		t.Fatal("rejected frame or trailing prompt reached provider", calls.Load())
	}
	ledgers, err := filepath.Glob(filepath.Join(home, ".hand", "sessions", "rpc-*", "requests.jsonl"))
	if err != nil || len(ledgers) != 1 {
		t.Fatal("missing durable request ledger", ledgers, err)
	}
	journal, err := os.ReadFile(ledgers[0])
	if err != nil || len(journal) != 0 {
		t.Fatal("rejected prompt was durably admitted", string(journal), err)
	}

}

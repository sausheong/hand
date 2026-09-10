//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	handrpc "github.com/sausheong/hand/internal/rpc"
	"github.com/sausheong/hand/protocol"
)

func TestRPCPipeDisconnectWithQueuedRequests(t *testing.T) {
	for _, direction := range []string{"input", "output", "cancel"} {
		t.Run(direction, func(t *testing.T) { checkRPCPipeDisconnect(t, direction) })
	}
}

func TestRPCPipeCloseInterruptsIdleRead(t *testing.T) {
	input, peerInput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer peerInput.Close()
	peerOutput, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer peerOutput.Close()
	defer output.Close()
	connection, err := prepareRPCFiles(input, output)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		_, err := connection.Read(make([]byte, 1))
		done <- err
	}()
	<-started
	// Let the reader enter the poller with an open peer and no input.
	time.Sleep(10 * time.Millisecond)
	closed := make(chan struct{})
	go func() { connection.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		peerInput.Close()
		t.Fatal("close did not interrupt idle reader and join monitor")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("idle read succeeded without input")
		}
	case <-time.After(time.Second):
		t.Fatal("reader survived close")
	}
}

func TestRPCPipeTruncatedFrame(t *testing.T) {
	input, peerInput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer peerInput.Close()
	peerOutput, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer peerOutput.Close()
	defer output.Close()
	connection, err := prepareRPCFiles(input, output)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	private := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	ledger, err := handrpc.OpenLedger(filepath.Join(private, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- handrpc.Serve(ctx, connection, handrpc.NewDispatcher(app.New(nil, app.Options{}), ledger), handrpc.TransportOptions{})
	}()
	if err := protocol.NewWriter(peerInput).Write(protocol.Request{Version: 1, ID: "hello", Method: "hello", Params: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.NewReader(peerOutput).ReadFrame(); err != nil {
		t.Fatal(err)
	}
	if _, err := peerInput.Write([]byte(`{"version":1,"id":"unfinished"`)); err != nil {
		t.Fatal(err)
	}
	peerInput.Close()
	if err := <-done; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated frame must fail promptly, got %v", err)
	}
}

func checkRPCPipeDisconnect(t *testing.T, direction string) {
	input, peerInput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer peerInput.Close()
	peerOutput, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer peerOutput.Close()
	defer output.Close()
	connection, err := prepareRPCFiles(input, output)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, joined := make(chan struct{}), make(chan struct{})
	service := app.New(nil, app.Options{SessionID: "session", MaxIterations: 1, ResolveInput: func(ctx context.Context, _ string) (agentio.PromptInput, error) {
		close(entered)
		<-ctx.Done()
		close(joined)
		return agentio.PromptInput{}, ctx.Err()
	}})
	private := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	ledger, err := handrpc.OpenLedger(filepath.Join(private, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	done := make(chan error, 1)
	go func() {
		done <- handrpc.Serve(ctx, connection, handrpc.NewDispatcher(service, ledger), handrpc.TransportOptions{})
	}()
	finished := false
	defer func() {
		if !finished {
			cancel()
			connection.Close()
			<-done
		}
	}()
	request := func(id, method, params string) protocol.Request {
		return protocol.Request{Version: protocol.Version, ID: id, Method: method, Params: json.RawMessage(params)}
	}
	writer, reader := protocol.NewWriter(peerInput), protocol.NewReader(peerOutput)
	for _, r := range []protocol.Request{request("hello", "hello", `{}`), request("enqueue", "followup.enqueue", `{"text":"@slow"}`)} {
		if err := writer.Write(r); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.ReadFrame(); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Write(request("start", "followup.start", `{}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("resolver not entered")
	}
	for _, id := range []string{"queued-1", "queued-2", "queued-3"} {
		if err := writer.Write(request(id, "followup.enqueue", `{"text":"must not execute"}`)); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	switch direction {
	case "input":
		peerInput.Close()
	case "output":
		peerOutput.Close()
	case "cancel":
		cancel()
	}
	select {
	case err := <-done:
		finished = true
		if direction == "cancel" {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel result: %v", err)
			}
		} else if err != nil {
			t.Fatalf("disconnect result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued requests hid pipe disconnection")
	}
	select {
	case <-joined:
	default:
		t.Fatal("resolver survived disconnect")
	}
	for _, id := range []string{"queued-1", "queued-2", "queued-3"} {
		if _, err := ledger.Lookup(id); err == nil {
			t.Fatal("queued request executed after disconnect")
		}
	}
	t.Logf("queued_pipe_disconnect_join_ms=%.3f", float64(time.Since(start).Microseconds())/1000)
}

//go:build darwin || linux

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
)

func transportDispatcher(t *testing.T, b *rpcBackend) *Dispatcher {
	t.Helper()
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return NewDispatcher(app.New(b, app.Options{SessionID: "session", MaxIterations: 1}), l)
}

func TestPreparedTransportCancelsStartupOnEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, client := net.Pipe()
	transport := PrepareTransport(ctx, server, cancel)
	defer transport.Close()
	client.Close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("EOF did not cancel startup")
	}
}

func TestPreparedTransportPipelinedEOFUnblocksStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, client := net.Pipe()
	defer client.Close()
	transport := PrepareTransport(ctx, server, cancel)
	defer transport.Close()
	writer := protocol.NewWriter(client)
	for _, id := range []string{"first", "second"} {
		if err := writer.Write(rpcRequest(id, "hello", `{}`)); err != nil {
			t.Fatal(err)
		}
	}
	client.Close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("queued frames prevented startup disconnection")
	}
	if !errors.Is(transport.StartupError(), ErrStartupBacklog) {
		t.Fatalf("missing startup backlog diagnostic: %v", transport.StartupError())
	}
}

func TestPreparedTransportPreservesQueuedNegotiation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, client := net.Pipe()
	defer client.Close()
	transport := PrepareTransport(ctx, server, cancel)
	defer transport.Close()
	request := rpcRequest("queued", "hello", `{}`)
	if err := protocol.NewWriter(client).Write(request); err != nil {
		t.Fatal(err)
	}
	d := transportDispatcher(t, &rpcBackend{joined: make(chan struct{})})
	done := make(chan error, 1)
	go func() { done <- ServePrepared(transport, d, TransportOptions{}) }()
	raw, err := protocol.NewReader(client).ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var response protocol.Response
	if err := json.Unmarshal(raw, &response); err != nil || response.Error != nil || response.RequestID != "queued" {
		t.Fatalf("queued request lost: %s %v", raw, err)
	}
	client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("transferred reader did not join")
	}
	if ctx.Err() != nil {
		t.Fatal("startup callback retained after dispatcher handoff")
	}
}
func TestTransportNegotiationAndDisconnectJoins(t *testing.T) {
	backend := &rpcBackend{hang: true, joined: make(chan struct{})}
	d := transportDispatcher(t, backend)
	server, client := net.Pipe()
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, d, TransportOptions{}) }()
	writer := protocol.NewWriter(client)
	reader := protocol.NewReader(client)
	for _, request := range []protocol.Request{rpcRequest("h", "hello", `{}`), rpcRequest("p1", "prompt", `{"text":"hello"}`)} {
		if err := writer.Write(request); err != nil {
			t.Fatal(err)
		}
		raw, err := reader.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		var response protocol.Response
		if err = json.Unmarshal(raw, &response); err != nil || response.Error != nil || response.RequestID != request.ID {
			t.Fatalf("response %s %v", raw, err)
		}
	}
	deadline := time.After(3 * time.Second)
	for backend.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("backend not started")
		case <-time.After(time.Millisecond):
		}
	}
	client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not join")
	}
	select {
	case <-backend.joined:
	default:
		t.Fatal("backend survived disconnect")
	}
	completedRequest(t, d.ledger)
}
func TestTransportSlowReaderIsClosed(t *testing.T) {
	d := transportDispatcher(t, &rpcBackend{joined: make(chan struct{})})
	server, client := net.Pipe()
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		done <- Serve(context.Background(), server, d, TransportOptions{WriteTimeout: 20 * time.Millisecond})
	}()
	if err := protocol.NewWriter(client).Write(rpcRequest("h", "hello", `{}`)); err != nil {
		t.Fatal(err)
	}
	// Never read the response: the server write must be interrupted and joined.
	select {
	case err := <-done:
		if !errors.Is(err, ErrWriteTimeout) {
			t.Fatalf("timeout %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked writer leaked")
	}
}
func TestTransportParentCancellationUnblocksRead(t *testing.T) {
	d := transportDispatcher(t, &rpcBackend{joined: make(chan struct{})})
	server, client := net.Pipe()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, server, d, TransportOptions{}) }()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked reader leaked")
	}
}

func TestTransportDisconnectUnblocksFollowupResolution(t *testing.T) {
	l, err := OpenLedger(ledgerPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	entered := make(chan struct{})
	service := app.New(nil, app.Options{SessionID: "session", MaxIterations: 1, ResolveInput: func(ctx context.Context, _ string) (agentio.PromptInput, error) {
		close(entered)
		<-ctx.Done()
		return agentio.PromptInput{}, ctx.Err()
	}})
	d := NewDispatcher(service, l)
	server, client := net.Pipe()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, server, d, TransportOptions{}) }()
	writer := protocol.NewWriter(client)
	reader := protocol.NewReader(client)
	for _, r := range []protocol.Request{rpcRequest("h", "hello", `{}`), rpcRequest("f", "followup.enqueue", `{"text":"@slow"}`)} {
		if err = writer.Write(r); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.ReadFrame(); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Write(rpcRequest("p1", "followup.start", `{}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("resolver not entered")
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("resolution prevented transport cancellation")
	}
}

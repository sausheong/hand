//go:build darwin || linux

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/harness/llm"
)

type recoveryEffectBackend struct {
	rpcBackend
	marker string
}

func (b *recoveryEffectBackend) Run(ctx context.Context, prompt string, images []llm.ImageContent) (<-chan app.BackendEvent, error) {
	f, err := os.OpenFile(b.marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString("executed\n")
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return b.rpcBackend.Run(ctx, prompt, images)
}

func TestSlowConsumerReconnectRecoversExactTerminalWithoutReplay(t *testing.T) {
	path := ledgerPath(t)
	marker := filepath.Join(t.TempDir(), "effects")
	ledger, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ledger.Close() }()
	backend := &recoveryEffectBackend{rpcBackend: rpcBackend{joined: make(chan struct{})}, marker: marker}
	dispatcher := NewDispatcher(app.New(backend, app.Options{SessionID: "session", MaxIterations: 1}), ledger)
	server, peer := net.Pipe()
	defer peer.Close()
	done := make(chan error, 1)
	go func() {
		done <- Serve(context.Background(), server, dispatcher, TransportOptions{WriteTimeout: 100 * time.Millisecond})
	}()
	call := func(conn net.Conn, req protocol.Request) json.RawMessage {
		t.Helper()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		if err := protocol.NewWriter(conn).Write(req); err != nil {
			t.Fatal(err)
		}
		raw, err := protocol.NewReader(conn).ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		var response protocol.Response
		if err := json.Unmarshal(raw, &response); err != nil || response.Error != nil || response.RequestID != req.ID {
			t.Fatal("invalid response", string(raw), err)
		}
		return response.Result
	}
	call(peer, rpcRequest("hello", "hello", `{}`))
	prompt := rpcRequest("p1", "prompt", `{"text":"perform effect"}`)
	call(peer, prompt)
	terminal := completedRequest(t, ledger)
	var event protocol.Event
	if err := json.Unmarshal(terminal.Result, &event); err != nil || event.Kind != "terminal" {
		t.Fatal("terminal missing", err)
	}
	var page struct {
		Gap    bool
		Events []json.RawMessage
	}
	raw := call(peer, rpcRequest("poll", "events.poll", `{"after":0}`))
	if err := json.Unmarshal(raw, &page); err != nil || !page.Gap || len(page.Events) > 16 {
		t.Fatal("progress did not overflow bounded retention", string(raw), err)
	}
	// Request the durable result but stop reading. Serve must close and join.
	if err := protocol.NewWriter(peer).Write(rpcRequest("unread", "request.get", `{"id":"p1"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrWriteTimeout) {
			t.Fatal("wrong slow-consumer termination", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("slow consumer not joined")
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	ledger, err = OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	fresh := &recoveryEffectBackend{rpcBackend: rpcBackend{joined: make(chan struct{})}, marker: marker}
	dispatcher = NewDispatcher(app.New(fresh, app.Options{SessionID: "session", MaxIterations: 1}), ledger)
	server2, peer2 := net.Pipe()
	defer peer2.Close()
	done2 := make(chan error, 1)
	go func() { done2 <- Serve(context.Background(), server2, dispatcher, TransportOptions{}) }()
	call(peer2, rpcRequest("hello-again", "hello", `{}`))
	var recovered RequestRecord
	if err := json.Unmarshal(call(peer2, rpcRequest("recover", "request.get", `{"id":"p1"}`)), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.State != "completed" || !bytes.Equal(recovered.Result, terminal.Result) || recovered.RunID != terminal.RunID {
		t.Fatal("terminal changed across reconnect", recovered)
	}
	call(peer2, prompt)
	if fresh.calls.Load() != 0 || backend.calls.Load() != 1 {
		t.Fatal("reconnect replay executed backend")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "executed\n" {
		t.Fatal("side effect repeated", string(data), err)
	}
	peer2.Close()
	select {
	case err := <-done2:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reconnected transport not joined")
	}
}

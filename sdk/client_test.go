//go:build darwin || linux

package sdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/rpc"
	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
	"github.com/sausheong/harness/llm"
)

type backend struct{ joined chan struct{} }

func (*backend) StopReason() string { return "" }
func (b *backend) Run(ctx context.Context, _ string, _ []llm.ImageContent) (<-chan app.BackendEvent, error) {
	ch := make(chan app.BackendEvent)
	go func() { defer close(ch); defer close(b.joined); <-ctx.Done() }()
	return ch, nil
}
func TestClientRealDispatcherCancellationAndTerminal(t *testing.T) {
	l, err := rpc.OpenLedger(filepath.Join(t.TempDir(), "ledger", "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	b := &backend{joined: make(chan struct{})}
	d := rpc.NewDispatcher(app.New(b, app.Options{SessionID: "sdk-session", MaxIterations: 1}), l)
	server, peer := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(context.Background(), server, d, rpc.TransportOptions{}) }()
	c := sdk.NewClient(peer)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = c.Hello(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	request, err := sdk.NewPromptRequest("prompt", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Submit(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Cancel(ctx, "cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-b.joined:
	case <-ctx.Done():
		t.Fatal("backend did not join")
	}

	for {
		record, e := c.Lookup(ctx, "record", "prompt")
		if e != nil {
			t.Fatal(e)
		}
		if terminal, ok := record.Terminal(); ok {
			if terminal.Kind() != "terminal" || terminal.SessionID() != "sdk-session" || terminal.Status() != "cancelled" {
				t.Fatalf("terminal %s %s %s", terminal.Kind(), terminal.SessionID(), terminal.Status())
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal missing")
		case <-time.After(time.Millisecond):
		}
	}
	page, err := c.PollEvents(ctx, "events", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events()) == 0 || page.Next() == 0 || page.Latest() < page.Next() {
		t.Fatal("empty event page")
	}
	events := page.Events()
	first := events[0].Event().ID()
	events[0] = sdk.CursorEvent{}
	if page.Events()[0].Event().ID() != first {
		t.Fatal("page snapshot was mutated")
	}

	c.Close()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("transport did not join")
	}
}
func TestClientCancellationInterruptsBlockedRead(t *testing.T) {
	server, peer := net.Pipe()
	defer server.Close()
	c := sdk.NewClient(peer)
	defer c.Close()
	received := make(chan struct{})
	go func() { protocol.NewReader(server).ReadFrame(); close(received) }()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := c.Hello(ctx, "h"); result <- err }()
	<-received
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked read did not cancel")
	}
}
func TestClientConcurrentCallsAndResponseIdentity(t *testing.T) {
	server, peer := net.Pipe()
	defer server.Close()
	c := sdk.NewClient(peer)
	defer c.Close()
	go func() {
		reader := protocol.NewReader(server)
		writer := protocol.NewWriter(server)
		for i := 0; i < 8; i++ {
			frame, err := reader.ReadFrame()
			if err != nil {
				return
			}
			req, err := protocol.DecodeRequest(frame)
			if err != nil {
				return
			}
			writer.Write(protocol.Response{Version: 1, RequestID: req.ID, Result: json.RawMessage(`{"ok":true}`)})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Hello(ctx, id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
func TestClientRejectsMismatchedResponse(t *testing.T) {
	server, peer := net.Pipe()
	defer server.Close()
	c := sdk.NewClient(peer)
	defer c.Close()
	go func() {
		protocol.NewReader(server).ReadFrame()
		protocol.NewWriter(server).Write(protocol.Response{Version: 1, RequestID: "wrong", Result: json.RawMessage(`{}`)})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, "hello"); err == nil {
		t.Fatal("accepted mismatched response")
	}
}

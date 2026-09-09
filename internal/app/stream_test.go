package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

func TestFullProgressQueueCannotBlockCancellationOrTerminal(t *testing.T) {
	joined := make(chan struct{})
	backend := &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
		events := make(chan BackendEvent)
		go func() {
			defer close(events)
			defer close(joined)
			events <- BackendEvent{Text: strings.Repeat("x", 2<<20)}
			<-ctx.Done()
		}()
		return events, nil
	}}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	stream, err := service.Start(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	deadline := time.After(3 * time.Second)
	for len(stream.Events) < EventQueueCapacity {
		select {
		case <-deadline:
			t.Fatal("queue never filled")
		case <-time.After(time.Millisecond):
		}
	}
	if !stream.Cancel() {
		t.Fatal("cancellation rejected")
	}
	select {
	case <-stream.Done:
	case <-time.After(3 * time.Second):
		t.Fatal("full queue blocked cleanup")
	}
	select {
	case <-joined:
	default:
		t.Fatal("terminal before backend joined")
	}
	result, err := stream.Wait()
	if err != nil || result.Status != agentio.Cancelled {
		t.Fatalf("%+v %v", result, err)
	}
	final, ok := <-stream.Terminal
	if !ok || final.Kind != "terminal" || final.Status != agentio.Cancelled {
		t.Fatalf("missing terminal %+v", final)
	}
	if _, ok := <-stream.Terminal; ok {
		t.Fatal("duplicate terminal")
	}
	count, bytes := 0, 0
	for event := range stream.Events {
		count++
		bytes += len(event.Text)
		if len(event.Text) > MaxEventTextBytes {
			t.Fatal("oversized progress chunk")
		}
	}
	if count > EventQueueCapacity || bytes > EventQueueCapacity*MaxEventTextBytes {
		t.Fatalf("unbounded queue: %d events %d bytes", count, bytes)
	}
}

func TestStreamPreservesUnicodeTextAndOrdering(t *testing.T) {
	text := strings.Repeat("hello世界🙂", 10000)
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) {
		events := make(chan BackendEvent, 2)
		events <- BackendEvent{Text: text}
		events <- BackendEvent{Done: true}
		close(events)
		return events, nil
	}}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	stream, err := service.Start(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	var sequence uint64
	for event := range stream.Events {
		if event.Sequence <= sequence {
			t.Fatal("sequence regressed")
		}
		sequence = event.Sequence
		if event.Kind == "text" {
			if len(event.Text) > MaxEventTextBytes || !utf8.ValidString(event.Text) {
				t.Fatal("invalid text chunk")
			}
			rendered.WriteString(event.Text)
		}
	}
	final := <-stream.Terminal
	if final.Sequence <= sequence || final.Status != agentio.Completed || rendered.String() != text {
		t.Fatal("stream lost or reordered text")
	}
	if result, err := stream.Wait(); err != nil || result.Status != agentio.Completed {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestOldStreamCannotCancelNewRunAndStartClaimsSynchronously(t *testing.T) {
	calls := 0
	started := make(chan struct{})
	backend := &backendFixture{run: func(ctx context.Context, _ string) (<-chan BackendEvent, error) {
		calls++
		if calls == 1 {
			return completedStream(), nil
		}
		events := make(chan BackendEvent)
		go func() { close(started); <-ctx.Done(); close(events) }()
		return events, nil
	}}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	old, err := service.Start(context.Background(), "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range old.Events {
	}
	old.Wait()
	current, err := service.Start(context.Background(), "second", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	if _, err := service.Start(context.Background(), "overlap", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("ownership not claimed: %v", err)
	}
	<-started
	if old.Cancel() {
		t.Fatal("stale handle cancelled current run")
	}
	if service.Snapshot().State != Running {
		t.Fatal(service.Snapshot())
	}
	current.Cancel()
	select {
	case <-current.Done:
	case <-time.After(3 * time.Second):
		t.Fatal("second run did not cancel")
	}
}

type imageBackendFixture struct {
	allow    chan struct{}
	received chan llm.ImageContent
}

func (b *imageBackendFixture) StopReason() string { return "" }
func (b *imageBackendFixture) Run(_ context.Context, _ string, images []llm.ImageContent) (<-chan BackendEvent, error) {
	<-b.allow
	b.received <- images[0]
	return completedStream(), nil
}
func TestStartOwnsImageData(t *testing.T) {
	backend := &imageBackendFixture{allow: make(chan struct{}), received: make(chan llm.ImageContent, 1)}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	images := []llm.ImageContent{{MimeType: "image/png", Data: []byte{1, 2}}}
	stream, err := service.Start(context.Background(), "hello", images)
	if err != nil {
		t.Fatal(err)
	}
	images[0].MimeType = "changed"
	images[0].Data[0] = 9
	close(backend.allow)
	received := <-backend.received
	if received.MimeType != "image/png" || received.Data[0] != 1 {
		t.Fatal("caller mutation reached active request")
	}
	for range stream.Events {
	}
	stream.Wait()
}

func TestTerminalErrorIsBoundedWithoutLosingOriginalCause(t *testing.T) {
	cause := errors.New(strings.Repeat("界", MaxEventTextBytes))
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return nil, cause }}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	stream, err := service.Start(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Events {
	}
	terminal := <-stream.Terminal
	if len(terminal.Error) > MaxEventTextBytes || !terminal.Truncated || !utf8.ValidString(terminal.Error) {
		t.Fatal("terminal error is unbounded or invalid UTF-8")
	}
	result, err := stream.Wait()
	if err != nil || !errors.Is(result.Cause, cause) {
		t.Fatal("original error chain was lost")
	}
}

func TestOldStreamCannotCancelLaterCompactionReservation(t *testing.T) {
	backend := &backendFixture{run: func(context.Context, string) (<-chan BackendEvent, error) { return completedStream(), nil }}
	service := New(backend, Options{SessionID: "s", MaxIterations: 1})
	stream, err := service.Start(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Events {
	}
	stream.Wait()
	operation, release, err := service.reserve(context.Background(), Compacting)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if stream.Cancel() || operation.Err() != nil {
		t.Fatal("old run handle cancelled a later compaction operation")
	}
}

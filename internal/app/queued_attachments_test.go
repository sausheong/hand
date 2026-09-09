package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

type queuedImageBackend struct {
	prompt string
	images []llm.ImageContent
	calls  int
}

func (b *queuedImageBackend) Run(_ context.Context, p string, images []llm.ImageContent) (<-chan BackendEvent, error) {
	b.calls++
	b.prompt = p
	b.images = images
	return completedStream(), nil
}
func (*queuedImageBackend) StopReason() string { return "" }

func TestFollowupResolvesImagesAndFilesAtAdmission(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "image file.png"), []byte("pixels"), 0600)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("initial"), 0600)
	backend := &queuedImageBackend{}
	owner := New(backend, Options{MaxIterations: 1, ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(ctx, dir, text)
	}})
	input, _ := owner.EnqueueInput(FollowupQueue, `review "image file.png" @note.txt`)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("latest contents"), 0600)
	stream, accepted, err := owner.StartFollowup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for range stream.Events {
	}
	stream.Wait()
	if accepted.ID != input.ID || backend.calls != 1 || len(backend.images) != 1 || string(backend.images[0].Data) != "pixels" || !strings.Contains(backend.prompt, "latest contents") {
		t.Fatal(accepted, backend)
	}
}

func TestFollowupAttachmentFailurePreservesPendingInput(t *testing.T) {
	dir := t.TempDir()
	backend := &queuedImageBackend{}
	owner := New(backend, Options{MaxIterations: 1, InputTypes: []string{"text"}, ResolveInput: func(ctx context.Context, text string) (agentio.PromptInput, error) {
		return agentio.ParsePromptInput(ctx, dir, text)
	}})
	input, _ := owner.EnqueueInput(FollowupQueue, "image.png")
	for _, exists := range []bool{false, true} {
		if exists {
			os.WriteFile(filepath.Join(dir, "image.png"), []byte("pixels"), 0600)
		}
		if _, _, err := owner.StartFollowup(context.Background()); err == nil {
			t.Fatal("invalid attachment admission accepted")
		}
		queue := owner.QueuedInputs()
		if len(queue) != 1 || queue[0] != input || backend.calls != 0 || owner.Snapshot().RunID != 0 {
			t.Fatal(queue, backend.calls)
		}
	}
}

func TestFollowupConcurrentEditCannotDeliverStaleAttachments(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	backend := &queuedImageBackend{}
	owner := New(backend, Options{MaxIterations: 1, ResolveInput: func(_ context.Context, text string) (agentio.PromptInput, error) {
		close(entered)
		<-release
		return agentio.PromptInput{Prompt: text}, nil
	}})
	input, _ := owner.EnqueueInput(FollowupQueue, "original")
	done := make(chan error, 1)
	go func() { _, _, err := owner.StartFollowup(context.Background()); done <- err }()
	<-entered
	if err := owner.EditQueuedInput(input.ID, "edited"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("stale input admitted")
	}
	queue := owner.QueuedInputs()
	if len(queue) != 1 || queue[0].Text != "edited" || backend.calls != 0 {
		t.Fatal(queue, backend.calls)
	}
}

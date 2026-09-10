package app

import (
	"context"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

func TestStartInputRechecksSessionAfterResolution(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := New(nil, Options{SessionID: "old", ResolveInput: func(context.Context, string) (agentio.PromptInput, error) {
		close(entered)
		<-release
		return agentio.PromptInput{Prompt: "resolved"}, nil
	}})
	done := make(chan error, 1)
	go func() { _, err := s.StartInput(context.Background(), "prompt"); done <- err }()
	<-entered
	s.mu.Lock()
	s.options.SessionID = "new"
	s.mu.Unlock()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("input admitted to different session")
	}
	if s.Snapshot().State != Idle {
		t.Fatal("rejected resolution reserved a run")
	}
}
func TestStartInputValidatesResolvedImageCapability(t *testing.T) {
	s := New(nil, Options{SessionID: "session", InputTypes: []string{"text"}, ResolveInput: func(context.Context, string) (agentio.PromptInput, error) {
		return agentio.PromptInput{Prompt: "image", Images: []llm.ImageContent{{MimeType: "image/png", Data: []byte("fixture")}}}, nil
	}})
	if _, err := s.StartInput(context.Background(), "image"); err == nil {
		t.Fatal("unsupported image admitted")
	}
	if s.Snapshot().State != Idle {
		t.Fatal("capability rejection reserved a run")
	}
}

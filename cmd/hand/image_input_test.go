package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
)

type imageInputProvider struct {
	completedProvider
	calls  int
	images []llm.ImageContent
}

func (p *imageInputProvider) ChatStream(_ context.Context, request llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	p.calls++
	for _, message := range request.Messages {
		p.images = append(p.images, message.Images...)
	}
	ch := make(chan llm.ChatEvent, 2)
	ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: "Answer"}
	ch <- llm.ChatEvent{Type: llm.EventDone}
	close(ch)
	return ch, nil
}

func TestOneShotImageInputMatchesTerminalParser(t *testing.T) {
	workspace := t.TempDir()
	data := []byte("image bytes")
	if err := os.WriteFile(filepath.Join(workspace, "sample image.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	provider := &imageInputProvider{}
	rt := &runtime.Runtime{LLM: provider, Tools: tool.NewRegistry(), Session: session.NewSession("hand", "key"), AgentID: "hand", Model: "test", MaxTurns: 1}
	reason := ""
	outcome := runOneShotOutcome(context.Background(), rt, `inspect @"sample image.png"`, nil, workspace, &reason, 1)
	if outcome.Status != agentio.Completed || provider.calls != 1 || len(provider.images) != 1 || string(provider.images[0].Data) != string(data) {
		t.Fatal(outcome, provider.calls, provider.images)
	}
}

func TestOneShotAttachmentErrorsPreventProviderAndJournal(t *testing.T) {
	for _, mode := range []string{"missing", "text_profile"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			if mode == "text_profile" {
				os.WriteFile(filepath.Join(workspace, "image.png"), []byte("image"), 0600)
			}
			provider := &imageInputProvider{}
			sess := session.NewSession("hand", "key")
			rt := &runtime.Runtime{LLM: provider, Tools: tool.NewRegistry(), Session: sess, AgentID: "hand", Model: "test", MaxTurns: 1}
			reason := ""
			outcome := runOneShotOutcome(context.Background(), rt, "image.png", nil, workspace, &reason, 1, config.ModelProfile{InputTypes: []string{"text"}})
			if outcome.Status != agentio.InfrastructureError || outcome.Cause == nil || provider.calls != 0 || len(sess.Entries()) != 0 {
				t.Fatal(outcome, provider.calls, sess.Entries())
			}
		})
	}
}

func TestOneShotExternalAttachmentGrant(t *testing.T) {
	workspace := t.TempDir()
	external := filepath.Join(t.TempDir(), "external image.png")
	if err := os.WriteFile(external, []byte("selected image"), 0600); err != nil {
		t.Fatal(err)
	}
	policy, err := agentio.NewAttachmentPolicy(context.Background(), []string{external})
	if err != nil {
		t.Fatal(err)
	}
	for _, granted := range []bool{false, true} {
		provider := &imageInputProvider{}
		sess := session.NewSession("hand", "key")
		rt := &runtime.Runtime{LLM: provider, Tools: tool.NewRegistry(), Session: sess, AgentID: "hand", Model: "test", MaxTurns: 1}
		ctx := context.Background()
		if granted {
			ctx = agentio.WithAttachmentPolicy(ctx, policy)
		}
		reason := ""
		outcome := runOneShotOutcome(ctx, rt, "inspect @\""+external+"\"", nil, workspace, &reason, 1)
		if granted {
			if outcome.Status != agentio.Completed || provider.calls != 1 || len(provider.images) != 1 || string(provider.images[0].Data) != "selected image" {
				t.Fatal(outcome, provider.calls, provider.images)
			}
		} else if provider.calls != 0 || len(sess.Entries()) != 0 || outcome.Status != agentio.InfrastructureError {
			t.Fatal(outcome, provider.calls, sess.Entries())
		}
	}
}

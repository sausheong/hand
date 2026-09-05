package agentio_test

import (
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

func TestBuildCompactionManager_SetsSummarizerFields(t *testing.T) {
	var fakeProvider llm.LLMProvider = struct{ llm.LLMProvider }{}

	mgr := agentio.BuildCompactionManager(fakeProvider, "claude-sonnet-5", 0.4)

	if mgr == nil {
		t.Fatal("BuildCompactionManager returned nil")
	}
	if mgr.Summarizer == nil {
		t.Fatal("Summarizer is nil")
	}
	if mgr.Summarizer.Provider != fakeProvider {
		t.Errorf("Summarizer.Provider = %v, want %v", mgr.Summarizer.Provider, fakeProvider)
	}
	if mgr.Summarizer.Model != "claude-sonnet-5" {
		t.Errorf("Summarizer.Model = %q, want %q", mgr.Summarizer.Model, "claude-sonnet-5")
	}
}

func TestBuildCompactionManager_SetsThresholdAndMessageCap(t *testing.T) {
	var fakeProvider llm.LLMProvider = struct{ llm.LLMProvider }{}

	mgr := agentio.BuildCompactionManager(fakeProvider, "claude-sonnet-5", 0.4)

	if mgr.Threshold != 0.4 {
		t.Errorf("Threshold = %v, want 0.4 (the value passed in, not harness's own 0.6 default)", mgr.Threshold)
	}
	if mgr.MessageCap <= 0 {
		t.Errorf("MessageCap = %d, want a positive backstop set", mgr.MessageCap)
	}
}

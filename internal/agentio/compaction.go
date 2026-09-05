package agentio

import (
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
)

// BuildCompactionManager returns a compaction.Manager whose Summarizer
// reuses the same provider and model as the active run — the simplest
// thing that works, with no new API key or config surface. model is the
// bare model name (no provider prefix), matching Summarizer.Model.
//
// PreserveTurns/Threshold/MessageCap are left zero to take harness's
// built-in defaults (K=4 turns preserved; the runtime's own 0.6
// preventive-compaction threshold).
//
// Reusing the same provider instance is safe: a background
// MaybeCompactAsync goroutine can call ChatStream on it while the main
// turn's own ChatStream call is still in flight, but hand's concrete
// providers/* implementations hold no mutable state beyond an immutable
// SDK client handle, so concurrent calls don't race.
func BuildCompactionManager(provider llm.LLMProvider, model string) *compaction.Manager {
	return &compaction.Manager{
		Summarizer: &compaction.Summarizer{Provider: provider, Model: model},
	}
}

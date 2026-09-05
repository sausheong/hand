package agentio

import (
	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
)

// defaultCompactionMessageCap backstops tool-call-heavy turns that grow
// message count faster than token count — many quick file reads/greps,
// each a tool_call + tool_result pair, can rack up messages well before
// crossing a token threshold. 60 is roughly 25-30 such round trips plus
// headroom for interleaved assistant text: generous enough not to
// interrupt a normal exploratory turn, tight enough to backstop the
// pathological case (see config.DefaultCompactionThreshold's own
// comment for the fuller "why" — a large tool result re-billed on every
// subsequent call in the same turn until compaction removes it).
// Deliberately not user-configurable, unlike Threshold: it's a
// backstop, not a primary tuning knob, and 60 messages is generous
// enough that a legitimate need to raise it is unlikely.
const defaultCompactionMessageCap = 60

// BuildCompactionManager returns a compaction.Manager whose Summarizer
// reuses the same provider and model as the active run — the simplest
// thing that works, with no new API key or config surface. model is the
// bare model name (no provider prefix), matching Summarizer.Model.
// threshold is the fraction of the context window that triggers
// preventive compaction — pass config.ResolveCompactionThreshold's
// result, not a raw flag/config value; see that function and
// config.DefaultCompactionThreshold for why hand deliberately doesn't
// use harness's own, more permissive built-in default (0.6) here.
//
// PreserveTurns is left zero to take harness's built-in default (K=4
// turns preserved).
//
// Reusing the same provider instance is safe: a background
// MaybeCompactAsync goroutine can call ChatStream on it while the main
// turn's own ChatStream call is still in flight, but hand's concrete
// providers/* implementations hold no mutable state beyond an immutable
// SDK client handle, so concurrent calls don't race.
func BuildCompactionManager(provider llm.LLMProvider, model string, threshold float64) *compaction.Manager {
	return &compaction.Manager{
		Summarizer: &compaction.Summarizer{Provider: provider, Model: model},
		Threshold:  threshold,
		MessageCap: defaultCompactionMessageCap,
	}
}

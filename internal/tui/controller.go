package tui

import (
	"context"
	"fmt"

	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

// Controller gives the TUI's slash commands (/model, /new, /compact)
// access to state that lives outside the Model: the active *runtime.Runtime
// and its session store. BuildProvider is supplied by the caller (main.go
// knows about provider packages and API keys; tui deliberately doesn't) and
// is only invoked when /model names a different provider than the one
// currently active.
type Controller struct {
	Rt         *runtime.Runtime
	Store      *session.Store
	SessionKey string
	BaseURL    string

	BuildProvider func(providerName, baseURL string) (llm.LLMProvider, error)

	// BuildModelIdentityHint returns the per-turn "you are running as
	// model X" text (see agentio.ModelIdentityHint) for a given
	// "provider/model" string. main.go supplies this; tui deliberately
	// doesn't know how that text is worded, same reasoning as
	// BuildProvider above.
	BuildModelIdentityHint func(providerModel string) string
}

// CurrentModel returns the active "provider/model" string.
func (c *Controller) CurrentModel() string {
	return c.Rt.Provider + "/" + c.Rt.Model
}

// SwitchModel points subsequent turns at a different provider/model.
// Switching to a different provider rebuilds the LLM client via
// BuildProvider; switching within the same provider just updates the
// model name. Either way, it also keeps two other Runtime fields that
// are otherwise silently pinned to hand's startup provider/model in
// sync: the compaction summarizer (agentio.BuildCompactionManager
// deliberately reuses the active run's own provider/model, an invariant
// this must preserve across a switch too) and FallbackModel (a bare
// model id meaningful only against the provider it was resolved for).
func (c *Controller) SwitchModel(providerModel string) error {
	providerName, modelName := llm.ParseProviderModel(providerModel)
	if providerName == "" || modelName == "" {
		return fmt.Errorf("model must be in \"provider/model\" form, e.g. anthropic/claude-sonnet-5")
	}
	if providerName != c.Rt.Provider {
		if c.BuildProvider == nil {
			return fmt.Errorf("switching provider is not supported in this build")
		}
		p, err := c.BuildProvider(providerName, c.BaseURL)
		if err != nil {
			return err
		}
		c.Rt.LLM = p
		c.Rt.Provider = providerName
		// The old FallbackModel is a bare model id for the previous
		// provider; sent to the new client it would misfire as a
		// confusing "model not found" on the next retryable error. hand
		// has no way to guess a sensible fallback for the new provider,
		// so the only safe choice is to drop it — the user can set a new
		// one via --fallback-model/config on their next run.
		c.Rt.FallbackModel = ""
		if c.Rt.Compaction != nil && c.Rt.Compaction.Summarizer != nil {
			c.Rt.Compaction.Summarizer.Provider = p
		}
	}
	c.Rt.Model = modelName
	if c.Rt.Compaction != nil && c.Rt.Compaction.Summarizer != nil {
		c.Rt.Compaction.Summarizer.Model = modelName
	}

	// The active model is named in Rt.DynamicIdentityHint, not the cached
	// StaticSystemPrompt — a fact baked only into a big cached block a
	// model read once near the top of the conversation loses out to
	// whatever the model already told the user in the conversation itself
	// (e.g. its own earlier, correct-at-the-time "which model are you"
	// answer). Resetting it here means the very next turn resends the new
	// fact fresh, with the same recency the model grants its own last
	// statement — see agentio.ModelIdentityHint's doc comment.
	if c.BuildModelIdentityHint != nil {
		c.Rt.DynamicIdentityHint = c.BuildModelIdentityHint(c.CurrentModel())
	}
	return nil
}

// NewSession discards this workspace's saved session and points the
// runtime at a fresh, empty one — the interactive equivalent of the
// --new-session flag.
func (c *Controller) NewSession() error {
	if err := c.Store.Delete(c.Rt.AgentID, c.SessionKey); err != nil {
		return fmt.Errorf("discard session: %w", err)
	}
	sess, err := c.Store.Load(c.Rt.AgentID, c.SessionKey)
	if err != nil {
		return fmt.Errorf("load fresh session: %w", err)
	}
	c.Rt.Session = sess
	return nil
}

// Compact forces a compaction pass on the active session
// (compaction.ReasonManual), bypassing the normal token/turn threshold.
// Result.Skipped explains why nothing happened (e.g. "no_summarizer" if
// this build has no compaction manager configured, "too_short" if there
// aren't enough turns yet to compact).
func (c *Controller) Compact(ctx context.Context) (compaction.Result, error) {
	return c.Rt.Compaction.MaybeCompact(ctx, c.Rt.Session, compaction.ReasonManual, "")
}

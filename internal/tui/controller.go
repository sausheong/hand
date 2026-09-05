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
}

// CurrentModel returns the active "provider/model" string.
func (c *Controller) CurrentModel() string {
	return c.Rt.Provider + "/" + c.Rt.Model
}

// SwitchModel points subsequent turns at a different provider/model.
// Switching to a different provider rebuilds the LLM client via
// BuildProvider; switching within the same provider just updates the
// model name.
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
	}
	c.Rt.Model = modelName
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
